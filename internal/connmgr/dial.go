// Package connmgr implements the connection establishment strategy for Kin:
// direct TCP first, relay-assisted NAT hole punching second.
//
// Relay endpoints in the invite token use the "relay://host:port" scheme.
// When direct TCP fails, the Dialer connects to the relay, requests a rendezvous,
// and uses nat.Punch for TCP simultaneous open to traverse NAT.
package connmgr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/nat"
	"github.com/niklod/kin/internal/relay"
	"github.com/niklod/kin/internal/transport"
)

const punchTimeout = 5 * time.Second

// ErrNoRoute is returned when all connection attempts fail.
var ErrNoRoute = errors.New("connmgr: no route to peer")

// Dialer establishes outbound connections to peers.
type Dialer struct {
	ID        *identity.Identity
	LocalPort uint32 // kin listen port, used as source port for NAT punch
}

// Dial connects to a peer identified by peerNodeID, trying each strategy in order:
//  1. Direct TCP to non-relay endpoints.
//  2. Relay-assisted NAT hole punch to relay:// endpoints.
//
// Returns ErrNoRoute if all strategies fail.
func (d *Dialer) Dial(ctx context.Context, peerNodeID [32]byte, endpoints []string) (*transport.Conn, error) {
	direct, relayAddrs := splitEndpoints(endpoints)

	slog.Debug("connmgr: dial start",
		"peer", fmt.Sprintf("%x", peerNodeID[:8]),
		"direct_endpoints", direct,
		"relay_endpoints", relayAddrs,
		"local_port", d.LocalPort)

	// 1. Direct TCP.
	if conn, err := tryEach(direct, func(ep string) (*transport.Conn, error) {
		slog.Debug("connmgr: trying direct TCP", "addr", ep)
		c, e := transport.DialContext(ctx, ep, d.ID, peerNodeID)
		if e != nil {
			slog.Debug("connmgr: direct TCP failed", "addr", ep, "err", e)
		}
		return c, e
	}); err == nil {
		slog.Debug("connmgr: direct TCP connected")
		return conn, nil
	}

	// 2. Relay + NAT punch.
	if conn, err := tryEach(relayAddrs, func(addr string) (*transport.Conn, error) {
		slog.Debug("connmgr: trying relay punch", "relay", addr)
		c, e := d.punchViaRelay(ctx, peerNodeID, addr)
		if e != nil {
			slog.Debug("connmgr: relay punch failed", "relay", addr, "err", e)
		}
		return c, e
	}); err == nil {
		slog.Debug("connmgr: relay punch connected")
		return conn, nil
	}

	slog.Debug("connmgr: no route to peer", "peer", fmt.Sprintf("%x", peerNodeID[:8]))
	return nil, ErrNoRoute
}

func (d *Dialer) punchViaRelay(ctx context.Context, peerNodeID [32]byte, relayAddr string) (*transport.Conn, error) {
	slog.Debug("connmgr: connecting to relay", "relay", relayAddr)
	rc, err := relay.Connect(ctx, relayAddr, d.ID, d.LocalPort)
	if err != nil {
		return nil, fmt.Errorf("relay %s: %w", relayAddr, err)
	}
	defer rc.Close()

	slog.Debug("connmgr: relay connected", "relay", relayAddr, "external", rc.ExternalAddr())

	punchCtx, cancel := context.WithTimeout(ctx, punchTimeout)
	defer cancel()

	slog.Debug("connmgr: requesting rendezvous", "peer", fmt.Sprintf("%x", peerNodeID[:8]))
	rv, err := rc.RequestRendezvous(punchCtx, peerNodeID)
	if err != nil {
		return nil, fmt.Errorf("rendezvous: %w", err)
	}

	slog.Debug("connmgr: rendezvous received",
		"peer", fmt.Sprintf("%x", rv.PeerNodeID[:8]),
		"peer_external_addr", rv.PeerExternalAddr)

	slog.Debug("connmgr: punching",
		"peer_addr", rv.PeerExternalAddr,
		"local_port", d.LocalPort)

	conn, err := nat.Punch(punchCtx, d.LocalPort, rv.PeerExternalAddr, d.ID, peerNodeID)
	if err != nil {
		return nil, fmt.Errorf("punch: %w", err)
	}

	slog.Debug("connmgr: punch succeeded", "peer_addr", rv.PeerExternalAddr)
	return conn, nil
}

// tryEach iterates addrs, calls try for each, and returns the first successful
// connection. Returns ErrNoRoute if addrs is empty; returns the last error otherwise.
func tryEach(addrs []string, try func(string) (*transport.Conn, error)) (*transport.Conn, error) {
	var lastErr error
	for _, addr := range addrs {
		conn, err := try(addr)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNoRoute
}

// splitEndpoints partitions endpoints into plain TCP addresses and relay addresses.
// Relay addresses are returned stripped of the "relay://" prefix.
func splitEndpoints(endpoints []string) (direct, relayAddrs []string) {
	for _, ep := range endpoints {
		if strings.HasPrefix(ep, "relay://") {
			relayAddrs = append(relayAddrs, strings.TrimPrefix(ep, "relay://"))
		} else {
			direct = append(direct, ep)
		}
	}
	return direct, relayAddrs
}
