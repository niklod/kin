package transfer_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/niklod/kin/internal/transfer"
	"github.com/niklod/kin/testutil"
)

func TestSmallFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	hash, path := testutil.WriteFile(t, srcDir, "hello.txt", "hello world")

	idx := transfer.NewLocalIndex()
	if err := idx.Add(path); err != nil {
		t.Fatalf("Add: %v", err)
	}

	sender := transfer.NewSender(idx)
	senderConn, receiverConn := testutil.NewMemConnPair("", 32)

	ctx := context.Background()
	go func() {
		if err := sender.HandleRequest(ctx, hash, senderConn); err != nil {
			t.Errorf("HandleRequest: %v", err)
		}
	}()

	outPath, err := transfer.RequestFile(ctx, hash, dstDir, receiverConn)
	if err != nil {
		t.Fatalf("RequestFile: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("content = %q, want %q", data, "hello world")
	}
}

func TestMultiChunkFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	content := strings.Repeat("A", 200*1024) // 200 KiB
	hash, path := testutil.WriteFile(t, srcDir, "big.bin", content)

	idx := transfer.NewLocalIndex()
	idx.Add(path)
	sender := transfer.NewSender(idx)
	senderConn, receiverConn := testutil.NewMemConnPair("", 32)

	ctx := context.Background()
	go sender.HandleRequest(ctx, hash, senderConn)

	outPath, err := transfer.RequestFile(ctx, hash, dstDir, receiverConn)
	if err != nil {
		t.Fatalf("RequestFile: %v", err)
	}

	got, _ := os.ReadFile(outPath)
	if !bytes.Equal(got, []byte(content)) {
		t.Error("multi-chunk file content mismatch")
	}
}

func TestFileNotFound(t *testing.T) {
	dstDir := t.TempDir()
	idx := transfer.NewLocalIndex()
	sender := transfer.NewSender(idx)
	senderConn, receiverConn := testutil.NewMemConnPair("", 32)

	missingHash := [32]byte{0xFF}
	ctx := context.Background()
	go sender.HandleRequest(ctx, missingHash, senderConn)

	_, err := transfer.RequestFile(ctx, missingHash, dstDir, receiverConn)
	if !errors.Is(err, transfer.ErrFileNotFound) {
		t.Errorf("error = %v, want ErrFileNotFound", err)
	}
}

func TestHashMismatch(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	_, path := testutil.WriteFile(t, srcDir, "real.txt", "real content")

	// Index the file under a wrong hash to trigger mismatch.
	wrongHash := [32]byte{0xAA}
	fakeIdx := transfer.NewLocalIndex()
	fakeIdx.AddWithHash(wrongHash, path)
	fakeSender := transfer.NewSender(fakeIdx)

	senderConn, receiverConn := testutil.NewMemConnPair("", 32)
	ctx := context.Background()
	go fakeSender.HandleRequest(ctx, wrongHash, senderConn)

	_, err := transfer.RequestFile(ctx, wrongHash, dstDir, receiverConn)
	if !errors.Is(err, transfer.ErrHashMismatch) {
		t.Errorf("error = %v, want ErrHashMismatch", err)
	}
}

func TestIdempotentRequest(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	hash, path := testutil.WriteFile(t, srcDir, "idem.txt", "idempotent")
	idx := transfer.NewLocalIndex()
	idx.Add(path)
	sender := transfer.NewSender(idx)

	ctx := context.Background()
	var lastPath string
	for i := 0; i < 2; i++ {
		s, r := testutil.NewMemConnPair("", 32)
		go sender.HandleRequest(ctx, hash, s)
		outPath, err := transfer.RequestFile(ctx, hash, dstDir, r)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		lastPath = outPath
	}
	_ = lastPath
}

func TestLocalIndex_Scan(t *testing.T) {
	dir := t.TempDir()
	content := "scan test"
	testutil.WriteFile(t, dir, "a.txt", content)
	testutil.WriteFile(t, dir, "b.txt", "other")

	idx := transfer.NewLocalIndex()
	if err := idx.Scan(dir); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	hash, _ := testutil.WriteFile(t, t.TempDir(), "x", content)
	path, ok := idx.Lookup(hash)
	if !ok {
		t.Fatal("Lookup: file not found after scan")
	}
	if !strings.HasSuffix(path, "a.txt") {
		t.Errorf("path = %q, want suffix a.txt", path)
	}
}

func TestContextCancellation(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	content := strings.Repeat("B", 500*1024)
	hash, path := testutil.WriteFile(t, srcDir, "large.bin", content)
	idx := transfer.NewLocalIndex()
	idx.Add(path)
	sender := transfer.NewSender(idx)

	senderConn, receiverConn := testutil.NewMemConnPair("", 32)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	go sender.HandleRequest(ctx, hash, senderConn)
	_, _ = transfer.RequestFile(ctx, hash, dstDir, receiverConn)
}
