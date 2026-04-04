package relay_test

import (
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/protocol"
	"github.com/niklod/kin/internal/relay"
	"github.com/niklod/kin/internal/transport"
	"github.com/niklod/kin/kinpb"
)

// startRelay starts a TLS relay server and returns its address.
func startRelay(t *testing.T) string {
	t.Helper()
	relayID, err := identity.Generate()
	if err != nil {
		t.Fatalf("relay identity: %v", err)
	}
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

// connectToRelay opens a plain TLS client connection to the relay.
func connectToRelay(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
		MinVersion:         tls.VersionTLS13,
	})
	if err != nil {
		t.Fatalf("dial relay: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func register(t *testing.T, conn net.Conn, id *identity.Identity, listenPort uint32) string {
	t.Helper()
	err := protocol.WriteMsg(conn, &kinpb.Envelope{Payload: &kinpb.Envelope_RelayRegister{
		RelayRegister: &kinpb.RelayRegister{
			NodeId:     id.NodeID[:],
			PublicKey:  id.PubKey,
			ListenPort: listenPort,
		},
	}})
	if err != nil {
		t.Fatalf("send register: %v", err)
	}
	env, err := protocol.ReadMsg(conn)
	if err != nil {
		t.Fatalf("recv registered: %v", err)
	}
	reg := env.GetRelayRegistered()
	if reg == nil {
		t.Fatalf("expected RelayRegistered, got %T", env.Payload)
	}
	return reg.ExternalAddr
}

func TestRegister(t *testing.T) {
	addr := startRelay(t)
	id, _ := identity.Generate()
	conn := connectToRelay(t, addr)

	extAddr := register(t, conn, id, 7777)
	if extAddr == "" {
		t.Error("external addr is empty")
	}
	// Should contain the port we reported.
	if extAddr[len(extAddr)-4:] != "7777" {
		t.Errorf("external addr %q does not end with 7777", extAddr)
	}
}

func TestRendezvous(t *testing.T) {
	addr := startRelay(t)

	idA, _ := identity.Generate()
	idB, _ := identity.Generate()

	connA := connectToRelay(t, addr)
	connB := connectToRelay(t, addr)

	register(t, connA, idA, 7001)
	register(t, connB, idB, 7002)

	// B requests rendezvous with A.
	err := protocol.WriteMsg(connB, &kinpb.Envelope{Payload: &kinpb.Envelope_RelayConnect{
		RelayConnect: &kinpb.RelayConnect{TargetNodeId: idA.NodeID[:]},
	}})
	if err != nil {
		t.Fatalf("send RelayConnect: %v", err)
	}

	// A should receive RelayRendezvous.
	envA, err := protocol.ReadMsg(connA)
	if err != nil {
		t.Fatalf("recv from A: %v", err)
	}
	rvA := envA.GetRelayRendezvous()
	if rvA == nil {
		t.Fatalf("A: expected RelayRendezvous, got %T", envA.Payload)
	}
	if string(rvA.PeerNodeId) != string(idB.NodeID[:]) {
		t.Error("A received wrong peer NodeID")
	}

	// B should receive RelayRendezvous.
	envB, err := protocol.ReadMsg(connB)
	if err != nil {
		t.Fatalf("recv from B: %v", err)
	}
	rvB := envB.GetRelayRendezvous()
	if rvB == nil {
		t.Fatalf("B: expected RelayRendezvous, got %T", envB.Payload)
	}
	if string(rvB.PeerNodeId) != string(idA.NodeID[:]) {
		t.Error("B received wrong peer NodeID")
	}
}

func TestRendezvous_TargetNotFound(t *testing.T) {
	addr := startRelay(t)
	idB, _ := identity.Generate()
	connB := connectToRelay(t, addr)
	register(t, connB, idB, 7003)

	missingID := [32]byte{0xFF}
	protocol.WriteMsg(connB, &kinpb.Envelope{Payload: &kinpb.Envelope_RelayConnect{
		RelayConnect: &kinpb.RelayConnect{TargetNodeId: missingID[:]},
	}})

	env, err := protocol.ReadMsg(connB)
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	re := env.GetRelayError()
	if re == nil {
		t.Fatalf("expected RelayError, got %T", env.Payload)
	}
	if re.Reason != "peer_not_registered" {
		t.Errorf("reason = %q, want peer_not_registered", re.Reason)
	}
}

func TestNode_Deregisters_On_Disconnect(t *testing.T) {
	addr := startRelay(t)
	idA, _ := identity.Generate()
	idB, _ := identity.Generate()

	connA := connectToRelay(t, addr)
	connB := connectToRelay(t, addr)

	register(t, connA, idA, 7004)
	register(t, connB, idB, 7005)

	// Disconnect A.
	connA.Close()
	time.Sleep(50 * time.Millisecond) // let relay process disconnect

	// B requests A — should get error.
	protocol.WriteMsg(connB, &kinpb.Envelope{Payload: &kinpb.Envelope_RelayConnect{
		RelayConnect: &kinpb.RelayConnect{TargetNodeId: idA.NodeID[:]},
	}})

	env, err := protocol.ReadMsg(connB)
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if env.GetRelayError() == nil {
		t.Errorf("expected RelayError after A disconnected, got %T", env.Payload)
	}
}
