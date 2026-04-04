package transport

import (
	"crypto/tls"
	"fmt"

	"github.com/niklod/kin/internal/protocol"
	"github.com/niklod/kin/kinpb"
)

// Conn wraps a TLS connection and provides length-prefixed protobuf framing.
type Conn struct {
	inner      *tls.Conn
	PeerNodeID [32]byte
}

func newConn(c *tls.Conn, peerNodeID [32]byte) *Conn {
	return &Conn{inner: c, PeerNodeID: peerNodeID}
}

// Send serialises and sends a protobuf Envelope over the connection.
func (c *Conn) Send(env *kinpb.Envelope) error {
	if err := protocol.WriteMsg(c.inner, env); err != nil {
		return fmt.Errorf("conn send: %w", err)
	}
	return nil
}

// Recv reads and deserialises the next protobuf Envelope from the connection.
func (c *Conn) Recv() (*kinpb.Envelope, error) {
	env, err := protocol.ReadMsg(c.inner)
	if err != nil {
		return nil, fmt.Errorf("conn recv: %w", err)
	}
	return env, nil
}

// Close closes the underlying TLS connection.
func (c *Conn) Close() error {
	return c.inner.Close()
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() string {
	return c.inner.RemoteAddr().String()
}
