package connmgr_test

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/niklod/kin/internal/connmgr"
	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/relay"
	"github.com/niklod/kin/internal/transport"
)

// startRelay starts a TLS relay server and returns its host:port.
func startRelay(t *testing.T) string {
	t.Helper()
	relayID, _ := identity.Generate()
	cert, err := transport.GenerateSelfSignedCert(relayID.PrivKey)
	if err != nil {
		t.Fatalf("relay cert: %v", err)
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert,
		MinVersion:   tls.VersionTLS13,
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatalf("relay listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	srv := relay.NewServer(nil)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			extIP, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
			go srv.Serve(conn, extIP)
		}
	}()
	return ln.Addr().String()
}

// startPeer starts a transport.Listener and returns its address and a channel of incoming Conns.
func startPeer(t *testing.T, id *identity.Identity) (addr string, conns chan *transport.Conn) {
	t.Helper()
	ln, err := transport.Listen("127.0.0.1:0", id)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	ch := make(chan *transport.Conn, 4)
	go func() {
		for {
			conn, _, err := ln.Accept()
			if err != nil {
				return
			}
			ch <- conn
		}
	}()
	return ln.Addr().String(), ch
}

func listenPort(addr string) uint32 {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return 0
	}
	return uint32(tcpAddr.Port)
}

func TestDial_Direct(t *testing.T) {
	idA, _ := identity.Generate()
	idB, _ := identity.Generate()

	bAddr, bConns := startPeer(t, idB)

	d := &connmgr.Dialer{ID: idA, LocalPort: 0}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := d.Dial(ctx, idB.NodeID, []string{bAddr})
	if err != nil {
		t.Fatalf("Dial direct: %v", err)
	}
	defer conn.Close()

	if conn.PeerNodeID != idB.NodeID {
		t.Error("wrong peer NodeID")
	}

	select {
	case c := <-bConns:
		c.Close()
	case <-ctx.Done():
		t.Fatal("timeout waiting for B to accept")
	}
}

func TestDial_NoEndpoints_ReturnsNoRoute(t *testing.T) {
	idA, _ := identity.Generate()
	d := &connmgr.Dialer{ID: idA, LocalPort: 0}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := d.Dial(ctx, [32]byte{}, nil)
	if !errors.Is(err, connmgr.ErrNoRoute) {
		t.Errorf("expected ErrNoRoute, got: %v", err)
	}
}

func TestDial_ViaRelay(t *testing.T) {
	idA, _ := identity.Generate()
	idB, _ := identity.Generate()

	relayAddr := startRelay(t)

	// B starts a listener (A will connect to it after the punch).
	bAddr, bConns := startPeer(t, idB)
	bPort := listenPort(bAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// B registers with relay and watches for incoming rendezvous.
	bRelay, err := relay.Connect(ctx, relayAddr, idB, bPort)
	if err != nil {
		t.Fatalf("B relay connect: %v", err)
	}
	defer bRelay.Close()

	// B handles incoming rendezvous by dialing directly to A (localhost — no real NAT).
	go func() {
		for rv := range bRelay.Incoming() {
			rv := rv
			go func() {
				dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				conn, err := transport.Dial(rv.PeerExternalAddr, idB, rv.PeerNodeID)
				if err != nil {
					_ = err // punch may fail on loopback; OK
					_ = dialCtx
					return
				}
				bConns <- conn
			}()
		}
	}()

	// A starts a listener to receive the return punch from B.
	aAddr, aConns := startPeer(t, idA)
	aPort := listenPort(aAddr)

	d := &connmgr.Dialer{ID: idA, LocalPort: aPort}

	// A dials B with a bad direct endpoint followed by a relay endpoint.
	endpoints := []string{
		"127.0.0.1:19997",      // bad — connection refused
		"relay://" + relayAddr, // relay fallback
	}

	conn, err := d.Dial(ctx, idB.NodeID, endpoints)
	if err != nil {
		t.Fatalf("Dial via relay: %v", err)
	}
	defer conn.Close()

	if conn.PeerNodeID != idB.NodeID {
		t.Errorf("A: wrong peer NodeID: %x", conn.PeerNodeID[:4])
	}

	// Either A's listener accepted B's return punch, or B accepted A's punch —
	// at least one connection must have arrived.
	select {
	case c := <-bConns:
		c.Close()
	case c := <-aConns:
		c.Close()
	case <-ctx.Done():
		t.Fatal("timeout: no connection arrived at either side")
	}
}

func TestServePunch(t *testing.T) {
	idA, _ := identity.Generate()
	idB, _ := identity.Generate()

	relayAddr := startRelay(t)

	// A starts a transport listener and registers with relay via ServePunch.
	// B will punch to A's external addr (= A's listener addr on loopback).
	aAddr, aConns := startPeer(t, idA)
	aPort := listenPort(aAddr)
	dA := &connmgr.Dialer{ID: idA, LocalPort: aPort}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go dA.ServePunch(ctx, relayAddr, nil) //nolint:errcheck

	// Give ServePunch time to register with relay.
	time.Sleep(150 * time.Millisecond)

	// B dials A via relay (direct endpoint is absent, so relay path is used).
	bAddr, _ := startPeer(t, idB)
	bPort := listenPort(bAddr)
	dB := &connmgr.Dialer{ID: idB, LocalPort: bPort}

	conn, err := dB.Dial(ctx, idA.NodeID, []string{"relay://" + relayAddr})
	if err != nil {
		t.Fatalf("B dial A via relay: %v", err)
	}
	defer conn.Close()

	if conn.PeerNodeID != idA.NodeID {
		t.Errorf("B: wrong peer NodeID, got %x", conn.PeerNodeID[:4])
	}

	// A's listener should have accepted B's punch as a normal incoming connection.
	select {
	case c := <-aConns:
		if c.PeerNodeID != idB.NodeID {
			t.Errorf("A: wrong peer NodeID")
		}
		c.Close()
	case <-ctx.Done():
		t.Fatal("timeout: A never received B's connection")
	}
}
