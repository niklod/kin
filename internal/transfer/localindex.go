// Package transfer implements streaming file transfer by SHA-256 content hash.
package transfer

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// LocalIndex maps SHA-256 file hashes to their paths on disk.
type LocalIndex struct {
	mu    sync.RWMutex
	index map[[32]byte]string // hash -> absolute path
}

// NewLocalIndex creates an empty index.
func NewLocalIndex() *LocalIndex {
	return &LocalIndex{index: make(map[[32]byte]string)}
}

// Scan walks dir recursively, hashing every regular file and adding it to the index.
// Existing entries for the same hash are overwritten.
// Walk-level errors (e.g., permission denied on the root) are returned; individual
// file hash errors are skipped so a partial index is still useful.
func (idx *LocalIndex) Scan(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // propagate walk-level errors (broken root, permission denied)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		hash, err := hashFile(path)
		if err != nil {
			return nil // skip unreadable files; partial index is still useful
		}
		idx.mu.Lock()
		idx.index[hash] = path
		idx.mu.Unlock()
		return nil
	})
}

// Add hashes the file at path and adds it to the index.
func (idx *LocalIndex) Add(path string) error {
	hash, err := hashFile(path)
	if err != nil {
		return fmt.Errorf("index add: %w", err)
	}
	idx.mu.Lock()
	idx.index[hash] = path
	idx.mu.Unlock()
	return nil
}

// AddWithHash adds a path to the index under an explicit hash (useful for testing).
func (idx *LocalIndex) AddWithHash(hash [32]byte, path string) {
	idx.mu.Lock()
	idx.index[hash] = path
	idx.mu.Unlock()
}

// Remove deletes the entry for the given hash from the index.
func (idx *LocalIndex) Remove(hash [32]byte) {
	idx.mu.Lock()
	delete(idx.index, hash)
	idx.mu.Unlock()
}

// Lookup returns the path for a file with the given hash, or ("", false).
func (idx *LocalIndex) Lookup(hash [32]byte) (string, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	p, ok := idx.index[hash]
	return p, ok
}

// hashFile computes the SHA-256 of the file at path.
func hashFile(path string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}
