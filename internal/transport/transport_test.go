package transport_test

import (
	"errors"
	"testing"

	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/transport"
	"github.com/niklod/kin/kinpb"
)

func mustGenID(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return id
}

func startListener(t *testing.T, id *identity.Identity) *transport.Listener {
	t.Helper()
	ln, err := transport.Listen("127.0.0.1:0", id)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

func TestMutualTLS_Connect(t *testing.T) {
	serverID := mustGenID(t)
	clientID := mustGenID(t)
	ln := startListener(t, serverID)

	errCh := make(chan error, 1)
	go func() {
		conn, peerNodeID, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		if peerNodeID != clientID.NodeID {
			errCh <- errors.New("server: unexpected client NodeID")
			return
		}
		errCh <- nil
	}()

	conn, err := transport.Dial(ln.Addr().String(), clientID, serverID.NodeID)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	if err := <-errCh; err != nil {
		t.Errorf("server error: %v", err)
	}
	if conn.PeerNodeID != serverID.NodeID {
		t.Errorf("client PeerNodeID = %x, want %x", conn.PeerNodeID, serverID.NodeID)
	}
}

func TestDial_WrongNodeID(t *testing.T) {
	serverID := mustGenID(t)
	clientID := mustGenID(t)
	wrongID := mustGenID(t)
	ln := startListener(t, serverID)

	go func() {
		conn, _, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	_, err := transport.Dial(ln.Addr().String(), clientID, wrongID.NodeID)
	if err == nil {
		t.Fatal("Dial with wrong NodeID: expected error, got nil")
	}
}

func TestSendRecv_RoundTrip(t *testing.T) {
	serverID := mustGenID(t)
	clientID := mustGenID(t)
	ln := startListener(t, serverID)

	errCh := make(chan error, 1)
	go func() {
		conn, _, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		env, err := conn.Recv()
		if err != nil {
			errCh <- err
			return
		}
		errCh <- conn.Send(env)
	}()

	clientConn, err := transport.Dial(ln.Addr().String(), clientID, serverID.NodeID)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer clientConn.Close()

	want := &kinpb.Envelope{Payload: &kinpb.Envelope_Error{
		Error: &kinpb.Error{Code: "test", Message: "hello"},
	}}
	if err := clientConn.Send(want); err != nil {
		t.Fatalf("Send: %v", err)
	}
	got, err := clientConn.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if got.GetError().GetCode() != "test" {
		t.Errorf("received code = %q, want %q", got.GetError().GetCode(), "test")
	}
	if err := <-errCh; err != nil {
		t.Errorf("server error: %v", err)
	}
}

func TestGenerateSelfSignedCert(t *testing.T) {
	id := mustGenID(t)
	cert, err := transport.GenerateSelfSignedCert(id.PrivKey)
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Error("certificate is empty")
	}
}

func TestNodeIDFromCert_ViaConnect(t *testing.T) {
	serverID := mustGenID(t)
	clientID := mustGenID(t)
	ln := startListener(t, serverID)

	gotIDCh := make(chan [32]byte, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, peerID, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		conn.Close()
		gotIDCh <- peerID
		errCh <- nil
	}()

	conn, err := transport.Dial(ln.Addr().String(), clientID, serverID.NodeID)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	conn.Close()

	if err := <-errCh; err != nil {
		t.Fatalf("server: %v", err)
	}
	gotID := <-gotIDCh
	if gotID != clientID.NodeID {
		t.Errorf("server got NodeID %x, want %x", gotID, clientID.NodeID)
	}
}

func TestConn_RemoteAddr(t *testing.T) {
	serverID := mustGenID(t)
	clientID := mustGenID(t)
	ln := startListener(t, serverID)

	go func() {
		conn, _, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	conn, err := transport.Dial(ln.Addr().String(), clientID, serverID.NodeID)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	addr := conn.RemoteAddr()
	if addr == "" {
		t.Error("RemoteAddr: empty string")
	}
}

func TestListen_ClosedListener(t *testing.T) {
	id := mustGenID(t)
	ln := startListener(t, id)
	ln.Close()

	_, _, err := ln.Accept()
	if err == nil {
		t.Error("Accept on closed listener: expected error, got nil")
	}
}

func TestNodeIDFromCert_EmptyCerts(t *testing.T) {
	_, err := transport.NodeIDFromCert(nil)
	if err == nil {
		t.Error("NodeIDFromCert(nil): expected error, got nil")
	}
}
