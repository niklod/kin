package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/niklod/kin/internal/catalog"
	"github.com/niklod/kin/internal/connmgr"
	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/invite"
	"github.com/niklod/kin/internal/peerstore"
	"github.com/niklod/kin/internal/protocol"
	"github.com/niklod/kin/internal/transport"
)

// Handler dispatches IPC requests to the appropriate daemon operations.
type Handler struct {
	id       *identity.Identity
	store    *peerstore.Store
	catalog  *catalog.Store
	proto    *protocol.Handler
	listener *transport.Listener
	dialer   *connmgr.Dialer
	server   *Server

	sharedDir string
	relayAddr string
	ctx       context.Context
	logger    *slog.Logger
}

// HandlerConfig holds the dependencies needed by Handler.
type HandlerConfig struct {
	ID        *identity.Identity
	Store     *peerstore.Store
	Catalog   *catalog.Store
	Proto     *protocol.Handler
	Listener  *transport.Listener
	Dialer    *connmgr.Dialer
	SharedDir string
	RelayAddr string
	Ctx       context.Context
	Logger    *slog.Logger
}

// NewHandler creates a new Handler with the given dependencies.
// The server field must be set via h.server = srv before Serve starts.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		id:        cfg.ID,
		store:     cfg.Store,
		catalog:   cfg.Catalog,
		proto:     cfg.Proto,
		listener:  cfg.Listener,
		dialer:    cfg.Dialer,
		sharedDir: cfg.SharedDir,
		relayAddr: cfg.RelayAddr,
		ctx:       cfg.Ctx,
		logger:    cfg.Logger,
	}
}

// Handle dispatches a request to the appropriate method handler.
func (h *Handler) Handle(req Request) Response {
	switch req.Method {
	case MethodStatus:
		return h.handleStatus(req.ID)
	case MethodPeers:
		return h.handlePeers(req.ID)
	case MethodInvite:
		return h.handleInvite(req.ID)
	case MethodJoin:
		return h.handleJoin(req.ID, req.Params)
	case MethodCatalog:
		return h.handleCatalog(req.ID)
	case MethodDownload:
		return h.handleDownload(req.ID, req.Params)
	default:
		return errResponse(req.ID, "unknown_method", fmt.Sprintf("unknown method: %s", req.Method))
	}
}

func (h *Handler) handleStatus(id uint64) Response {
	peers, err := h.store.ListPeers()
	if err != nil {
		return errResponse(id, "internal", fmt.Sprintf("list peers: %v", err))
	}

	nodeID := h.id.NodeIDHex()
	resp := StatusResponse{
		NodeID:      nodeID,
		NodeIDShort: shortID(nodeID),
		ListenAddr:  h.listener.Addr().String(),
		SharedDir:   h.sharedDir,
		PeerCount:   len(peers),
		RelayAddr:   h.relayAddr,
		RelayOnline: h.relayAddr != "",
	}
	return okResponse(id, resp)
}

func (h *Handler) handlePeers(id uint64) Response {
	peers, err := h.store.ListPeers()
	if err != nil {
		return errResponse(id, "internal", fmt.Sprintf("list peers: %v", err))
	}

	infos := make([]PeerInfo, 0, len(peers))
	for _, p := range peers {
		nodeID := hexNodeID(p.NodeID)
		infos = append(infos, PeerInfo{
			NodeID:      nodeID,
			NodeIDShort: shortID(nodeID),
			TrustState:  string(p.TrustState),
			LastSeen:    p.LastSeen.Format(time.RFC3339),
			Endpoints:   p.Endpoints,
		})
	}
	return okResponse(id, PeersResponse{Peers: infos})
}

func (h *Handler) handleInvite(id uint64) Response {
	var endpoints []string
	if addr := h.listener.Addr().String(); addr != "" && isRoutableListenAddr(addr) {
		endpoints = append(endpoints, addr)
	}
	if h.relayAddr != "" {
		endpoints = append(endpoints, "relay://"+h.relayAddr)
	}

	tok, err := invite.Create(h.id.PrivKey, endpoints, invite.DefaultTTL)
	if err != nil {
		return errResponse(id, "internal", fmt.Sprintf("create invite: %v", err))
	}
	raw, err := invite.Encode(tok, h.id.PrivKey)
	if err != nil {
		return errResponse(id, "internal", fmt.Sprintf("encode invite: %v", err))
	}
	return okResponse(id, InviteResponse{Token: raw})
}

