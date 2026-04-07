// Kin — P2P shared folder.
//
// Subcommands:
//
//	kin run                 Start the node daemon
//	kin invite              Generate a one-time invite link
//	kin join <kin:token>    Join via an invite link
//	kin status              Show node identity and peer count
package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/niklod/kin/internal/catalog"
	"github.com/niklod/kin/internal/config"
	"github.com/niklod/kin/internal/connmgr"
	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/invite"
	"github.com/niklod/kin/internal/peerstore"
	"github.com/niklod/kin/internal/protocol"
	"github.com/niklod/kin/internal/transfer"
	"github.com/niklod/kin/internal/transport"
	"github.com/niklod/kin/internal/watcher"
	"github.com/niklod/kin/kinpb"
)

const defaultListenAddr = "0.0.0.0:7777"

func main() {
	configDir := flag.String("config-dir", "", "override config directory")
	sharedDir := flag.String("shared-dir", "", "override shared folder directory")
	listenAddr := flag.String("listen", defaultListenAddr, "address to listen on")
	relayAddr := flag.String("relay", "", "relay server address (host:port) for NAT traversal")
	debug := flag.Bool("debug", false, "enable verbose debug logging")
	flag.Parse()

	if *debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	cfgDir, err := resolveConfigDir(*configDir)
	if err != nil {
		fatalf("config dir: %v", err)
	}

	switch args[0] {
	case "run":
		cmdRun(cfgDir, *sharedDir, *listenAddr, *relayAddr)
	case "invite":
		cmdInvite(cfgDir, *listenAddr, *relayAddr)
	case "join":
		if len(args) < 2 {
			fatalf("usage: kin join <kin:token>")
		}
		cmdJoin(cfgDir, args[1], *listenAddr)
	case "status":
		cmdStatus(cfgDir)
	default:
		usage()
		os.Exit(1)
	}
}

func resolveConfigDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return config.DefaultConfigDir()
}

func resolveSharedDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return config.DefaultSharedDir()
}

func cmdRun(cfgDir, sharedDirOverride, listenAddr, relayAddr string) {
	id := mustLoadOrGenerate(cfgDir)
	store := mustOpenStore(cfgDir)
	defer store.Close()

	sharedDir, err := resolveSharedDir(sharedDirOverride)
	if err != nil {
		fatalf("shared dir: %v", err)
	}
	if err := os.MkdirAll(sharedDir, 0755); err != nil {
		fatalf("create shared dir: %v", err)
	}

	cat := mustOpenCatalog(cfgDir, id.NodeID)
	defer cat.Close()

	idx := transfer.NewLocalIndex()
	if err := idx.Scan(sharedDir); err != nil {
		slog.Warn("scan shared dir", "err", err)
	}

	sender := transfer.NewSender(idx)
	handler := protocol.NewHandler(sender, cat, id.NodeID, slog.Default())

	ln, err := transport.Listen(listenAddr, id)
	if err != nil {
		fatalf("listen: %v", err)
	}
	defer ln.Close()

	fmt.Printf("kin running\n")
	fmt.Printf("  NodeID: %s\n", id.NodeIDHex())
	fmt.Printf("  Listen: %s\n", ln.Addr())
	fmt.Printf("  Shared: %s\n", sharedDir)
	if relayAddr != "" {
		fmt.Printf("  Relay:  %s\n", relayAddr)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start file watcher for the shared folder.
	go func() {
		w := watcher.New(sharedDir, cat, idx, slog.Default())
		if err := w.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("watcher", "err", err)
		}
	}()

	// Accept incoming connections from the transport listener.
	go func() {
		for {
			conn, _, err := ln.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					slog.Warn("accept", "err", err)
					continue
				}
			}
			if err := store.UpdateLastSeen(conn.PeerNodeID); err != nil {
				slog.Warn("update last seen", "peer", fmt.Sprintf("%x", conn.PeerNodeID[:8]), "err", err)
			}
			go handler.Serve(ctx, conn)
		}
	}()

	// Register with relay and keep the connection alive so remote peers can
	// discover our external address for NAT hole punching.
	if relayAddr != "" {
		d := &connmgr.Dialer{ID: id, Listener: ln}
		go func() {
			backoff := time.Second
			const maxBackoff = 30 * time.Second
			for {
				err := d.ServePunch(ctx, relayAddr, slog.Default())
				if ctx.Err() != nil {
					return
				}
				slog.Warn("relay connection lost, reconnecting", "err", err, "backoff", backoff)
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return
				}
				if backoff < maxBackoff {
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
				}
			}
		}()
	}

	<-ctx.Done()
	fmt.Println("\nshutting down")
}

