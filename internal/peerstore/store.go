// Package peerstore implements a bbolt-backed store for known peers and invite nonces.
package peerstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	bucketNamePeers  = "peers"
	bucketNameNonces = "nonces"
)

// ErrKeyChanged is returned when a peer reconnects with a different public key.
var ErrKeyChanged = errors.New("peerstore: public key changed for known node (TOFU violation)")

// Store persists peer information and invite nonces using bbolt.
type Store struct {
	db *bolt.DB
}

// Open opens (or creates) the bbolt database at the given path.
// Returns an error if the file lock cannot be acquired within 3 seconds
// (e.g. another kin process is already running with the same config dir).
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open peerstore: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(bucketNamePeers)); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists([]byte(bucketNameNonces))
		return err
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("init buckets: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// PutPeer stores or updates a peer.
// TOFU: if the NodeID is already known with a different public key, ErrKeyChanged is returned.
// Otherwise endpoints are merged and last_seen is updated.
// The caller's *Peer value is never mutated.
func (s *Store) PutPeer(p *Peer) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNamePeers))
		// Work on a copy so the caller's value is never mutated.
		updated := *p
		key := updated.NodeID[:]
		existing := b.Get(key)
		if existing != nil {
			var prev Peer
			if err := json.Unmarshal(existing, &prev); err != nil {
				return fmt.Errorf("unmarshal existing peer: %w", err)
			}
			if !bytes.Equal(prev.PublicKey, updated.PublicKey) {
				return ErrKeyChanged
			}
			updated.Endpoints = mergeEndpoints(prev.Endpoints, updated.Endpoints)
		}
		if updated.TrustState == "" {
			updated.TrustState = TrustTOFU
		}
		updated.LastSeen = time.Now().UTC()
		data, err := json.Marshal(&updated)
		if err != nil {
			return fmt.Errorf("marshal peer: %w", err)
		}
		return b.Put(key, data)
	})
}

// GetPeer returns a peer by NodeID, or nil if not found.
func (s *Store) GetPeer(nodeID [32]byte) (*Peer, error) {
	var p *Peer
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNamePeers))
		data := b.Get(nodeID[:])
		if data == nil {
			return nil
		}
		p = new(Peer)
		return json.Unmarshal(data, p)
	})
	if err != nil {
		return nil, fmt.Errorf("get peer: %w", err)
	}
	return p, nil
}

// ListPeers returns all peers in the store.
func (s *Store) ListPeers() ([]*Peer, error) {
	var peers []*Peer
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNamePeers))
		return b.ForEach(func(_, v []byte) error {
			var p Peer
			if err := json.Unmarshal(v, &p); err != nil {
				return err
			}
			peers = append(peers, &p)
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("list peers: %w", err)
	}
	return peers, nil
}

// UpdateLastSeen updates the last_seen timestamp for a peer.
func (s *Store) UpdateLastSeen(nodeID [32]byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNamePeers))
		data := b.Get(nodeID[:])
		if data == nil {
			return nil
		}
		var p Peer
		if err := json.Unmarshal(data, &p); err != nil {
			return fmt.Errorf("unmarshal peer: %w", err)
		}
		p.LastSeen = time.Now().UTC()
		updated, err := json.Marshal(&p)
		if err != nil {
			return fmt.Errorf("marshal peer: %w", err)
		}
		return b.Put(nodeID[:], updated)
	})
}

// HasNonce returns true if the nonce has already been recorded.
func (s *Store) HasNonce(nonce [16]byte) (bool, error) {
	var found bool
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNameNonces))
		found = b.Get(nonce[:]) != nil
		return nil
	})
	return found, err
}

// SaveNonce records a nonce as used.
func (s *Store) SaveNonce(nonce [16]byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNameNonces))
		return b.Put(nonce[:], []byte{})
	})
}

// mergeEndpoints returns the union of two endpoint slices, preserving order.
func mergeEndpoints(existing, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing))
	result := make([]string, 0, len(existing)+len(incoming))
	for _, ep := range existing {
		seen[ep] = struct{}{}
		result = append(result, ep)
	}
	for _, ep := range incoming {
		if _, ok := seen[ep]; !ok {
			result = append(result, ep)
		}
	}
	return result
}
