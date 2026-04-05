package transport

import (
	"context"
	"fmt"
	"net"

	"github.com/quic-go/quic-go"

	"github.com/niklod/kin/internal/identity"
)

// Dial establishes a QUIC connection to addr, verifying that the remote peer's
// NodeID matches expectedNodeID. Uses a background context; prefer DialContext
// when a deadline or cancellation is needed.
//
// This function opens a fresh UDP socket. When NAT hole punching is required,
// use Listener.Dial instead so the connection shares the listener's UDP port.
func Dial(addr string, id *identity.Identity, expectedNodeID [32]byte) (*Conn, error) {
	return DialContext(context.Background(), addr, id, expectedNodeID)
}

// DialContext establishes a QUIC connection to addr with context support,
// verifying the remote peer's NodeID matches expectedNodeID.
//
// This function opens a fresh UDP socket. When NAT hole punching is required,
// use Listener.Dial instead so the connection shares the listener's UDP port.
func DialContext(ctx context.Context, addr string, id *identity.Identity, expectedNodeID [32]byte) (*Conn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", addr, err)
	}

	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("dial: open udp socket: %w", err)
	}

	cert, err := certFromIdentity(id, "dial")
	if err != nil {
		_ = udpConn.Close()
		return nil, err
	}

	qt := &quic.Transport{Conn: udpConn}
	qConn, err := qt.Dial(ctx, udpAddr, clientTLSConfig(cert, expectedNodeID), defaultQUICConfig())
	if err != nil {
		_ = qt.Close()
		return nil, fmt.Errorf("quic dial %s: %w", addr, err)
	}

	stream, err := openDialStream(ctx, qConn)
	if err != nil {
		_ = qt.Close()
		return nil, err
	}

	return newOwnedConn(qConn, stream, expectedNodeID, qt), nil
}
