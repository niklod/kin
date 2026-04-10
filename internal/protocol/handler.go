package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/niklod/kin/internal/catalog"
	"github.com/niklod/kin/internal/transfer"
	"github.com/niklod/kin/kinpb"
)

// Conn is the interface the handler uses to communicate with a peer.
type Conn interface {
	Send(*kinpb.Envelope) error
	Recv() (*kinpb.Envelope, error)
	RemoteAddr() string
	PeerID() [32]byte
}

// CatalogExchanger provides catalog data for exchange with peers.
type CatalogExchanger interface {
	ListForPeer(excludeNodeID [32]byte) ([]*catalog.Entry, error)
	PutPeerEntries(peerNodeID [32]byte, entries []*catalog.Entry) error
}

// Handler dispatches incoming protobuf messages for a single peer connection.
type Handler struct {
	sender          *transfer.Sender
	catalog         CatalogExchanger
	selfID          [32]byte
	logger          *slog.Logger
	onCatalogUpdate func()

	mu    sync.Mutex
	conns map[[32]byte]*connEntry // active peer connections
}

// connEntry wraps a Conn with a send mutex to serialize writes from
// Serve (dispatch loop) and BroadcastCatalog (watcher callback).
// It implements the Conn interface so it can be used transparently.
type connEntry struct {
	conn Conn
	mu   sync.Mutex
}

func (e *connEntry) Send(env *kinpb.Envelope) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.conn.Send(env)
}

func (e *connEntry) Recv() (*kinpb.Envelope, error) { return e.conn.Recv() }
func (e *connEntry) RemoteAddr() string             { return e.conn.RemoteAddr() }
func (e *connEntry) PeerID() [32]byte               { return e.conn.PeerID() }

// NewHandler creates a Handler backed by the given file sender and optional
// catalog exchanger. If catalog is nil, catalog exchange is disabled.
func NewHandler(sender *transfer.Sender, cat CatalogExchanger, selfID [32]byte, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{sender: sender, catalog: cat, selfID: selfID, logger: logger, conns: make(map[[32]byte]*connEntry)}
}

// SetOnCatalogUpdate sets a callback invoked when remote catalog entries are received.
// Must be called before any goroutine calls Serve (the go statement that starts
// acceptLoop creates the required happens-before edge).
func (h *Handler) SetOnCatalogUpdate(fn func()) {
	h.onCatalogUpdate = fn
}

// Serve reads messages from conn in a loop and dispatches them until the
// connection closes or ctx is cancelled.
func (h *Handler) Serve(ctx context.Context, conn Conn) {
	peerID := conn.PeerID()
	entry := h.registerConn(peerID, conn)
	defer h.unregisterConn(peerID)

	// Use entry (mutex-wrapped) for all sends to serialize with BroadcastCatalog.
	if h.catalog != nil {
		if err := h.sendCatalogOfferTo(entry); err != nil {
			h.logger.Warn("send catalog offer", "peer", conn.RemoteAddr(), "err", err)
		}
	}

	for {
		if ctx.Err() != nil {
			return
		}
		env, err := conn.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			h.logger.Debug("recv error", "peer", conn.RemoteAddr(), "err", err)
			return
		}
		if err := h.dispatch(ctx, env, entry); err != nil {
			h.logger.Warn("dispatch error", "peer", conn.RemoteAddr(), "err", err)
		}
	}
}

func (h *Handler) dispatch(ctx context.Context, env *kinpb.Envelope, conn Conn) error {
	switch p := env.Payload.(type) {
	case *kinpb.Envelope_FileRequest:
		return h.handleFileRequest(ctx, p.FileRequest, conn)
	case *kinpb.Envelope_CatalogOffer:
		return h.handleCatalogOffer(p.CatalogOffer, conn)
	case *kinpb.Envelope_CatalogAck:
		h.logger.Debug("catalog ack", "peer", conn.RemoteAddr(), "count", p.CatalogAck.ReceivedCount)
		return nil
	default:
		return fmt.Errorf("handler: unhandled message type %T", env.Payload)
	}
}

func (h *Handler) handleFileRequest(ctx context.Context, req *kinpb.FileRequest, conn Conn) error {
	if len(req.FileId) != 32 {
		return conn.Send(&kinpb.Envelope{Payload: &kinpb.Envelope_Error{
			Error: &kinpb.Error{Code: "bad_request", Message: "file_id must be 32 bytes"},
		}})
	}
	var fileID [32]byte
	copy(fileID[:], req.FileId)

	h.logger.Debug("file request", "peer", conn.RemoteAddr(), "file_id", fmt.Sprintf("%x", fileID[:8]))
	return h.sender.HandleRequest(ctx, fileID, conn)
}

// sendCatalogOfferTo builds and sends a catalog offer to a single peer.
func (h *Handler) sendCatalogOfferTo(conn Conn) error {
	entries, err := h.catalog.ListForPeer(conn.PeerID())
	if err != nil {
		return fmt.Errorf("list for peer: %w", err)
	}

	files := catalog.EntriesToProto(entries)
	h.logger.Debug("sending catalog offer", "peer", conn.RemoteAddr(), "files", len(files))
	return conn.Send(&kinpb.Envelope{
		Payload: &kinpb.Envelope_CatalogOffer{
			CatalogOffer: &kinpb.CatalogOffer{Files: files},
		},
	})
}

func (h *Handler) handleCatalogOffer(offer *kinpb.CatalogOffer, conn Conn) error {
	if h.catalog == nil {
		return nil
	}

	peerID := conn.PeerID()
	entries := make([]*catalog.Entry, 0, len(offer.Files))
	for _, f := range offer.Files {
		e, err := catalog.ProtoToEntry(f)
		if err != nil {
			h.logger.Debug("skip bad catalog entry", "peer", conn.RemoteAddr(), "err", err)
			continue
		}
		entries = append(entries, e)
	}

	if err := h.catalog.PutPeerEntries(peerID, entries); err != nil {
		return fmt.Errorf("put peer entries: %w", err)
	}

	h.logger.Debug("received catalog offer", "peer", conn.RemoteAddr(), "accepted", len(entries))
	if h.onCatalogUpdate != nil {
		h.onCatalogUpdate()
	}
	return conn.Send(&kinpb.Envelope{
		Payload: &kinpb.Envelope_CatalogAck{
			CatalogAck: &kinpb.CatalogAck{ReceivedCount: uint32(len(entries))}, //nolint:gosec
		},
	})
}

func (h *Handler) registerConn(peerID [32]byte, conn Conn) *connEntry {
	entry := &connEntry{conn: conn}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[peerID] = entry
	return entry
}

func (h *Handler) unregisterConn(peerID [32]byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, peerID)
}

// BroadcastCatalog sends the current catalog to all connected peers.
// Errors on individual connections are logged but do not stop the broadcast.
func (h *Handler) BroadcastCatalog() {
	if h.catalog == nil {
		return
	}

	h.mu.Lock()
	snapshot := make(map[[32]byte]*connEntry, len(h.conns))
	for id, ce := range h.conns {
		snapshot[id] = ce
	}
	h.mu.Unlock()

	for _, ce := range snapshot {
		if err := h.sendCatalogOfferTo(ce); err != nil {
			h.logger.Warn("broadcast catalog", "peer", ce.RemoteAddr(), "err", err)
		}
	}
}
