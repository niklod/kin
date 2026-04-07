package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketEntries      = []byte("entries")
	bucketAvailability = []byte("availability")
)

// Store persists the file catalog using bbolt.
type Store struct {
	db     *bolt.DB
	selfID [32]byte
}

// Open opens or creates a catalog database at path.
func Open(path string, selfID [32]byte) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("catalog open: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketEntries); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(bucketAvailability)
		return err
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("catalog init buckets: %w", err)
	}
	return &Store{db: db, selfID: selfID}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// PutLocal inserts or updates a locally-owned catalog entry.
// OwnerNodeID is set to selfID and Deleted is cleared automatically.
// The availability index is updated in the same transaction.
func (s *Store) PutLocal(entry *Entry) error {
	e := *entry
	e.OwnerNodeID = s.selfID
	e.UpdatedAt = time.Now().UTC()
	e.Deleted = false

	return s.db.Update(func(tx *bolt.Tx) error {
		if err := putEntry(tx, &e); err != nil {
			return fmt.Errorf("put local: %w", err)
		}
		return addAvailability(tx, e.FileID, s.selfID)
	})
}

// DeleteLocal marks a local file as deleted and removes it from the
// availability index.
func (s *Store) DeleteLocal(fileID [32]byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		key := compositeKey(s.selfID, fileID)
		b := tx.Bucket(bucketEntries)
		raw := b.Get(key[:])
		if raw == nil {
			return nil // nothing to delete
		}
		var e Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			return fmt.Errorf("delete local unmarshal: %w", err)
		}
		e.Deleted = true
		e.UpdatedAt = time.Now().UTC()
		if err := putEntry(tx, &e); err != nil {
			return fmt.Errorf("delete local: %w", err)
		}
		return removeAvailability(tx, fileID, s.selfID)
	})
}

// ListLocal returns all non-deleted entries owned by this node.
func (s *Store) ListLocal() ([]*Entry, error) {
	return s.listByPrefix(s.selfID)
}

// LookupLocalByPath returns the catalog entry for a local file at the given
// path, or nil if not found.
func (s *Store) LookupLocalByPath(path string) (*Entry, error) {
	var found *Entry
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketEntries).Cursor()
		prefix := s.selfID[:]
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			var e Entry
			if err := json.Unmarshal(v, &e); err != nil {
				continue
			}
			if !e.Deleted && e.LocalPath == path {
				found = &e
				return nil
			}
		}
		return nil
	})
	return found, err
}

// PutPeerEntries replaces all catalog entries for the given peer. Existing
// entries for peerNodeID are deleted first, then the new set is inserted.
// The availability index is updated in the same transaction.
func (s *Store) PutPeerEntries(peerNodeID [32]byte, entries []*Entry) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := deleteByPrefix(tx, peerNodeID); err != nil {
			return fmt.Errorf("put peer clear: %w", err)
		}
		for _, entry := range entries {
			e := *entry
			e.OwnerNodeID = peerNodeID
			e.UpdatedAt = time.Now().UTC()
			if err := putEntry(tx, &e); err != nil {
				return fmt.Errorf("put peer entry: %w", err)
			}
			if !e.Deleted {
				if err := addAvailability(tx, e.FileID, peerNodeID); err != nil {
					return fmt.Errorf("put peer availability: %w", err)
				}
			}
		}
		return nil
	})
}

// RemovePeer deletes all entries for the given peer and cleans up availability.
func (s *Store) RemovePeer(peerNodeID [32]byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		entries := listByPrefixTx(tx, peerNodeID)
		for _, e := range entries {
			if err := removeAvailability(tx, e.FileID, peerNodeID); err != nil {
				return fmt.Errorf("remove peer availability: %w", err)
			}
		}
		return deleteByPrefix(tx, peerNodeID)
	})
}

// ListAll returns all non-deleted entries across all owners.
func (s *Store) ListAll() ([]*Entry, error) {
	var result []*Entry
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketEntries).ForEach(func(_, v []byte) error {
			var e Entry
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			if !e.Deleted {
				result = append(result, &e)
			}
			return nil
		})
	})
	return result, err
}

// ListByOwner returns all non-deleted entries for a specific owner.
func (s *Store) ListByOwner(nodeID [32]byte) ([]*Entry, error) {
	return s.listByPrefix(nodeID)
}

