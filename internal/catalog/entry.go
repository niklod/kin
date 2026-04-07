// Package catalog implements a persistent file catalog backed by bbolt.
//
// The catalog tracks files owned locally (in the shared folder) and files
// reported by peers during catalog exchange. Each entry is keyed by
// (owner_node_id, file_id) so the same content hash from different owners
// produces distinct records.
package catalog

import "time"

// Entry represents a single file known to the catalog.
type Entry struct {
	FileID      [32]byte  `json:"file_id"`
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
	OwnerNodeID [32]byte  `json:"owner_node_id"`
	LocalPath   string    `json:"local_path,omitempty"` // non-empty only for local files
	Deleted     bool      `json:"deleted,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// OwnerHint records that a specific peer has a copy of a file.
type OwnerHint struct {
	NodeID [32]byte  `json:"node_id"`
	SeenAt time.Time `json:"seen_at"`
}
