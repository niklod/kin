package transport_test

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
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
	ln, err := transport.Listen("127.0.0.1:0", id)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ln.Close()

	_, _, err = ln.Accept()
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

func TestListener_Port_MatchesAddr(t *testing.T) {
	id := mustGenID(t)
	ln := startListener(t, id)

	addrStr := ln.Addr().String()
	_, portStr, err := net.SplitHostPort(addrStr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addrStr, err)
	}

	port64, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		t.Fatalf("ParseUint(%q): %v", portStr, err)
	}
	addrPort := uint16(port64)

	if ln.Port() != addrPort {
		t.Errorf("Port() = %d, addr port = %d", ln.Port(), addrPort)
	}
}

func TestListener_Dial_SharedTransport(t *testing.T) {
	idA := mustGenID(t)
	idB := mustGenID(t)
	lnA := startListener(t, idA)
	lnB := startListener(t, idB)

	errCh := make(chan error, 1)
	go func() {
		conn, peerID, err := lnB.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		if peerID != idA.NodeID {
			errCh <- errors.New("lnB: unexpected peer NodeID")
			return
		}
		errCh <- nil
	}()

	ctx := context.Background()
	connA, err := lnA.Dial(ctx, lnB.Addr().String(), idB.NodeID)
	if err != nil {
		t.Fatalf("lnA.Dial: %v", err)
	}
	defer connA.Close()

	if connA.PeerNodeID != idB.NodeID {
		t.Errorf("PeerNodeID = %x, want %x", connA.PeerNodeID, idB.NodeID)
	}
	if err := <-errCh; err != nil {
		t.Errorf("server error: %v", err)
	}
}

func TestListener_Prime_NoError(t *testing.T) {
	id := mustGenID(t)
	ln := startListener(t, id)

	// Prime to a port that has nothing listening — should not error.
	// (UDP writes to closed ports succeed at the socket level.)
	if err := ln.Prime("127.0.0.1:19999"); err != nil {
		t.Errorf("Prime: unexpected error: %v", err)
	}
}

func TestConn_Close_PropagatesToPeer(t *testing.T) {
	serverID := mustGenID(t)
	clientID := mustGenID(t)
	ln := startListener(t, serverID)

	recvErrCh := make(chan error, 1)
	go func() {
		conn, _, err := ln.Accept()
		if err != nil {
			recvErrCh <- err
			return
		}
		_, err = conn.Recv()
		recvErrCh <- err
	}()

	clientConn, err := transport.Dial(ln.Addr().String(), clientID, serverID.NodeID)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// Close the client — server's Recv should return an error, not hang.
	clientConn.Close()

	recvErr := <-recvErrCh
	if recvErr == nil {
		t.Error("server Recv: expected error after peer closed, got nil")
	}
}

func TestListener_Addr_IsUDP(t *testing.T) {
	id := mustGenID(t)
	ln := startListener(t, id)

	addr := ln.Addr().String()
	if !strings.Contains(addr, ":") {
		t.Errorf("Addr = %q, expected host:port format", addr)
	}
}
