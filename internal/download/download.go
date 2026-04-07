// Package download implements catalog-aware file retrieval from peers.
package download

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/niklod/kin/internal/catalog"
	"github.com/niklod/kin/internal/transfer"
)

// ErrNoAvailableOwner is returned when no peer with the requested file can be reached.
var ErrNoAvailableOwner = errors.New("download: no available owner for file")

// CatalogReader provides file ownership information.
type CatalogReader interface {
	GetOwners(fileID [32]byte) ([]catalog.OwnerHint, error)
	GetEntry(ownerNodeID, fileID [32]byte) (*catalog.Entry, error)
}

// PeerDialer establishes a connection to a peer.
type PeerDialer interface {
	Dial(ctx context.Context, peerNodeID [32]byte) (transfer.MsgReadWriter, error)
}

// FileIndexer registers downloaded files for serving.
type FileIndexer interface {
	Add(path string) error
}

// Downloader fetches files from peers using catalog availability hints.
type Downloader struct {
	catalog   CatalogReader
	dialer    PeerDialer
	index     FileIndexer
	sharedDir string
	selfID    [32]byte
	logger    *slog.Logger
}

// New creates a Downloader.
func New(
	cat CatalogReader, dialer PeerDialer, index FileIndexer,
	sharedDir string, selfID [32]byte, logger *slog.Logger,
) *Downloader {
	if logger == nil {
		logger = slog.Default()
	}
	return &Downloader{
		catalog:   cat,
		dialer:    dialer,
		index:     index,
		sharedDir: sharedDir,
		selfID:    selfID,
		logger:    logger,
	}
}

// Download fetches a file identified by fileID from available peers.
// It tries each owner in order (most recently seen first) until one succeeds.
// On success the file is saved to the shared directory and registered in the
// local index.
func (d *Downloader) Download(ctx context.Context, fileID [32]byte) (string, error) {
	owners, err := d.catalog.GetOwners(fileID)
	if err != nil {
		return "", fmt.Errorf("download get owners: %w", err)
	}

	owners = filterSelf(owners, d.selfID)
	if len(owners) == 0 {
		return "", ErrNoAvailableOwner
	}

	sort.Slice(owners, func(i, j int) bool {
		return owners[i].SeenAt.After(owners[j].SeenAt)
	})

	var lastErr error
	for _, owner := range owners {
		path, err := d.tryOwner(ctx, fileID, owner.NodeID)
		if err != nil {
			d.logger.Debug("download attempt failed",
				"owner", fmt.Sprintf("%x", owner.NodeID[:8]),
				"err", err)
			lastErr = err
			continue
		}
		return path, nil
	}

	return "", fmt.Errorf("%w: last error: %v", ErrNoAvailableOwner, lastErr)
}

func (d *Downloader) tryOwner(ctx context.Context, fileID [32]byte, ownerNodeID [32]byte) (string, error) {
	conn, err := d.dialer.Dial(ctx, ownerNodeID)
	if err != nil {
		return "", fmt.Errorf("dial %x: %w", ownerNodeID[:8], err)
	}

	destPath, err := transfer.RequestFile(ctx, fileID, d.sharedDir, conn)
	if err != nil {
		return "", fmt.Errorf("request file: %w", err)
	}

	// Try to rename to original filename from catalog.
	finalPath := d.renameToOriginal(fileID, ownerNodeID, destPath)

	if err := d.index.Add(finalPath); err != nil {
		d.logger.Warn("index downloaded file", "path", finalPath, "err", err)
	}

	d.logger.Info("downloaded file",
		"file_id", fmt.Sprintf("%x", fileID[:8]),
		"path", finalPath)
	return finalPath, nil
}

func (d *Downloader) renameToOriginal(fileID, ownerNodeID [32]byte, hashPath string) string {
	entry, err := d.catalog.GetEntry(ownerNodeID, fileID)
	if err != nil || entry == nil || entry.Name == "" {
		return hashPath
	}

	// Use only the base name to prevent path traversal via peer-supplied names.
	safeName := filepath.Base(entry.Name)
	if safeName == "." || safeName == string(os.PathSeparator) {
		return hashPath
	}
	target := filepath.Join(d.sharedDir, safeName)
	if _, err := os.Stat(target); err == nil {
		// Name collision — keep hash-based name.
		return hashPath
	}

	if err := os.Rename(hashPath, target); err != nil {
		return hashPath
	}
	return target
}

func filterSelf(owners []catalog.OwnerHint, selfID [32]byte) []catalog.OwnerHint {
	filtered := make([]catalog.OwnerHint, 0, len(owners))
	for _, o := range owners {
		if o.NodeID != selfID {
			filtered = append(filtered, o)
		}
	}
	return filtered
}
