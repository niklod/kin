package protocol_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/niklod/kin/internal/protocol"
	"github.com/niklod/kin/internal/transfer"
	"github.com/niklod/kin/kinpb"
	"github.com/niklod/kin/testutil"
)

func TestHandler_FileRequest(t *testing.T) {
	dir := t.TempDir()
	content := "integration test file"
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte(content), 0644)

	hash := sha256.Sum256([]byte(content))
	idx := transfer.NewLocalIndex()
	idx.Add(path)

	sender := transfer.NewSender(idx)
	handler := protocol.NewHandler(sender, nil)

	serverConn, clientConn := testutil.NewMemConnPair("127.0.0.1:0", 64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handler.Serve(ctx, serverConn)

	clientConn.Send(&kinpb.Envelope{Payload: &kinpb.Envelope_FileRequest{
		FileRequest: &kinpb.FileRequest{FileId: hash[:]},
	}})

	env, err := clientConn.Recv()
	if err != nil {
		t.Fatalf("Recv FileResponse: %v", err)
	}
	resp := env.GetFileResponse()
	if resp == nil {
		t.Fatalf("expected FileResponse, got %T", env.Payload)
	}
	if resp.NotFound {
		t.Fatal("FileResponse.not_found = true, want false")
	}

	var received []byte
	for {
		env, err := clientConn.Recv()
		if err != nil {
			t.Fatalf("Recv chunk: %v", err)
		}
		chunk := env.GetFileChunk()
		if chunk == nil {
			t.Fatalf("expected FileChunk, got %T", env.Payload)
		}
		received = append(received, chunk.Data...)
		if chunk.Eof {
			break
		}
	}

	if string(received) != content {
		t.Errorf("received %q, want %q", received, content)
	}
}

func TestHandler_FileRequest_NotFound(t *testing.T) {
	idx := transfer.NewLocalIndex()
	sender := transfer.NewSender(idx)
	handler := protocol.NewHandler(sender, nil)

	serverConn, clientConn := testutil.NewMemConnPair("127.0.0.1:0", 64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handler.Serve(ctx, serverConn)

	missingID := [32]byte{0xAB}
	clientConn.Send(&kinpb.Envelope{Payload: &kinpb.Envelope_FileRequest{
		FileRequest: &kinpb.FileRequest{FileId: missingID[:]},
	}})

	env, err := clientConn.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	resp := env.GetFileResponse()
	if resp == nil {
		t.Fatalf("expected FileResponse, got %T", env.Payload)
	}
	if !resp.NotFound {
		t.Error("FileResponse.not_found = false, want true")
	}
}

func TestHandler_InvalidFileID(t *testing.T) {
	idx := transfer.NewLocalIndex()
	sender := transfer.NewSender(idx)
	handler := protocol.NewHandler(sender, nil)

	serverConn, clientConn := testutil.NewMemConnPair("127.0.0.1:0", 64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handler.Serve(ctx, serverConn)

	clientConn.Send(&kinpb.Envelope{Payload: &kinpb.Envelope_FileRequest{
		FileRequest: &kinpb.FileRequest{FileId: []byte("short")},
	}})

	env, err := clientConn.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if env.GetError() == nil {
		t.Errorf("expected Error response, got %T", env.Payload)
	}
}

func TestHandler_ContextCancel(t *testing.T) {
	idx := transfer.NewLocalIndex()
	sender := transfer.NewSender(idx)
	handler := protocol.NewHandler(sender, nil)

	serverConn, _ := testutil.NewMemConnPair("127.0.0.1:0", 64)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		handler.Serve(ctx, serverConn)
		close(done)
	}()

	cancel()
	serverConn.CloseRecv()
	<-done
}

func TestHandler_EOF(t *testing.T) {
	idx := transfer.NewLocalIndex()
	sender := transfer.NewSender(idx)
	handler := protocol.NewHandler(sender, nil)

	serverConn, _ := testutil.NewMemConnPair("127.0.0.1:0", 64)
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		handler.Serve(ctx, serverConn)
		close(done)
	}()

	// Closing the recv channel makes Recv return io.EOF → Serve exits cleanly.
	serverConn.CloseRecv()
	<-done
}

func TestHandler_UnknownMessage(t *testing.T) {
	idx := transfer.NewLocalIndex()
	sender := transfer.NewSender(idx)
	handler := protocol.NewHandler(sender, nil)

	serverConn, clientConn := testutil.NewMemConnPair("127.0.0.1:0", 64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handler.Serve(ctx, serverConn)

	clientConn.Send(&kinpb.Envelope{Payload: &kinpb.Envelope_Handshake{
		Handshake: &kinpb.Handshake{},
	}})

	// Handler should log the unknown message and continue.
	dir := t.TempDir()
	content := "handler unknown msg test"
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte(content), 0644)
	hash := sha256.Sum256([]byte(content))
	idx.Add(path)

	clientConn.Send(&kinpb.Envelope{Payload: &kinpb.Envelope_FileRequest{
		FileRequest: &kinpb.FileRequest{FileId: hash[:]},
	}})
	env, err := clientConn.Recv()
	if err != nil {
		t.Fatalf("Recv after unknown msg: %v", err)
	}
	if env.GetFileResponse() == nil {
		t.Errorf("expected FileResponse after recovery, got %T", env.Payload)
	}
	fmt.Println("handler continued after unknown message")
}
