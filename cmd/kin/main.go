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

	"github.com/niklod/kin/internal/config"
	"github.com/niklod/kin/internal/connmgr"
	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/invite"
	"github.com/niklod/kin/internal/peerstore"
	"github.com/niklod/kin/internal/protocol"
	"github.com/niklod/kin/internal/transfer"
	"github.com/niklod/kin/internal/transport"
)

const defaultListenAddr = "0.0.0.0:7777"

func main() {
	configDir := flag.String("config-dir", "", "override config directory")
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
		cmdRun(cfgDir, *listenAddr, *relayAddr)
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

func cmdRun(cfgDir, listenAddr, relayAddr string) {
	id := mustLoadOrGenerate(cfgDir)
	store := mustOpenStore(cfgDir)
	defer store.Close()

	sharedDir, err := config.DefaultSharedDir()
	if err != nil {
		fatalf("shared dir: %v", err)
	}
	if err := os.MkdirAll(sharedDir, 0755); err != nil {
		fatalf("create shared dir: %v", err)
	}

	idx := transfer.NewLocalIndex()
	if err := idx.Scan(sharedDir); err != nil {
		slog.Warn("scan shared dir", "err", err)
	}

	sender := transfer.NewSender(idx)
	handler := protocol.NewHandler(sender, slog.Default())

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

	fmt.Printf("connected to %x\n", peerID[:8])
}

func cmdStatus(cfgDir string) {
	id := mustLoadOrGenerate(cfgDir)
	store := mustOpenStore(cfgDir)
	defer store.Close()

	peers, err := store.ListPeers()
	if err != nil {
		fatalf("list peers: %v", err)
	}

	fmt.Printf("NodeID: %s\n", id.NodeIDHex())
	fmt.Printf("Peers:  %d\n", len(peers))
	for _, p := range peers {
		fmt.Printf("  %x  %s  %s\n", p.NodeID[:8], p.TrustState, p.LastSeen.Format("2006-01-02 15:04:05"))
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

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "kin: "+format+"\n", args...)
	os.Exit(1)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: kin [flags] <command>")
	fmt.Fprintln(os.Stderr, "commands: run, invite, join <token>, status")
	flag.PrintDefaults()
}
