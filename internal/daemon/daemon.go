package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/niklod/kin/internal/catalog"
	"github.com/niklod/kin/internal/config"
	"github.com/niklod/kin/internal/connmgr"
	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/peerstore"
	"github.com/niklod/kin/internal/protocol"
	"github.com/niklod/kin/internal/transfer"
	"github.com/niklod/kin/internal/transport"
	"github.com/niklod/kin/internal/watcher"
)

// Daemon is the long-running kin process that manages connections, file
// watching, and exposes a Unix socket for CLI/TUI control.
type Daemon struct {
	cfgDir     string
	sharedDir  string
	listenAddr string
	relayAddr  string
	logger     *slog.Logger

	// Embedded suppresses stdout output (for use when TUI owns the terminal).
	Embedded bool
	// ReadyCh is closed when the daemon is fully initialized and accepting IPC.
	// Callers should create this channel before calling Run if they need to wait.
	ReadyCh chan struct{}
}

// New creates a Daemon with the given configuration.
func New(cfgDir, sharedDir, listenAddr, relayAddr string, logger *slog.Logger) *Daemon {
	return &Daemon{
		cfgDir:     cfgDir,
		sharedDir:  sharedDir,
		listenAddr: listenAddr,
		relayAddr:  relayAddr,
		logger:     logger,
	}
}

// Run starts the daemon and blocks until ctx is cancelled. It initializes all
// subsystems (identity, stores, transport, watcher) and opens the IPC socket.
func (d *Daemon) Run(ctx context.Context) error {
	id, err := identity.LoadOrGenerate(d.cfgDir)
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}

	store, err := peerstore.Open(filepath.Join(d.cfgDir, "peers.db"))
	if err != nil {
		return fmt.Errorf("peerstore: %w", err)
	}
	defer store.Close()

	if err := os.MkdirAll(d.sharedDir, 0755); err != nil {
		return fmt.Errorf("create shared dir: %w", err)
	}

	cat, err := catalog.Open(filepath.Join(d.cfgDir, "catalog.db"), id.NodeID)
	if err != nil {
		return fmt.Errorf("catalog: %w", err)
	}
	defer cat.Close()

	idx := transfer.NewLocalIndex()
	if err := idx.Scan(d.sharedDir); err != nil {
		d.logger.Warn("scan shared dir", "err", err)
	}

	sender := transfer.NewSender(idx)
	protoHandler := protocol.NewHandler(sender, cat, id.NodeID, d.logger)

	ln, err := transport.Listen(d.listenAddr, id)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer ln.Close()

	dialer := &connmgr.Dialer{ID: id, Listener: ln}

	// Build IPC handler first so it's ready before the server starts.
	ipcHandler := NewHandler(HandlerConfig{
		ID:        id,
		Store:     store,
		Catalog:   cat,
		Proto:     protoHandler,
		Listener:  ln,
		Dialer:    dialer,
		SharedDir: d.sharedDir,
		RelayAddr: d.relayAddr,
		Ctx:       ctx,
		Logger:    d.logger,
	})

	sockPath := config.SocketPath(d.cfgDir)
	srv, err := NewServer(sockPath, ipcHandler, d.logger)
	if err != nil {
		return fmt.Errorf("daemon socket: %w", err)
	}
	ipcHandler.server = srv

	// When peer catalog arrives: notify IPC subscribers (TUI) only.
	// Do NOT re-broadcast to peers here — that would create a ping-pong loop
	// (A sends to B, B's onCatalogUpdate sends back to A, repeat forever).
	protoHandler.SetOnCatalogUpdate(srv.BroadcastCatalogUpdated)

	d.logger.Info("kin running",
		"node_id", id.NodeIDHex(),
		"listen", ln.Addr().String(),
		"shared", d.sharedDir,
		"relay", d.relayAddr,
		"socket", sockPath,
	)

	// Startup banner on stdout — E2E tests match this pattern for readiness.
	// Suppressed in embedded mode (TUI owns the terminal).
	if !d.Embedded {
		fmt.Printf("kin running\n")
		fmt.Printf("  NodeID: %s\n", id.NodeIDHex())
		fmt.Printf("  Listen: %s\n", ln.Addr())
		fmt.Printf("  Shared: %s\n", d.sharedDir)
		if d.relayAddr != "" {
			fmt.Printf("  Relay:  %s\n", d.relayAddr)
		}
	}

	go srv.Serve(ctx)

	// Start file watcher. Notifies IPC subscribers (TUI) and pushes updated catalog to connected peers.
	go func() {
		onChange := func() {
			srv.BroadcastCatalogUpdated()
			protoHandler.BroadcastCatalog()
		}
		w := watcher.New(d.sharedDir, cat, idx, d.logger, onChange)
		if err := w.Run(ctx); err != nil && ctx.Err() == nil {
			d.logger.Error("watcher", "err", err)
		}
	}()

	// Accept incoming P2P connections.
	go d.acceptLoop(ctx, ln, store, protoHandler, srv)

	// Maintain relay connection if configured.
	if d.relayAddr != "" {
		go d.relayLoop(ctx, dialer)
	}

	// Signal readiness to callers waiting for initialization.
	if d.ReadyCh != nil {
		close(d.ReadyCh)
	}

	<-ctx.Done()
	d.logger.Info("shutting down")
	srv.Close()
	return nil
}

func (d *Daemon) acceptLoop(
	ctx context.Context,
	ln *transport.Listener,
	store *peerstore.Store,
	handler *protocol.Handler,
	srv *Server,
) {
	for {
		conn, _, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				d.logger.Warn("accept", "err", err)
				continue
			}
		}

		peerID := conn.PeerNodeID
		if err := store.UpdateLastSeen(peerID); err != nil {
			d.logger.Warn("update last seen", "peer", hexNodeID(peerID)[:16], "err", err)
		}

		srv.BroadcastPeerOnline(peerID)

		go func() {
			defer conn.Close()
			handler.Serve(ctx, conn)
			srv.BroadcastPeerOffline(peerID)
		}()
	}
}

func (d *Daemon) relayLoop(ctx context.Context, dialer *connmgr.Dialer) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		err := dialer.ServePunch(ctx, d.relayAddr, d.logger)
		if ctx.Err() != nil {
			return
		}
		d.logger.Warn("relay connection lost, reconnecting", "err", err, "backoff", backoff)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		backoff = min(backoff*2, maxBackoff)
	}
}
