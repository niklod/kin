// Package testutil provides shared helpers for Kin tests.
package testutil

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/niklod/kin/kinpb"
)

// WriteFile writes content to dir/name and returns its SHA-256 hash and path.
func WriteFile(t *testing.T, dir, name, content string) ([32]byte, string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", name, err)
	}
	return sha256.Sum256([]byte(content)), path
}

// MustMkdir creates a directory or fails the test.
func MustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("MkdirAll %s: %v", path, err)
	}
}

// MemConn is an in-memory bidirectional message pipe for testing.
// It satisfies the MsgReadWriter, MsgWriter, and protocol.Conn interfaces.
type MemConn struct {
	send chan *kinpb.Envelope
	recv chan *kinpb.Envelope
	addr string
}

// NewMemConnPair returns two connected MemConns.
// bufSize controls per-channel buffer capacity.
func NewMemConnPair(addr string, bufSize int) (a, b *MemConn) {
	ch1 := make(chan *kinpb.Envelope, bufSize)
	ch2 := make(chan *kinpb.Envelope, bufSize)
	return &MemConn{send: ch1, recv: ch2, addr: addr},
		&MemConn{send: ch2, recv: ch1, addr: addr}
}

// Send puts env onto the outgoing channel.
func (m *MemConn) Send(env *kinpb.Envelope) error {
	m.send <- env
	return nil
}

// Recv blocks until an envelope is available or the channel is closed.
func (m *MemConn) Recv() (*kinpb.Envelope, error) {
	env, ok := <-m.recv
	if !ok {
		return nil, io.EOF
	}
	return env, nil
}

// RemoteAddr returns the address string provided at construction.
func (m *MemConn) RemoteAddr() string { return m.addr }

// CloseRecv closes the receive channel, causing subsequent Recv calls to return io.EOF.
func (m *MemConn) CloseRecv() { close(m.recv) }

// SendChan exposes the send channel for direct manipulation in tests.
func (m *MemConn) SendChan() chan *kinpb.Envelope { return m.send }

// RecvChan exposes the recv channel for direct manipulation in tests.
func (m *MemConn) RecvChan() chan *kinpb.Envelope { return m.recv }

// Drain reads and discards all buffered envelopes, returning their count.
func (m *MemConn) Drain() int {
	n := 0
	for {
		select {
		case _, ok := <-m.recv:
			if !ok {
				return n
			}
			n++
		default:
			return n
		}
	}
}

// Ensure MemConn satisfies common interface shapes at compile time.
var _ interface {
	Send(*kinpb.Envelope) error
	Recv() (*kinpb.Envelope, error)
	RemoteAddr() string
} = (*MemConn)(nil)

// Logf wraps t.Logf with a prefix — convenience for verbose test logging.
func Logf(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Logf("[testutil] "+format, args...)
	_ = fmt.Sprintf // keep import alive
}
