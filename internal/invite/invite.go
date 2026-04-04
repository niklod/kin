// Package invite implements one-time invite tokens for peer discovery.
//
// Token binary layout (before base64url encoding):
//
//	[32B pubkey][2B endpoint count (BE uint16)][endpoints as [2B len][bytes]...][16B nonce][8B expiry unix (BE int64)][64B signature]
//
// The signature covers everything before the signature field.
// Encoded as: "kin:" + base64url(binary)
package invite

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	prefix     = "kin:"
	pubkeySize = 32
	nonceSize  = 16
	expirySize = 8
	sigSize    = 64
)

// Token holds the decoded fields of an invite link.
type Token struct {
	PublicKey []byte    // 32-byte Ed25519 pubkey of inviter
	Endpoints []string  // host:port strings
	Nonce     [16]byte  // random nonce for one-time use
	ExpiresAt time.Time // when the invite expires
}

// NonceStore is satisfied by peerstore.Store.
type NonceStore interface {
	HasNonce(nonce [16]byte) (bool, error)
	SaveNonce(nonce [16]byte) error
}

// DefaultTTL is the default validity window for new invite tokens.
const DefaultTTL = 24 * time.Hour

// Create builds and signs a new invite token.
// ttl must be positive; zero or negative values are replaced with DefaultTTL.
func Create(privKey ed25519.PrivateKey, endpoints []string, ttl time.Duration) (*Token, error) {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	var nonce [nonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return &Token{
		PublicKey: privKey.Public().(ed25519.PublicKey),
		Endpoints: endpoints,
		Nonce:     nonce,
		ExpiresAt: time.Now().UTC().Add(ttl),
	}, nil
}

// Encode serialises and signs the token, returning a "kin:..." string.
func Encode(t *Token, privKey ed25519.PrivateKey) (string, error) {
	payload, err := marshalPayload(t)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(privKey, payload)
	full := append(payload, sig...)
	return prefix + base64.RawURLEncoding.EncodeToString(full), nil
}

// Decode parses a "kin:..." string into a Token. Does not verify anything beyond structure.
func Decode(raw string) (*Token, error) {
	if !strings.HasPrefix(raw, prefix) {
		return nil, errors.New("invite: missing 'kin:' prefix")
	}
	data, err := base64.RawURLEncoding.DecodeString(raw[len(prefix):])
	if err != nil {
		return nil, fmt.Errorf("invite: base64 decode: %w", err)
	}
	return unmarshal(data)
}

// Validate verifies signature, expiry, and nonce replay.
// On success it records the nonce in the store.
func Validate(t *Token, raw string, ns NonceStore) error {
	if time.Now().UTC().After(t.ExpiresAt) {
		return errors.New("invite: token expired")
	}
	// Reconstruct the signed payload and verify signature.
	data, err := base64.RawURLEncoding.DecodeString(raw[len(prefix):])
	if err != nil {
		return fmt.Errorf("invite: decode for validation: %w", err)
	}
	if len(data) < sigSize {
		return errors.New("invite: data too short for signature")
	}
	payload := data[:len(data)-sigSize]
	sig := data[len(data)-sigSize:]
	if !ed25519.Verify(t.PublicKey, payload, sig) {
		return errors.New("invite: invalid signature")
	}
	has, err := ns.HasNonce(t.Nonce)
	if err != nil {
		return fmt.Errorf("invite: nonce check: %w", err)
	}
	if has {
		return errors.New("invite: nonce already used")
	}
	if err := ns.SaveNonce(t.Nonce); err != nil {
		return fmt.Errorf("invite: save nonce: %w", err)
	}
	return nil
}

// marshalPayload encodes the signable portion of the token (everything except signature).
func marshalPayload(t *Token) ([]byte, error) {
	if len(t.PublicKey) != pubkeySize {
		return nil, fmt.Errorf("invite: public key must be %d bytes", pubkeySize)
	}
	if len(t.Endpoints) > 0xFFFF {
		return nil, errors.New("invite: too many endpoints")
	}
	var buf []byte
	buf = append(buf, t.PublicKey...)

	epCount := uint16(len(t.Endpoints))
	buf = binary.BigEndian.AppendUint16(buf, epCount)
	for _, ep := range t.Endpoints {
		epBytes := []byte(ep)
		if len(epBytes) > 0xFFFF {
			return nil, errors.New("invite: endpoint string too long")
		}
		buf = binary.BigEndian.AppendUint16(buf, uint16(len(epBytes)))
		buf = append(buf, epBytes...)
	}

	buf = append(buf, t.Nonce[:]...)

	buf = binary.BigEndian.AppendUint64(buf, uint64(t.ExpiresAt.Unix()))

	return buf, nil
}

// unmarshal decodes the full binary token (payload + signature).
func unmarshal(data []byte) (*Token, error) {
	minSize := pubkeySize + 2 + nonceSize + expirySize + sigSize
	if len(data) < minSize {
		return nil, fmt.Errorf("invite: token too short (%d bytes)", len(data))
	}

	offset := 0
	pubkey := make([]byte, pubkeySize)
	copy(pubkey, data[offset:offset+pubkeySize])
	offset += pubkeySize

	epCount := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	endpoints := make([]string, 0, epCount)
	for i := uint16(0); i < epCount; i++ {
		if offset+2 > len(data)-nonceSize-expirySize-sigSize {
			return nil, errors.New("invite: truncated endpoint")
		}
		epLen := binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
		if offset+int(epLen) > len(data)-nonceSize-expirySize-sigSize {
			return nil, errors.New("invite: truncated endpoint data")
		}
		endpoints = append(endpoints, string(data[offset:offset+int(epLen)]))
		offset += int(epLen)
	}

	if offset+nonceSize > len(data)-expirySize-sigSize {
		return nil, errors.New("invite: truncated nonce")
	}
	var nonce [16]byte
	copy(nonce[:], data[offset:offset+nonceSize])
	offset += nonceSize

	if offset+expirySize > len(data)-sigSize {
		return nil, errors.New("invite: truncated expiry")
	}
	expiry := int64(binary.BigEndian.Uint64(data[offset : offset+expirySize]))
	offset += expirySize

	// Remaining sigSize bytes are the signature (consumed by Validate, not stored in Token).

	return &Token{
		PublicKey: pubkey,
		Endpoints: endpoints,
		Nonce:     nonce,
		ExpiresAt: time.Unix(expiry, 0).UTC(),
	}, nil
}