// GetOwners returns the list of peers who have a given file_id.
func (s *Store) GetOwners(fileID [32]byte) ([]OwnerHint, error) {
	var hints []OwnerHint
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(bucketAvailability).Get(fileID[:])
		if raw == nil {
			return nil
		}
		return json.Unmarshal(raw, &hints)
	})
	return hints, err
}

// GetEntry returns a specific entry by owner+fileID, or nil if not found.
func (s *Store) GetEntry(ownerNodeID, fileID [32]byte) (*Entry, error) {
	var entry *Entry
	err := s.db.View(func(tx *bolt.Tx) error {
		key := compositeKey(ownerNodeID, fileID)
		raw := tx.Bucket(bucketEntries).Get(key[:])
		if raw == nil {
			return nil
		}
		var e Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			return fmt.Errorf("get entry: %w", err)
		}
		entry = &e
		return nil
	})
	return entry, err
}

// ListForPeer returns all non-deleted entries suitable for sending to a peer,
// excluding entries owned by excludeNodeID (loop prevention).
func (s *Store) ListForPeer(excludeNodeID [32]byte) ([]*Entry, error) {
	var result []*Entry
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketEntries).ForEach(func(_, v []byte) error {
			var e Entry
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			if !e.Deleted && e.OwnerNodeID != excludeNodeID {
				result = append(result, &e)
			}
			return nil
		})
	})
	return result, err
}

// --- internal helpers ---

func compositeKey(ownerNodeID, fileID [32]byte) [64]byte {
	var key [64]byte
	copy(key[:32], ownerNodeID[:])
	copy(key[32:], fileID[:])
	return key
}

func putEntry(tx *bolt.Tx, e *Entry) error {
	key := compositeKey(e.OwnerNodeID, e.FileID)
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return tx.Bucket(bucketEntries).Put(key[:], data)
}

func listByPrefixTx(tx *bolt.Tx, ownerNodeID [32]byte) []*Entry {
	var result []*Entry
	c := tx.Bucket(bucketEntries).Cursor()
	prefix := ownerNodeID[:]
	for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
		var e Entry
		if err := json.Unmarshal(v, &e); err != nil {
			continue
		}
		if !e.Deleted {
			result = append(result, &e)
		}
	}
	return result
}

func (s *Store) listByPrefix(ownerNodeID [32]byte) ([]*Entry, error) {
	var result []*Entry
	err := s.db.View(func(tx *bolt.Tx) error {
		result = listByPrefixTx(tx, ownerNodeID)
		return nil
	})
	return result, err
}

func deleteByPrefix(tx *bolt.Tx, ownerNodeID [32]byte) error {
	c := tx.Bucket(bucketEntries).Cursor()
	prefix := ownerNodeID[:]
	for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() { //nolint:revive
		if err := c.Delete(); err != nil {
			return err
		}
	}
	return nil
}

func addAvailability(tx *bolt.Tx, fileID [32]byte, nodeID [32]byte) error {
	b := tx.Bucket(bucketAvailability)
	var hints []OwnerHint

	raw := b.Get(fileID[:])
	if raw != nil {
		if err := json.Unmarshal(raw, &hints); err != nil {
			hints = nil
		}
	}

	now := time.Now().UTC()
	found := false
	for i, h := range hints {
		if h.NodeID == nodeID {
			hints[i].SeenAt = now
			found = true
			break
		}
	}
	if !found {
		hints = append(hints, OwnerHint{NodeID: nodeID, SeenAt: now})
	}

	data, err := json.Marshal(hints)
	if err != nil {
		return err
	}
	return b.Put(fileID[:], data)
}

func removeAvailability(tx *bolt.Tx, fileID [32]byte, nodeID [32]byte) error {
	b := tx.Bucket(bucketAvailability)
	raw := b.Get(fileID[:])
	if raw == nil {
		return nil
	}

	var hints []OwnerHint
	if err := json.Unmarshal(raw, &hints); err != nil {
		return b.Delete(fileID[:])
	}

	filtered := hints[:0]
	for _, h := range hints {
		if h.NodeID != nodeID {
			filtered = append(filtered, h)
		}
	}

	if len(filtered) == 0 {
		return b.Delete(fileID[:])
	}

	data, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	return b.Put(fileID[:], data)
}
