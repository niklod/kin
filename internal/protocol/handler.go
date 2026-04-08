package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

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
}

// NewHandler creates a Handler backed by the given file sender and optional
// catalog exchanger. If catalog is nil, catalog exchange is disabled.
func NewHandler(sender *transfer.Sender, cat CatalogExchanger, selfID [32]byte, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{sender: sender, catalog: cat, selfID: selfID, logger: logger}
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
	if h.catalog != nil {
		if err := h.sendCatalogOffer(conn); err != nil {
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
		if err := h.dispatch(ctx, env, conn); err != nil {
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

func (h *Handler) sendCatalogOffer(conn Conn) error {
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