func (h *Handler) handleJoin(id uint64, params json.RawMessage) Response {
	var p JoinParams
	if err := json.Unmarshal(params, &p); err != nil {
		return errResponse(id, "invalid_params", fmt.Sprintf("decode params: %v", err))
	}
	if p.Token == "" {
		return errResponse(id, "invalid_params", "token is required")
	}

	tok, err := invite.Decode(p.Token)
	if err != nil {
		return errResponse(id, "invalid_token", fmt.Sprintf("decode invite: %v", err))
	}
	if err := invite.Validate(tok, p.Token, h.store); err != nil {
		return errResponse(id, "invalid_token", fmt.Sprintf("validate invite: %v", err))
	}

	if len(tok.PublicKey) != 32 {
		return errResponse(id, "invalid_token", fmt.Sprintf("unexpected public key length %d", len(tok.PublicKey)))
	}
	peerNodeID := sha256.Sum256(tok.PublicKey)

	if len(tok.Endpoints) == 0 {
		return errResponse(id, "invalid_token", "invite has no endpoints")
	}

	dialCtx, dialCancel := context.WithTimeout(h.ctx, 30*time.Second)
	defer dialCancel()

	conn, err := h.dialer.Dial(dialCtx, peerNodeID, tok.Endpoints)
	if err != nil {
		return errResponse(id, "connect_failed", fmt.Sprintf("connect: %v", err))
	}

	actualPeerID := conn.PeerNodeID
	if err := h.store.PutPeer(&peerstore.Peer{
		NodeID:     actualPeerID,
		PublicKey:  tok.PublicKey,
		Endpoints:  tok.Endpoints,
		TrustState: peerstore.TrustTOFU,
	}); err != nil {
		h.logger.Warn("put peer after join", "err", err)
	}

	// Serve the new connection in the background.
	// protocol.Handler.Serve sends our catalog offer and handles incoming messages.
	go h.serveJoinedPeer(conn, actualPeerID)

	return okResponse(id, JoinResponse{PeerNodeID: hexNodeID(actualPeerID)})
}

func (h *Handler) serveJoinedPeer(conn *transport.Conn, peerID [32]byte) {
	defer conn.Close()

	shortPeer := hexNodeID(peerID)[:16]
	h.logger.Info("peer joined via daemon", "peer", shortPeer)

	if h.server != nil {
		h.server.BroadcastPeerOnline(peerID)
	}

	h.proto.Serve(h.ctx, conn)

	if h.server != nil {
		h.server.BroadcastPeerOffline(peerID)
	}
	h.logger.Info("peer disconnected", "peer", shortPeer)
}

func (h *Handler) handleCatalog(id uint64) Response {
	entries, err := h.catalog.ListAll()
	if err != nil {
		return errResponse(id, "internal", fmt.Sprintf("list catalog: %v", err))
	}

	files := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		files = append(files, FileInfo{
			FileID:      hexNodeID(e.FileID),
			Name:        e.Name,
			Size:        e.Size,
			ModTime:     e.ModTime.Format(time.RFC3339),
			OwnerNodeID: hexNodeID(e.OwnerNodeID),
			IsLocal:     e.OwnerNodeID == h.id.NodeID,
		})
	}
	return okResponse(id, CatalogResponse{Files: files})
}

func (h *Handler) handleDownload(id uint64, params json.RawMessage) Response {
	var p DownloadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return errResponse(id, "invalid_params", fmt.Sprintf("decode params: %v", err))
	}
	if p.FileID == "" {
		return errResponse(id, "invalid_params", "file_id is required")
	}

	// TODO: wire up download.Downloader with a PeerDialer adapter.
	return errResponse(id, "not_implemented", "download via daemon is not yet implemented")
}

func shortID(hex string) string {
	if len(hex) > 16 {
		return hex[:16]
	}
	return hex
}

// isRoutableListenAddr returns false for wildcard listen addresses like
// [::]:7777 or 0.0.0.0:7777 that are not useful as direct dial targets.
// Loopback addresses (127.0.0.1, ::1) pass — they are valid for local testing.
func isRoutableListenAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && !ip.IsUnspecified()
}
