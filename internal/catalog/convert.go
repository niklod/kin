package catalog

import (
	"fmt"
	"time"

	"github.com/niklod/kin/kinpb"
)

// EntryToProto converts a catalog Entry to its protobuf representation.
func EntryToProto(e *Entry) *kinpb.CatalogFile {
	return &kinpb.CatalogFile{
		FileId:      e.FileID[:],
		Name:        e.Name,
		Size:        e.Size,
		ModTimeUnix: e.ModTime.Unix(),
		OwnerNodeId: e.OwnerNodeID[:],
		Deleted:     e.Deleted,
	}
}

// ProtoToEntry converts a protobuf CatalogFile to a catalog Entry.
// It returns an error if FileId or OwnerNodeId are not exactly 32 bytes.
func ProtoToEntry(f *kinpb.CatalogFile) (*Entry, error) {
	if len(f.FileId) != 32 {
		return nil, fmt.Errorf("file_id must be 32 bytes, got %d", len(f.FileId))
	}
	if len(f.OwnerNodeId) != 32 {
		return nil, fmt.Errorf("owner_node_id must be 32 bytes, got %d", len(f.OwnerNodeId))
	}
	var fileID, ownerNodeID [32]byte
	copy(fileID[:], f.FileId)
	copy(ownerNodeID[:], f.OwnerNodeId)
	return &Entry{
		FileID:      fileID,
		Name:        f.Name,
		Size:        f.Size,
		ModTime:     time.Unix(f.ModTimeUnix, 0).UTC(),
		OwnerNodeID: ownerNodeID,
		Deleted:     f.Deleted,
	}, nil
}

// EntriesToProto converts a slice of catalog entries to protobuf CatalogFile messages.
func EntriesToProto(entries []*Entry) []*kinpb.CatalogFile {
	files := make([]*kinpb.CatalogFile, 0, len(entries))
	for _, e := range entries {
		files = append(files, EntryToProto(e))
	}
	return files
}
