package nat_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/nat"
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

func startListener(t *testing.T, id *identity.Identity) (*transport.Listener, <-chan *transport.Conn) {
	t.Helper()
	ln, err := transport.Listen("127.0.0.1:0", id)
	if err != nil {
		t.Fatalf("transport.Listen: %v", err)
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
	return ln, ch
}

func TestPunch_ConnectsAndAuthenticated(t *testing.T) {
	idA := mustGenID(t)
	idB := mustGenID(t)

	lnA, _ := startListener(t, idA)
	lnB, bConns := startListener(t, idB)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connA, err := nat.Punch(ctx, lnA, lnB.Addr().String(), idB.NodeID)
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
	idA := mustGenID(t)
	idB := mustGenID(t)
	wrong := mustGenID(t)

	lnA, _ := startListener(t, idA)
	lnB, _ := startListener(t, idB)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := nat.Punch(ctx, lnA, lnB.Addr().String(), wrong.NodeID)
	if err == nil {
		t.Fatal("expected error for wrong NodeID, got nil")
	}
	if !errors.Is(err, nat.ErrPunchFailed) {
		t.Errorf("expected ErrPunchFailed, got: %v", err)
	}
}

func TestPunch_ContextCancelled(t *testing.T) {
	idA := mustGenID(t)

	lnA, _ := startListener(t, idA)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Dial a port with no listener — will fail quickly or on context expiry.
	_, err := nat.Punch(ctx, lnA, "127.0.0.1:19999", [32]byte{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, nat.ErrPunchFailed) {
		t.Errorf("expected ErrPunchFailed, got: %v", err)
	}
}

// TestPunch_ViaSharedListener verifies that an active listener can also dial
// outbound via Punch — the same UDP socket handles both roles without conflict.
func TestPunch_ViaSharedListener(t *testing.T) {
	idA := mustGenID(t)
	idB := mustGenID(t)

	lnA, _ := startListener(t, idA)
	lnB, bConns := startListener(t, idB)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connA, err := nat.Punch(ctx, lnA, lnB.Addr().String(), idB.NodeID)
	if err != nil {
		t.Fatalf("Punch from shared listener: %v", err)
	}
	defer connA.Close()

	select {
	case connB := <-bConns:
		connB.Close()
	case <-ctx.Done():
		t.Fatal("timeout waiting for B to accept")
	}
}

func TestPrimeNAT_BurstSent(t *testing.T) {
	idA := mustGenID(t)
	lnA, _ := startListener(t, idA)

	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	defer udpConn.Close()

	count := make(chan int, 1)
	go func() {
		var n int
		buf := make([]byte, 64)
		_ = udpConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		for {
			_, _, err := udpConn.ReadFrom(buf)
			if err != nil {
				break
			}
			n++
		}
		count <- n
	}()

	nat.PrimeNAT(context.Background(), lnA, udpConn.LocalAddr().String())

	received := <-count
	if received != 5 {
		t.Errorf("PrimeNAT sent %d packets, want 5", received)
	}
}
