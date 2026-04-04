package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/niklod/kin/internal/identity"
)

// Dial establishes a mutual TLS connection to addr, verifying that the server's
// NodeID matches expectedNodeID. Uses a background context; prefer DialContext
// when a deadline or cancellation is needed.
func Dial(addr string, id *identity.Identity, expectedNodeID [32]byte) (*Conn, error) {
	return DialContext(context.Background(), addr, id, expectedNodeID)
}

// DialContext establishes a mutual TLS connection to addr with context support,
// verifying the server's NodeID matches expectedNodeID.
func DialContext(ctx context.Context, addr string, id *identity.Identity, expectedNodeID [32]byte) (*Conn, error) {
	cert, err := certFromIdentity(id, "dial")
	if err != nil {
		return nil, err
	}
	cfg := clientTLSConfig(cert, expectedNodeID)
	dialer := tls.Dialer{Config: cfg}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return newConn(conn.(*tls.Conn), expectedNodeID), nil
}

// DialConn wraps an existing net.Conn in mutual TLS client mode,
// verifying the server's NodeID matches expectedNodeID.
// rawConn is closed on any error.
func DialConn(rawConn net.Conn, id *identity.Identity, expectedNodeID [32]byte) (*Conn, error) {
	cert, err := certFromIdentity(id, "dial-conn")
	if err != nil {
		rawConn.Close()
		return nil, err
	}
	cfg := clientTLSConfig(cert, expectedNodeID)
	tlsConn := tls.Client(rawConn, cfg)
	if err := tlsConn.Handshake(); err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("dial-conn: TLS handshake: %w", err)
	}
	return newConn(tlsConn, expectedNodeID), nil
}
