// Package nat implements TCP simultaneous open for NAT hole punching.
//
// Both peers receive a RelayRendezvous message and call Punch at the same time.
// Each side binds its local kin listen port (SO_REUSEPORT) and dials outward to
// the peer's external address. On cone NATs the outbound SYN opens a mapping that
// the peer's simultaneous SYN traverses, completing the TCP handshake.
//
// Symmetric NAT is not supported; Punch returns an error in that case.
package nat

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/transport"
)

// ErrPunchFailed is returned when the hole punch dial does not succeed.
var ErrPunchFailed = errors.New("nat: hole punch failed")

// Punch dials peerAddr from localPort using SO_REUSEPORT, then wraps the
// resulting connection in mutual TLS verified against peerNodeID.
//
// Pass a context with a deadline (≤5 s recommended) to bound the attempt.
// On platforms without SO_REUSEPORT (Windows) the dial proceeds without
// port reuse; traversal still works if the local node is not behind NAT.
func Punch(ctx context.Context, localPort uint32, peerAddr string, id *identity.Identity, peerNodeID [32]byte) (*transport.Conn, error) {
	dialer := net.Dialer{
		LocalAddr: &net.TCPAddr{Port: int(localPort)},
		Control:   dialControl,
	}
	rawConn, err := dialer.DialContext(ctx, "tcp", peerAddr)
	if err != nil {
		return nil, fmt.Errorf("%w: dial %s: %v", ErrPunchFailed, peerAddr, err)
	}
	conn, err := transport.DialConn(rawConn, id, peerNodeID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPunchFailed, err)
	}
	return conn, nil
}
