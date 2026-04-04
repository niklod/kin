package identity_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/niklod/kin/internal/identity"
)

func TestGenerate(t *testing.T) {
	id1, err := identity.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	id2, err := identity.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if id1.NodeID == id2.NodeID {
		t.Error("two generated identities have identical NodeIDs")
	}
	if len(id1.PubKey) != 32 {
		t.Errorf("PubKey length = %d, want 32", len(id1.PubKey))
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if err := identity.Save(id, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := identity.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if id.NodeID != loaded.NodeID {
		t.Errorf("NodeID mismatch after load: got %s, want %s",
			loaded.NodeIDHex(), id.NodeIDHex())
	}
	if string(id.PubKey) != string(loaded.PubKey) {
		t.Error("PubKey mismatch after load")
	}
}

func TestSave_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	id, _ := identity.Generate()

	if err := identity.Save(id, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "identity.key"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file permissions = %04o, want 0600", perm)
	}
}

func TestLoad_NotExist(t *testing.T) {
	_, err := identity.Load(t.TempDir())
	if !os.IsNotExist(err) {
		t.Errorf("Load missing file: got %v, want os.ErrNotExist", err)
	}
}

func TestLoad_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "identity.key"), []byte("short"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := identity.Load(dir)
	if err == nil {
		t.Error("Load corrupt file: expected error, got nil")
	}
}

func TestLoadOrGenerate_Idempotent(t *testing.T) {
	dir := t.TempDir()

	id1, err := identity.LoadOrGenerate(dir)
	if err != nil {
		t.Fatalf("first LoadOrGenerate: %v", err)
	}
	id2, err := identity.LoadOrGenerate(dir)
	if err != nil {
		t.Fatalf("second LoadOrGenerate: %v", err)
	}
	if id1.NodeID != id2.NodeID {
		t.Error("NodeID changed between LoadOrGenerate calls")
	}
}

func TestLoadOrGenerate_CreatesNew(t *testing.T) {
	dir := t.TempDir()

	id1, err := identity.LoadOrGenerate(dir)
	if err != nil {
		t.Fatalf("first LoadOrGenerate: %v", err)
	}

	// Remove the key file and regenerate.
	if err := os.Remove(filepath.Join(dir, "identity.key")); err != nil {
		t.Fatal(err)
	}

	id2, err := identity.LoadOrGenerate(dir)
	if err != nil {
		t.Fatalf("second LoadOrGenerate after deletion: %v", err)
	}
	if id1.NodeID == id2.NodeID {
		t.Error("NodeID should differ after key file deletion")
	}
}

func TestNodeIDHex(t *testing.T) {
	id, _ := identity.Generate()
	hex1 := id.NodeIDHex()
	hex2 := id.NodeIDHex()
	if hex1 != hex2 {
		t.Error("NodeIDHex not deterministic")
	}
	if len(hex1) != 64 {
		t.Errorf("NodeIDHex length = %d, want 64", len(hex1))
	}
}