func cmdInvite(cfgDir, listenAddr, relayAddr string) {
	id := mustLoadOrGenerate(cfgDir)

	var endpoints []string
	if host, _, err := net.SplitHostPort(listenAddr); err == nil && host != "0.0.0.0" && host != "" {
		endpoints = append(endpoints, listenAddr)
	}
	if relayAddr != "" {
		endpoints = append(endpoints, "relay://"+relayAddr)
	}

	tok, err := invite.Create(id.PrivKey, endpoints, invite.DefaultTTL)
	if err != nil {
		fatalf("create invite: %v", err)
	}
	raw, err := invite.Encode(tok, id.PrivKey)
	if err != nil {
		fatalf("encode invite: %v", err)
	}
	fmt.Println(raw)
}

func cmdJoin(cfgDir, rawToken, listenAddr string) {
	id := mustLoadOrGenerate(cfgDir)
	store := mustOpenStore(cfgDir)
	defer store.Close()

	tok, err := invite.Decode(rawToken)
	if err != nil {
		fatalf("decode invite: %v", err)
	}
	if err := invite.Validate(tok, rawToken, store); err != nil {
		fatalf("validate invite: %v", err)
	}

	if len(tok.PublicKey) != 32 {
		fatalf("invalid invite: unexpected public key length %d", len(tok.PublicKey))
	}
	peerNodeID := sha256.Sum256(tok.PublicKey)

	if len(tok.Endpoints) == 0 {
		fatalf("invite has no endpoints")
	}

	// Open a listener so Dial can share the UDP socket for NAT punch.
	// Use --listen 0.0.0.0:0 (or a different port) if kin run is already
	// binding the default port on this host.
	ln, err := transport.Listen(listenAddr, id)
	if err != nil {
		fatalf("listen: %v", err)
	}
	defer ln.Close()
	// Drain any inbound QUIC attempts that arrive during the join window.
	go func() {
		for {
			c, _, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	d := &connmgr.Dialer{ID: id, Listener: ln}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("connecting to peer %x...\n", peerNodeID[:8])
	conn, err := d.Dial(ctx, peerNodeID, tok.Endpoints)
	if err != nil {
		fatalf("connect: %v", err)
	}
	defer conn.Close()

	peerID := conn.PeerNodeID
	if err := store.PutPeer(&peerstore.Peer{
		NodeID:     peerID,
		PublicKey:  tok.PublicKey,
		Endpoints:  tok.Endpoints,
		TrustState: peerstore.TrustTOFU,
	}); err != nil {
		slog.Warn("put peer", "err", err)
	}

	// Exchange catalogs with the peer.
	cat := mustOpenCatalog(cfgDir, id.NodeID)
	defer cat.Close()
	exchangeCatalog(cat, conn, peerID)

	fmt.Printf("connected to %x\n", peerID[:8])
}

func cmdStatus(cfgDir string) {
	id := mustLoadOrGenerate(cfgDir)
	fmt.Printf("NodeID: %s\n", id.NodeIDHex())

	store, err := peerstore.OpenReadOnly(filepath.Join(cfgDir, "peers.db"))
	if err != nil {
		fmt.Printf("Peers:  (unavailable while kin run is active)\n")
		return
	}
	defer store.Close()

	peers, err := store.ListPeers()
	if err != nil {
		fatalf("list peers: %v", err)
	}
	fmt.Printf("Peers:  %d\n", len(peers))
	for _, p := range peers {
		fmt.Printf("  %x  %s  %s\n", p.NodeID[:8], p.TrustState, p.LastSeen.Format("2006-01-02 15:04:05"))
	}
}

// exchangeCatalog sends our catalog to the peer and receives theirs.
func exchangeCatalog(cat *catalog.Store, conn *transport.Conn, peerID [32]byte) {
	if err := sendCatalogOffer(cat, conn, peerID); err != nil {
		slog.Warn("send catalog offer", "err", err)
		return
	}
	receivePeerCatalog(cat, conn, peerID)
}

func sendCatalogOffer(cat *catalog.Store, conn *transport.Conn, peerID [32]byte) error {
	entries, err := cat.ListForPeer(peerID)
	if err != nil {
		return fmt.Errorf("list for peer: %w", err)
	}

	files := catalog.EntriesToProto(entries)
	slog.Debug("sent catalog offer", "files", len(files))
	return conn.Send(&kinpb.Envelope{
		Payload: &kinpb.Envelope_CatalogOffer{
			CatalogOffer: &kinpb.CatalogOffer{Files: files},
		},
	})
}

func receivePeerCatalog(cat *catalog.Store, conn *transport.Conn, peerID [32]byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for {
		if ctx.Err() != nil {
			slog.Debug("catalog exchange timeout")
			return
		}
		env, err := conn.Recv()
		if err != nil {
			slog.Debug("catalog exchange recv", "err", err)
			return
		}
		switch p := env.Payload.(type) {
		case *kinpb.Envelope_CatalogOffer:
			handleReceivedOffer(cat, conn, peerID, p.CatalogOffer)
			return
		case *kinpb.Envelope_CatalogAck:
			slog.Debug("catalog ack", "count", p.CatalogAck.ReceivedCount)
		default:
			slog.Debug("unexpected message during catalog exchange", "type", fmt.Sprintf("%T", env.Payload))
			return
		}
	}
}

func handleReceivedOffer(cat *catalog.Store, conn *transport.Conn, peerID [32]byte, offer *kinpb.CatalogOffer) {
	entries := make([]*catalog.Entry, 0, len(offer.Files))
	for _, f := range offer.Files {
		e, err := catalog.ProtoToEntry(f)
		if err != nil {
			slog.Debug("skip bad catalog entry", "err", err)
			continue
		}
		entries = append(entries, e)
	}

	if err := cat.PutPeerEntries(peerID, entries); err != nil {
		slog.Warn("save peer catalog", "err", err)
	}
	slog.Debug("received peer catalog", "entries", len(entries))

	if err := conn.Send(&kinpb.Envelope{
		Payload: &kinpb.Envelope_CatalogAck{
			CatalogAck: &kinpb.CatalogAck{ReceivedCount: uint32(len(entries))}, //nolint:gosec
		},
	}); err != nil {
		slog.Warn("send catalog ack", "err", err)
	}
}

func mustLoadOrGenerate(cfgDir string) *identity.Identity {
	id, err := identity.LoadOrGenerate(cfgDir)
	if err != nil {
		fatalf("identity: %v", err)
	}
	return id
}

func mustOpenStore(cfgDir string) *peerstore.Store {
	s, err := peerstore.Open(filepath.Join(cfgDir, "peers.db"))
	if err != nil {
		fatalf("peerstore: %v", err)
	}
	return s
}

func mustOpenCatalog(cfgDir string, selfID [32]byte) *catalog.Store {
	s, err := catalog.Open(filepath.Join(cfgDir, "catalog.db"), selfID)
	if err != nil {
		fatalf("catalog: %v", err)
	}
	return s
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "kin: "+format+"\n", args...)
	os.Exit(1)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: kin [flags] <command>")
	fmt.Fprintln(os.Stderr, "commands: run, invite, join <token>, status")
	flag.PrintDefaults()
}
