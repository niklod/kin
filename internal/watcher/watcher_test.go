package watcher_test

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/kin/internal/catalog"
	"github.com/niklod/kin/internal/watcher"
)

var selfID = [32]byte{0xAA}

// stubIndex is a minimal LocalIndexer for tests.
type stubIndex struct {
	added   map[[32]byte]string
	removed map[[32]byte]bool
}

func newStubIndex() *stubIndex {
	return &stubIndex{
		added:   make(map[[32]byte]string),
		removed: make(map[[32]byte]bool),
	}
}

func (s *stubIndex) Add(path string) error {
	hash := hashFileSync(path)
	s.added[hash] = path
	return nil
}

func (s *stubIndex) Remove(hash [32]byte) {
	s.removed[hash] = true
	delete(s.added, hash)
}

type WatcherSuite struct {
	suite.Suite
	dir   string
	store *catalog.Store
	index *stubIndex
}

func (s *WatcherSuite) SetupTest() {
	s.dir = s.T().TempDir()
	store, err := catalog.Open(filepath.Join(s.T().TempDir(), "catalog.db"), selfID)
	s.Require().NoError(err)
	s.store = store
	s.index = newStubIndex()
}

func (s *WatcherSuite) TearDownTest() {
	s.store.Close()
}

func (s *WatcherSuite) TestStartupScan() {
	os.WriteFile(filepath.Join(s.dir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(s.dir, "b.txt"), []byte("bbb"), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	w := watcher.New(s.dir, s.store, s.index, nil)

	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	s.Eventually(func() bool {
		entries, _ := s.store.ListLocal()
		return len(entries) == 2
	}, 2*time.Second, 50*time.Millisecond)

	cancel()
	s.Require().NoError(<-done)
	s.Len(s.index.added, 2)
}

func (s *WatcherSuite) TestFileCreate() {
	ctx, cancel := context.WithCancel(context.Background())
	w := watcher.New(s.dir, s.store, s.index, nil)

	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	time.Sleep(100 * time.Millisecond) // let watcher start
	os.WriteFile(filepath.Join(s.dir, "new.txt"), []byte("hello"), 0644)

	s.Eventually(func() bool {
		entries, _ := s.store.ListLocal()
		return len(entries) == 1
	}, 3*time.Second, 50*time.Millisecond)

	entries, _ := s.store.ListLocal()
	s.Equal("new.txt", entries[0].Name)

	cancel()
	s.Require().NoError(<-done)
}

func (s *WatcherSuite) TestFileModify() {
	path := filepath.Join(s.dir, "modify.txt")
	os.WriteFile(path, []byte("original"), 0644)
	originalHash := sha256.Sum256([]byte("original"))

	ctx, cancel := context.WithCancel(context.Background())
	w := watcher.New(s.dir, s.store, s.index, nil)

	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	s.Eventually(func() bool {
		entries, _ := s.store.ListLocal()
		return len(entries) == 1
	}, 2*time.Second, 50*time.Millisecond)

	os.WriteFile(path, []byte("modified"), 0644)
	modifiedHash := sha256.Sum256([]byte("modified"))

	s.Eventually(func() bool {
		entry, _ := s.store.GetEntry(selfID, modifiedHash)
		return entry != nil
	}, 3*time.Second, 50*time.Millisecond)

	s.NotEqual(originalHash, modifiedHash)

	cancel()
	s.Require().NoError(<-done)
}

func (s *WatcherSuite) TestFileDelete() {
	path := filepath.Join(s.dir, "delete.txt")
	os.WriteFile(path, []byte("doomed"), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	w := watcher.New(s.dir, s.store, s.index, nil)

	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	s.Eventually(func() bool {
		entries, _ := s.store.ListLocal()
		return len(entries) == 1
	}, 2*time.Second, 50*time.Millisecond)

	os.Remove(path)

	s.Eventually(func() bool {
		entries, _ := s.store.ListLocal()
		return len(entries) == 0
	}, 3*time.Second, 50*time.Millisecond)

	cancel()
	s.Require().NoError(<-done)
	s.True(s.index.removed[sha256.Sum256([]byte("doomed"))])
}

func (s *WatcherSuite) TestContextCancel() {
	ctx, cancel := context.WithCancel(context.Background())
	w := watcher.New(s.dir, s.store, s.index, nil)

	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	s.Require().NoError(<-done)
}

func TestWatcherSuite(t *testing.T) {
	suite.Run(t, new(WatcherSuite))
}

func hashFileSync(path string) [32]byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return [32]byte{}
	}
	return sha256.Sum256(data)
}
