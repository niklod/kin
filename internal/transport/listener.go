package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/niklod/kin/internal/identity"
)

// Listener wraps a TLS listener and produces authenticated Conns.
type Listener struct {
	inner net.Listener
}

// Listen creates a TLS listener on addr, presenting the identity's certificate.
// On Linux and macOS, SO_REUSEPORT is set so the NAT punch goroutine can
// bind the same local port for an outgoing connection.
func Listen(addr string, id *identity.Identity) (*Listener, error) {
	cert, err := certFromIdentity(id, "listen")
	if err != nil {
		return nil, err
	}
	cfg := serverTLSConfig(cert)
	lc := listenConfig()
	rawLn, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	return &Listener{inner: tls.NewListener(rawLn, cfg)}, nil
}

// Accept waits for an incoming connection and returns a Conn plus the peer's NodeID.
func (l *Listener) Accept() (*Conn, [32]byte, error) {
	raw, err := l.inner.Accept()
	if err != nil {
		return nil, [32]byte{}, err
	}
	tlsConn, ok := raw.(*tls.Conn)
	if !ok {
		raw.Close()
		return nil, [32]byte{}, fmt.Errorf("accept: not a TLS connection")
	}
	// Force handshake to populate peer certificates.
	if err := tlsConn.Handshake(); err != nil {
		tlsConn.Close()
		return nil, [32]byte{}, fmt.Errorf("accept: TLS handshake: %w", err)
	}
	peerNodeID, err := NodeIDFromCert(tlsConn.ConnectionState().PeerCertificates)
	if err != nil {
		tlsConn.Close()
		return nil, [32]byte{}, fmt.Errorf("accept: extract peer NodeID: %w", err)
	}
	return newConn(tlsConn, peerNodeID), peerNodeID, nil
}

// Addr returns the listener's network address.
func (l *Listener) Addr() net.Addr {
	return l.inner.Addr()
}

// Close closes the listener.
func (l *Listener) Close() error {
	return l.inner.Close()
}
