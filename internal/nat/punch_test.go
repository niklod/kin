package nat_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/nat"
	"github.com/niklod/kin/internal/transport"
)

// startListener starts a transport.Listener on a random port and returns its
// address plus a channel that produces the first accepted Conn.
func startListener(t *testing.T, id *identity.Identity) (addr string, conns <-chan *transport.Conn) {
	t.Helper()
	ln, err := transport.Listen("127.0.0.1:0", id)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	ch := make(chan *transport.Conn, 1)
	go func() {
		conn, _, err := ln.Accept()
		if err != nil {
			return
		}
		ch <- conn
	}()
	return ln.Addr().String(), ch
}

func TestPunch_ConnectsAndAuthenticated(t *testing.T) {
	idA, _ := identity.Generate()
	idB, _ := identity.Generate()

	// B listens; A punches to B with localPort=0 (OS picks).
	bAddr, bConns := startListener(t, idB)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connA, err := nat.Punch(ctx, 0, bAddr, idA, idB.NodeID)
	if err != nil {
		t.Fatalf("Punch: %v", err)
	}
	defer connA.Close()

	if connA.PeerNodeID != idB.NodeID {
		t.Errorf("A: PeerNodeID mismatch")
	}

	select {
	case connB := <-bConns:
		if connB.PeerNodeID != idA.NodeID {
			t.Errorf("B: PeerNodeID mismatch")
		}
		connB.Close()
	case <-ctx.Done():
		t.Fatal("timeout waiting for B to accept")
	}
}

func TestPunch_WrongNodeIDRejected(t *testing.T) {
	idA, _ := identity.Generate()
	idB, _ := identity.Generate()
	wrong, _ := identity.Generate()

	bAddr, _ := startListener(t, idB)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := nat.Punch(ctx, 0, bAddr, idA, wrong.NodeID)
	if err == nil {
		t.Fatal("expected error for wrong NodeID, got nil")
	}
	if !errors.Is(err, nat.ErrPunchFailed) {
		t.Errorf("expected ErrPunchFailed, got: %v", err)
	}
}

func TestPunch_ContextCancelled(t *testing.T) {
	idA, _ := identity.Generate()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Dial a port with no listener — will fail quickly with connection refused.
	_, err := nat.Punch(ctx, 0, "127.0.0.1:19999", idA, [32]byte{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, nat.ErrPunchFailed) {
		t.Errorf("expected ErrPunchFailed, got: %v", err)
	}
}

func TestPunch_ReusePort_WithExistingListener(t *testing.T) {
	idA, _ := identity.Generate()
	idB, _ := identity.Generate()

	// A starts a listener so we can discover its address, then keeps it open.
	lnA, err := transport.Listen("127.0.0.1:0", idA)
	if err != nil {
		t.Fatalf("listen A: %v", err)
	}
	defer lnA.Close()

	_, portStr, err := net.SplitHostPort(lnA.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	var localPort int
	if _, err := fmt.Sscanf(portStr, "%d", &localPort); err != nil {
		t.Fatalf("parse port: %v", err)
	}

	bAddr, bConns := startListener(t, idB)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// A punches from its listen port to B (SO_REUSEPORT lets both sockets share the port).
	connA, err := nat.Punch(ctx, uint32(localPort), bAddr, idA, idB.NodeID)
	if err != nil {
		t.Fatalf("Punch from listen port %d: %v", localPort, err)
	}
	defer connA.Close()

	select {
	case connB := <-bConns:
		connB.Close()
	case <-ctx.Done():
		t.Fatal("timeout waiting for B to accept")
	}
}
