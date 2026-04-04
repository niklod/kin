package relay_test

import (
	"context"
	"testing"
	"time"

	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/relay"
)

func TestClientConnect_ExternalAddr(t *testing.T) {
	addr := startRelay(t)
	id, _ := identity.Generate()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := relay.Connect(ctx, addr, id, 7100)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	ext := c.ExternalAddr()
	if ext == "" {
		t.Fatal("ExternalAddr is empty")
	}
	// The relay builds addr as "ip:listenPort", so it should end with the port we gave.
	if ext[len(ext)-4:] != "7100" {
		t.Errorf("ExternalAddr %q does not end with 7100", ext)
	}
}

func TestClientRequestRendezvous(t *testing.T) {
	addr := startRelay(t)

	idA, _ := identity.Generate()
	idB, _ := identity.Generate()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cA, err := relay.Connect(ctx, addr, idA, 7101)
	if err != nil {
		t.Fatalf("connect A: %v", err)
	}
	defer cA.Close()

	cB, err := relay.Connect(ctx, addr, idB, 7102)
	if err != nil {
		t.Fatalf("connect B: %v", err)
	}
	defer cB.Close()

	// A waits for B's rendezvous before B calls RequestRendezvous.
	waitCh := cA.WaitForRendezvous(ctx, idB.NodeID)

	// B requests rendezvous with A.
	rvB, err := cB.RequestRendezvous(ctx, idA.NodeID)
	if err != nil {
		t.Fatalf("RequestRendezvous: %v", err)
	}

	if rvB.PeerNodeID != idA.NodeID {
		t.Errorf("B got wrong peer NodeID")
	}
	if len(rvB.PeerPublicKey) == 0 {
		t.Error("B: PeerPublicKey is empty")
	}
	if rvB.PeerExternalAddr == "" {
		t.Error("B: PeerExternalAddr is empty")
	}

	// A should have received rendezvous info about B.
	select {
	case rvA, ok := <-waitCh:
		if !ok || rvA == nil {
			t.Fatal("A: WaitForRendezvous channel closed without value")
		}
		if rvA.PeerNodeID != idB.NodeID {
			t.Errorf("A got wrong peer NodeID")
		}
		if rvA.PeerExternalAddr == "" {
			t.Error("A: PeerExternalAddr is empty")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for A's rendezvous")
	}
}

func TestClientRequestRendezvous_PeerNotRegistered(t *testing.T) {
	addr := startRelay(t)
	id, _ := identity.Generate()
	unknown, _ := identity.Generate()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := relay.Connect(ctx, addr, id, 7103)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	_, err = c.RequestRendezvous(ctx, unknown.NodeID)
	if err == nil {
		t.Fatal("expected error for unknown peer, got nil")
	}
}

func TestClientRequestRendezvous_RelayDisconnect(t *testing.T) {
	addr := startRelay(t)
	id, _ := identity.Generate()
	target, _ := identity.Generate()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := relay.Connect(ctx, addr, id, 7104)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Close the client; any pending RequestRendezvous should fail.
	c.Close()

	_, err = c.RequestRendezvous(ctx, target.NodeID)
	if err == nil {
		t.Fatal("expected error after Close, got nil")
	}
}

func TestClientClose_Idempotent(t *testing.T) {
	addr := startRelay(t)
	id, _ := identity.Generate()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := relay.Connect(ctx, addr, id, 7105)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestClientContextCancelled(t *testing.T) {
	addr := startRelay(t)
	idA, _ := identity.Generate()
	idB, _ := identity.Generate()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cA, err := relay.Connect(ctx, addr, idA, 7106)
	if err != nil {
		t.Fatalf("connect A: %v", err)
	}
	defer cA.Close()

	// B is not registered, so RequestRendezvous would wait — cancel quickly.
	reqCtx, reqCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer reqCancel()

	_, err = cA.RequestRendezvous(reqCtx, idB.NodeID)
	// With unregistered peer the relay sends an error immediately.
	// This test confirms we get an error either way.
	if err == nil {
		t.Fatal("expected error for unregistered peer, got nil")
	}
}
