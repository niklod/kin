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
	"log/slog"
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
	slog.Debug("nat: punch attempt",
		"local_port", localPort,
		"peer_addr", peerAddr,
		"peer_node_id", fmt.Sprintf("%x", peerNodeID[:8]))

	dialer := net.Dialer{
		LocalAddr: &net.TCPAddr{Port: int(localPort)},
		Control:   dialControl,
	}
	rawConn, err := dialer.DialContext(ctx, "tcp", peerAddr)
	if err != nil {
		slog.Debug("nat: TCP dial failed", "peer_addr", peerAddr, "err", err)
		return nil, fmt.Errorf("%w: dial %s: %v", ErrPunchFailed, peerAddr, err)
	}

	slog.Debug("nat: TCP dial OK",
		"local_addr", rawConn.LocalAddr(),
		"remote_addr", rawConn.RemoteAddr())

	conn, err := transport.DialConn(rawConn, id, peerNodeID)
	if err != nil {
		slog.Debug("nat: TLS handshake failed", "err", err)
		return nil, fmt.Errorf("%w: %v", ErrPunchFailed, err)
	}

	slog.Debug("nat: TLS handshake OK", "peer_node_id", fmt.Sprintf("%x", peerNodeID[:8]))
	return conn, nil
}
