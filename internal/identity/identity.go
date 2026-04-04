// Package identity manages the node's long-term Ed25519 keypair and NodeID.
//
// Only the 32-byte seed is persisted to disk; the full key pair is
// reconstructed deterministically via ed25519.NewKeyFromSeed.
// NodeID = SHA-256(public_key), encoded as 32 bytes.
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	keyFile  = "identity.key"
	seedSize = 32 // ed25519 seed length
)

// Identity holds the node's cryptographic identity.
type Identity struct {
	PrivKey ed25519.PrivateKey
	PubKey  ed25519.PublicKey
	NodeID  [32]byte // SHA-256(PubKey)
}

// NodeIDHex returns the node ID as a lowercase hex string.
func (id *Identity) NodeIDHex() string {
	return hex.EncodeToString(id.NodeID[:])
}

// Generate creates a fresh Ed25519 keypair and derives the NodeID.
func Generate() (*Identity, error) {
	seed := make([]byte, seedSize)
	if _, err := rand.Read(seed); err != nil {
		return nil, fmt.Errorf("generate seed: %w", err)
	}
	return fromSeed(seed), nil
}

// Save writes the 32-byte seed to configDir/identity.key with mode 0600.
// configDir is created if it does not exist.
func Save(id *Identity, configDir string) error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	seed := id.PrivKey.Seed()
	path := filepath.Join(configDir, keyFile)
	if err := os.WriteFile(path, seed, 0600); err != nil {
		return fmt.Errorf("write identity file: %w", err)
	}
	return nil
}

// Load reads the seed from configDir/identity.key and reconstructs the keypair.
// Returns os.ErrNotExist if the file does not exist.
func Load(configDir string) (*Identity, error) {
	path := filepath.Join(configDir, keyFile)
	seed, err := os.ReadFile(path)
	if err != nil {
		return nil, err // preserves os.ErrNotExist for caller
	}
	if len(seed) != seedSize {
		return nil, fmt.Errorf("identity file corrupt: expected %d bytes, got %d", seedSize, len(seed))
	}
	return fromSeed(seed), nil
}

// LoadOrGenerate loads the identity from disk; if not found, generates and saves a new one.
func LoadOrGenerate(configDir string) (*Identity, error) {
	id, err := Load(configDir)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load identity: %w", err)
	}
	id, err = Generate()
	if err != nil {
		return nil, fmt.Errorf("generate identity: %w", err)
	}
	if err := Save(id, configDir); err != nil {
		return nil, fmt.Errorf("save identity: %w", err)
	}
	return id, nil
}

// fromSeed constructs an Identity from a raw 32-byte seed.
func fromSeed(seed []byte) *Identity {
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	nodeID := sha256.Sum256(pub)
	return &Identity{
		PrivKey: priv,
		PubKey:  pub,
		NodeID:  nodeID,
	}
}
