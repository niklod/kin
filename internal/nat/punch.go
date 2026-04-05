// Package nat implements UDP hole punching for NAT traversal.
//
// Both peers receive a RelayRendezvous message and call Punch at the same time.
// Each side sends a burst of UDP prime packets from the shared listener socket,
// opening NAT mappings on both routers. Then the dialing side calls Listener.Dial
// via the same socket; on cone NATs the QUIC handshake completes end-to-end.
//
// Symmetric NAT is not supported; Punch returns ErrPunchFailed in that case.
package nat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/niklod/kin/internal/transport"
)

// ErrPunchFailed is returned when the hole punch dial does not succeed.
var ErrPunchFailed = errors.New("nat: hole punch failed")

const (
	primeBurst    = 5
	primeInterval = 20 * time.Millisecond
)

// PrimeNAT sends a burst of primeBurst UDP datagrams at primeInterval spacing
// to peerAddr via the listener's shared UDP socket, opening a NAT mapping so
// that inbound QUIC packets from peerAddr can reach us.
//
// It is fire-and-forget: errors are logged at debug level. Cancelling ctx stops
// the burst early.
func PrimeNAT(ctx context.Context, ln *transport.Listener, peerAddr string) {
	for i := 0; i < primeBurst; i++ {
		if ctx.Err() != nil {
			return
		}
		if err := ln.Prime(peerAddr); err != nil {
			slog.Debug("nat: prime write failed", "peer_addr", peerAddr, "i", i, "err", err)
		} else {
			slog.Debug("nat: prime sent", "peer_addr", peerAddr, "i", i)
		}
		if i < primeBurst-1 {
			t := time.NewTimer(primeInterval)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
		}
	}
}

// Punch sends a prime burst to open the local NAT mapping, then dials peerAddr
// from the listener's shared UDP socket, verifying the peer's NodeID.
//
// Pass a context with a deadline (≤5 s recommended) to bound the attempt.
func Punch(ctx context.Context, ln *transport.Listener, peerAddr string, peerNodeID [32]byte) (*transport.Conn, error) {
	slog.Debug("nat: punch attempt",
		"peer_addr", peerAddr,
		"peer_node_id", fmt.Sprintf("%x", peerNodeID[:8]))

	PrimeNAT(ctx, ln, peerAddr)

	conn, err := ln.Dial(ctx, peerAddr, peerNodeID)
	if err != nil {
		slog.Debug("nat: punch dial failed", "peer_addr", peerAddr, "err", err)
		return nil, fmt.Errorf("%w: dial %s: %v", ErrPunchFailed, peerAddr, err)
	}

	slog.Debug("nat: punch OK", "peer_node_id", fmt.Sprintf("%x", peerNodeID[:8]))
	return conn, nil
}
