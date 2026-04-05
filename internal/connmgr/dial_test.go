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

func mustGenID(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("identity.Generate: %v", err)
	}
	return id
}

// startRelay starts a TLS relay server and returns its host:port.
func startRelay(t *testing.T) string {
	t.Helper()
	relayID := mustGenID(t)
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

// startPeer starts a transport.Listener and returns it alongside a channel of incoming Conns.
func startPeer(t *testing.T, id *identity.Identity) (*transport.Listener, chan *transport.Conn) {
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
	return ln, ch
}

func TestDial_Direct(t *testing.T) {
	idA := mustGenID(t)
	idB := mustGenID(t)

	lnA, _ := startPeer(t, idA)
	lnB, bConns := startPeer(t, idB)

	d := &connmgr.Dialer{ID: idA, Listener: lnA}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := d.Dial(ctx, idB.NodeID, []string{lnB.Addr().String()})
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
	idA := mustGenID(t)
	lnA, _ := startPeer(t, idA)
	d := &connmgr.Dialer{ID: idA, Listener: lnA}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := d.Dial(ctx, [32]byte{}, nil)
	if !errors.Is(err, connmgr.ErrNoRoute) {
		t.Errorf("expected ErrNoRoute, got: %v", err)
	}
}

func TestDial_ViaRelay(t *testing.T) {
	idA := mustGenID(t)
	idB := mustGenID(t)

	relayAddr := startRelay(t)

	lnB, bConns := startPeer(t, idB)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bRelay, err := relay.Connect(ctx, relayAddr, idB, uint32(lnB.Port()))
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
				conn, err := lnB.Dial(dialCtx, rv.PeerExternalAddr, rv.PeerNodeID)
				if err != nil {
					return
				}
				bConns <- conn
			}()
		}
	}()

	lnA, aConns := startPeer(t, idA)
	d := &connmgr.Dialer{ID: idA, Listener: lnA}

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
	idA := mustGenID(t)
	idB := mustGenID(t)

	relayAddr := startRelay(t)

	lnA, aConns := startPeer(t, idA)
	dA := &connmgr.Dialer{ID: idA, Listener: lnA}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go dA.ServePunch(ctx, relayAddr, nil) //nolint:errcheck

	// Give ServePunch time to register with relay.
	time.Sleep(150 * time.Millisecond)

	lnB, _ := startPeer(t, idB)
	dB := &connmgr.Dialer{ID: idB, Listener: lnB}

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
