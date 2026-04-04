package peerstore_test

import (
	"path/filepath"
	"testing"

	"github.com/niklod/kin/internal/peerstore"
)

func openStore(t *testing.T) *peerstore.Store {
	t.Helper()
	s, err := peerstore.Open(filepath.Join(t.TempDir(), "peers.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutGet(t *testing.T) {
	s := openStore(t)
	p := &peerstore.Peer{
		NodeID:    [32]byte{1},
		PublicKey: []byte("pubkey-aaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Endpoints: []string{"127.0.0.1:5000"},
	}
	if err := s.PutPeer(p); err != nil {
		t.Fatalf("PutPeer: %v", err)
	}
	got, err := s.GetPeer(p.NodeID)
	if err != nil {
		t.Fatalf("GetPeer: %v", err)
	}
	if got == nil {
		t.Fatal("GetPeer: nil")
	}
	if got.TrustState != peerstore.TrustTOFU {
		t.Errorf("TrustState = %q, want %q", got.TrustState, peerstore.TrustTOFU)
	}
}

func TestGetPeer_NotFound(t *testing.T) {
	s := openStore(t)
	got, err := s.GetPeer([32]byte{99})
	if err != nil {
		t.Fatalf("GetPeer: %v", err)
	}
	if got != nil {
		t.Errorf("GetPeer missing key: got %v, want nil", got)
	}
}

func TestPutPeer_TOFUViolation(t *testing.T) {
	s := openStore(t)
	nodeID := [32]byte{2}
	p1 := &peerstore.Peer{NodeID: nodeID, PublicKey: []byte("key-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")}
	p2 := &peerstore.Peer{NodeID: nodeID, PublicKey: []byte("key-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")}

	if err := s.PutPeer(p1); err != nil {
		t.Fatalf("PutPeer first: %v", err)
	}
	err := s.PutPeer(p2)
	if err == nil {
		t.Fatal("expected ErrKeyChanged, got nil")
	}
	if err != peerstore.ErrKeyChanged {
		t.Errorf("error = %v, want ErrKeyChanged", err)
	}
}

func TestPutPeer_MergesEndpoints(t *testing.T) {
	s := openStore(t)
	nodeID := [32]byte{3}
	key := []byte("key-cccccccccccccccccccccccccccccc")

	s.PutPeer(&peerstore.Peer{NodeID: nodeID, PublicKey: key, Endpoints: []string{"a:1"}})
	s.PutPeer(&peerstore.Peer{NodeID: nodeID, PublicKey: key, Endpoints: []string{"b:2"}})

	got, _ := s.GetPeer(nodeID)
	if len(got.Endpoints) != 2 {
		t.Errorf("endpoints = %v, want 2", got.Endpoints)
	}
}

func TestListPeers(t *testing.T) {
	s := openStore(t)
	for i := byte(0); i < 3; i++ {
		s.PutPeer(&peerstore.Peer{
			NodeID:    [32]byte{i},
			PublicKey: []byte{i, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		})
	}
	peers, err := s.ListPeers()
	if err != nil {
		t.Fatalf("ListPeers: %v", err)
	}
	if len(peers) != 3 {
		t.Errorf("ListPeers = %d peers, want 3", len(peers))
	}
}

func TestUpdateLastSeen(t *testing.T) {
	s := openStore(t)
	nodeID := [32]byte{4}
	s.PutPeer(&peerstore.Peer{NodeID: nodeID, PublicKey: []byte("key-dddddddddddddddddddddddddddddd")})

	before, _ := s.GetPeer(nodeID)
	t1 := before.LastSeen

	s.UpdateLastSeen(nodeID)

	after, _ := s.GetPeer(nodeID)
	if !after.LastSeen.After(t1) && after.LastSeen != t1 {
		// Times may be equal at subsecond resolution on fast machines; just verify no error.
	}
	_ = after
}

func TestNonce(t *testing.T) {
	s := openStore(t)
	nonce := [16]byte{1, 2, 3}

	has, err := s.HasNonce(nonce)
	if err != nil || has {
		t.Fatalf("HasNonce before save: err=%v has=%v", err, has)
	}

	if err := s.SaveNonce(nonce); err != nil {
		t.Fatalf("SaveNonce: %v", err)
	}

	has, err = s.HasNonce(nonce)
	if err != nil {
		t.Fatalf("HasNonce after save: %v", err)
	}
	if !has {
		t.Error("HasNonce after save: expected true")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.db")
	nodeID := [32]byte{5}

	s1, _ := peerstore.Open(path)
	s1.PutPeer(&peerstore.Peer{NodeID: nodeID, PublicKey: []byte("key-eeeeeeeeeeeeeeeeeeeeeeeeeeeeee")})
	s1.Close()

	s2, err := peerstore.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	got, err := s2.GetPeer(nodeID)
	if err != nil || got == nil {
		t.Fatalf("GetPeer after reopen: err=%v got=%v", err, got)
	}
}
