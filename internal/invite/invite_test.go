package invite_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/niklod/kin/internal/invite"
)

// memNonceStore is an in-memory nonce store for testing.
type memNonceStore struct {
	mu     sync.Mutex
	nonces map[[16]byte]struct{}
}

func newMemNonceStore() *memNonceStore {
	return &memNonceStore{nonces: make(map[[16]byte]struct{})}
}

func (m *memNonceStore) HasNonce(n [16]byte) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.nonces[n]
	return ok, nil
}

func (m *memNonceStore) SaveNonce(n [16]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nonces[n] = struct{}{}
	return nil
}

func generateKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return pub, priv
}

func TestRoundTrip(t *testing.T) {
	_, priv := generateKey(t)
	ns := newMemNonceStore()

	tok, err := invite.Create(priv, []string{"127.0.0.1:5000"}, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	raw, err := invite.Encode(tok, priv)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if raw[:4] != "kin:" {
		t.Errorf("missing kin: prefix: %q", raw[:4])
	}

	decoded, err := invite.Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if err := invite.Validate(decoded, raw, ns); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if len(decoded.Endpoints) != 1 || decoded.Endpoints[0] != "127.0.0.1:5000" {
		t.Errorf("endpoints = %v, want [127.0.0.1:5000]", decoded.Endpoints)
	}
}

func TestNonceReplay(t *testing.T) {
	_, priv := generateKey(t)
	ns := newMemNonceStore()

	tok, _ := invite.Create(priv, nil, time.Hour)
	raw, _ := invite.Encode(tok, priv)
	decoded, _ := invite.Decode(raw)

	if err := invite.Validate(decoded, raw, ns); err != nil {
		t.Fatalf("first Validate: %v", err)
	}
	// Decode again to get a fresh Token (same nonce).
	decoded2, _ := invite.Decode(raw)
	err := invite.Validate(decoded2, raw, ns)
	if err == nil {
		t.Fatal("replay: expected error, got nil")
	}
}

func TestExpiredToken(t *testing.T) {
	_, priv := generateKey(t)
	ns := newMemNonceStore()

	// Create a valid token, then back-date its expiry before encoding.
	tok, err := invite.Create(priv, nil, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tok.ExpiresAt = time.Now().UTC().Add(-time.Second) // move expiry into the past

	raw, err := invite.Encode(tok, priv)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := invite.Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	err = invite.Validate(decoded, raw, ns)
	if err == nil {
		t.Fatal("expired token: expected error, got nil")
	}
}

func TestTamperedSignature(t *testing.T) {
	_, priv := generateKey(t)
	ns := newMemNonceStore()

	tok, _ := invite.Create(priv, nil, time.Hour)
	raw, _ := invite.Encode(tok, priv)

	// Flip the last character of the base64url payload.
	tampered := raw[:len(raw)-1] + "X"
	decoded, err := invite.Decode(tampered)
	if err != nil {
		// Tampered base64 might fail to decode — that's also acceptable.
		return
	}
	err = invite.Validate(decoded, tampered, ns)
	if err == nil {
		t.Fatal("tampered signature: expected error, got nil")
	}
}

func TestMissingPrefix(t *testing.T) {
	_, err := invite.Decode("notakinlink")
	if err == nil {
		t.Error("Decode without prefix: expected error")
	}
}

func TestNoEndpoints(t *testing.T) {
	_, priv := generateKey(t)
	ns := newMemNonceStore()

	tok, _ := invite.Create(priv, nil, time.Hour)
	raw, _ := invite.Encode(tok, priv)
	decoded, _ := invite.Decode(raw)

	if err := invite.Validate(decoded, raw, ns); err != nil {
		t.Fatalf("Validate with no endpoints: %v", err)
	}
	if len(decoded.Endpoints) != 0 {
		t.Errorf("endpoints = %v, want []", decoded.Endpoints)
	}
}

func TestMultipleEndpoints(t *testing.T) {
	_, priv := generateKey(t)
	ns := newMemNonceStore()

	eps := []string{"1.2.3.4:5000", "[::1]:5001", "example.com:5002"}
	tok, _ := invite.Create(priv, eps, time.Hour)
	raw, _ := invite.Encode(tok, priv)
	decoded, _ := invite.Decode(raw)

	if err := invite.Validate(decoded, raw, ns); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(decoded.Endpoints) != len(eps) {
		t.Errorf("endpoints = %v, want %v", decoded.Endpoints, eps)
	}
	for i, ep := range eps {
		if decoded.Endpoints[i] != ep {
			t.Errorf("endpoint[%d] = %q, want %q", i, decoded.Endpoints[i], ep)
		}
	}
}
