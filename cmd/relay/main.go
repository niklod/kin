// Relay — Kin signaling server.
//
// Usage:
//
//	relay [--listen addr] [--config-dir dir]
//
// The relay server registers connected nodes and coordinates NAT hole-punching
// rendezvous. It never forwards application traffic.
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/niklod/kin/internal/config"
	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/relay"
	"github.com/niklod/kin/internal/transport"
)

func main() {
	listenAddr := flag.String("listen", "0.0.0.0:7778", "relay listen address")
	configDir := flag.String("config-dir", "", "config directory for relay identity")
	flag.Parse()

	cfgDir, err := resolveConfigDir(*configDir)
	if err != nil {
		fatalf("config dir: %v", err)
	}

	id, err := identity.LoadOrGenerate(cfgDir)
	if err != nil {
		fatalf("identity: %v", err)
	}

	cert, err := transport.GenerateSelfSignedCert(id.PrivKey)
	if err != nil {
		fatalf("generate cert: %v", err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		// Clients connect without verifying relay's NodeID.
		// Relay identity is not part of the trust model.
		ClientAuth: tls.NoClientCert,
		MinVersion: tls.VersionTLS13,
	}

	ln, err := tls.Listen("tcp", *listenAddr, tlsCfg)
	if err != nil {
		fatalf("listen: %v", err)
	}
	defer ln.Close()

	srv := relay.NewServer(slog.Default())

	fmt.Printf("relay running\n")
	fmt.Printf("  NodeID: %s\n", id.NodeIDHex())
	fmt.Printf("  Listen: %s\n", *listenAddr)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-stop:
					return
				default:
					slog.Warn("relay accept", "err", err)
					continue
				}
			}
			extIP := remoteIP(conn)
			go srv.Serve(conn, extIP)
		}
	}()

	<-stop
	fmt.Println("\nshutting down")
}

func remoteIP(conn net.Conn) string {
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return conn.RemoteAddr().String()
	}
	return host
}

func resolveConfigDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	d, err := config.DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return d + "-relay", nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "relay: "+format+"\n", args...)
	os.Exit(1)
}
