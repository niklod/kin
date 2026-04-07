package transport

import (
	"fmt"
	"sync"

	"github.com/quic-go/quic-go"

	"github.com/niklod/kin/internal/protocol"
	"github.com/niklod/kin/kinpb"
)

// Conn wraps a QUIC stream and provides length-prefixed protobuf framing.
// PeerNodeID is extracted from the TLS handshake and is immutable after creation.
type Conn struct {
	qConn      *quic.Conn
	stream     *quic.Stream
	PeerNodeID [32]byte

	// ownedTransport is non-nil only for connections created by standalone
	// Dial/DialContext (not via Listener.Dial). It is closed asynchronously
	// once the QUIC connection goes away.
	ownedTransport *quic.Transport

	closeOnce sync.Once
	closeErr  error
}

func newConn(qConn *quic.Conn, stream *quic.Stream, peerNodeID [32]byte) *Conn {
	return &Conn{qConn: qConn, stream: stream, PeerNodeID: peerNodeID}
}

func newOwnedConn(qConn *quic.Conn, stream *quic.Stream, peerNodeID [32]byte, owned *quic.Transport) *Conn {
	c := &Conn{qConn: qConn, stream: stream, PeerNodeID: peerNodeID, ownedTransport: owned}
	// Close the transport once the QUIC connection ends (idle timeout, error,
	// or explicit close by either peer). This runs in the background and does
	// not block the caller.
	go func() {
		<-qConn.Context().Done()
		_ = owned.Close()
	}()
	return c
}

// Send serialises and sends a protobuf Envelope over the connection.
func (c *Conn) Send(env *kinpb.Envelope) error {
	if err := protocol.WriteMsg(c.stream, env); err != nil {
		return fmt.Errorf("conn send: %w", err)
	}
	return nil
}

// Recv reads and deserialises the next protobuf Envelope from the connection.
func (c *Conn) Recv() (*kinpb.Envelope, error) {
	env, err := protocol.ReadMsg(c.stream)
	if err != nil {
		return nil, fmt.Errorf("conn recv: %w", err)
	}
	return env, nil
}

// Close sends a graceful FIN on the stream. Callers must not call Send or Recv
// after Close returns.
//
// The underlying QUIC connection is left open until it becomes idle (no active
// streams) and is cleaned up by the transport's idle timeout. For standalone
// Dial connections the owned transport is closed in the background once the
// connection ends.
func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.stream.Close()
	})
	return c.closeErr
}

// RemoteAddr returns the remote network address as a string.
func (c *Conn) RemoteAddr() string {
	return c.qConn.RemoteAddr().String()
}

// PeerID returns the authenticated peer's NodeID.
func (c *Conn) PeerID() [32]byte {
	return c.PeerNodeID
}
