// Package watcher monitors the shared folder for file changes and keeps the
// catalog store and in-memory file index up to date.
package watcher

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/niklod/kin/internal/catalog"
)

const debounceDelay = 500 * time.Millisecond

// CatalogWriter is the consumer-side interface for catalog mutations.
type CatalogWriter interface {
	PutLocal(entry *catalog.Entry) error
	DeleteLocal(fileID [32]byte) error
	LookupLocalByPath(path string) (*catalog.Entry, error)
}

// LocalIndexer is the consumer-side interface for the in-memory file index.
type LocalIndexer interface {
	Add(path string) error
	Remove(hash [32]byte)
}

// Watcher monitors a directory for filesystem events and updates the catalog
// and local index accordingly.
type Watcher struct {
	dir      string
	catalog  CatalogWriter
	index    LocalIndexer
	logger   *slog.Logger
	onChange func()
}

// New creates a Watcher for dir. The optional onChange callback is invoked
// after each catalog mutation (file indexed or removed).
func New(dir string, cat CatalogWriter, index LocalIndexer, logger *slog.Logger, onChange func()) *Watcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Watcher{
		dir:      dir,
		catalog:  cat,
		index:    index,
		logger:   logger,
		onChange: onChange,
	}
}

// Run performs an initial full scan, then watches for changes until ctx is
// cancelled. It blocks until shutdown.
func (w *Watcher) Run(ctx context.Context) error {
	if err := w.scan(); err != nil {
		return fmt.Errorf("watcher scan: %w", err)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watcher init: %w", err)
	}
	defer fsw.Close()

	if err := fsw.Add(w.dir); err != nil {
		return fmt.Errorf("watcher add: %w", err)
	}

	w.logger.Info("watcher started", "dir", w.dir)
	return w.loop(ctx, fsw)
}

func (w *Watcher) scan() error {
	return filepath.WalkDir(w.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if err := w.indexFile(path); err != nil {
			w.logger.Warn("scan index file", "path", path, "err", err)
		}
		return nil
	})
}

func (w *Watcher) loop(ctx context.Context, fsw *fsnotify.Watcher) error {
	db := newDebouncer(w)

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-fsw.Events:
			if !ok {
				return nil
			}
			w.handleEvent(ev, db)
		case err, ok := <-fsw.Errors:
			if !ok {
				return nil
			}
			w.logger.Warn("watcher error", "err", err)
		}
	}
}

func (w *Watcher) handleEvent(ev fsnotify.Event, db *debouncer) {
	switch {
	case ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write):
		db.schedule(ev.Name)
	case ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename):
		db.cancel(ev.Name)
		w.handleRemove(ev.Name)
	}
}

func (w *Watcher) indexFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}

	hash, err := hashFile(path)
	if err != nil {
		return fmt.Errorf("hash %s: %w", path, err)
	}

	entry := &catalog.Entry{
		FileID:    hash,
		Name:      filepath.Base(path),
		Size:      info.Size(),
		ModTime:   info.ModTime().UTC(),
		LocalPath: path,
	}
	if err := w.catalog.PutLocal(entry); err != nil {
		return fmt.Errorf("catalog put: %w", err)
	}
	if err := w.index.Add(path); err != nil {
		return fmt.Errorf("index add: %w", err)
	}

	w.logger.Debug("indexed file", "name", entry.Name, "size", entry.Size,
		"file_id", fmt.Sprintf("%x", hash[:8]))
	w.notifyChange()
	return nil
}

func (w *Watcher) handleRemove(path string) {
	entry, err := w.catalog.LookupLocalByPath(path)
	if err != nil {
		w.logger.Warn("watcher lookup removed file", "path", path, "err", err)
		return
	}
	if entry == nil {
		return
	}

	if err := w.catalog.DeleteLocal(entry.FileID); err != nil {
		w.logger.Warn("watcher delete", "path", path, "err", err)
		return
	}
	w.index.Remove(entry.FileID)
	w.logger.Debug("removed file", "name", entry.Name,
		"file_id", fmt.Sprintf("%x", entry.FileID[:8]))
	w.notifyChange()
}

// debouncer groups rapid filesystem events by path.
type debouncer struct {
	mu     sync.Mutex
	timers map[string]*time.Timer
	w      *Watcher
}

func newDebouncer(w *Watcher) *debouncer {
	return &debouncer{timers: make(map[string]*time.Timer), w: w}
}

func (d *debouncer) schedule(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if t, ok := d.timers[path]; ok {
		t.Stop()
		t.Reset(debounceDelay)
		return
	}
	d.timers[path] = time.AfterFunc(debounceDelay, func() {
		d.mu.Lock()
		delete(d.timers, path)
		d.mu.Unlock()

		if err := d.w.indexFile(path); err != nil {
			d.w.logger.Debug("watcher index", "path", path, "err", err)
		}
	})
}

func (d *debouncer) cancel(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if t, ok := d.timers[path]; ok {
		t.Stop()
		delete(d.timers, path)
	}
}

func (w *Watcher) notifyChange() {
	if w.onChange != nil {
		w.onChange()
	}
}

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
