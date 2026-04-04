package peerstore

import "time"

// TrustState indicates how much a peer is trusted.
type TrustState string

const (
	TrustTOFU      TrustState = "tofu"      // key pinned on first contact
	TrustSuspect   TrustState = "suspect"   // key mismatch detected
	TrustBlocked   TrustState = "blocked"   // manually blocked
)

// Peer represents a known remote node.
type Peer struct {
	NodeID     [32]byte   `json:"node_id"`
	PublicKey  []byte     `json:"public_key"`   // 32-byte Ed25519 pubkey
	Endpoints  []string   `json:"endpoints"`    // host:port strings
	TrustState TrustState `json:"trust_state"`
	LastSeen   time.Time  `json:"last_seen"`
}
