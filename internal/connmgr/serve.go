package connmgr

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/niklod/kin/internal/nat"
	"github.com/niklod/kin/internal/relay"
)

// ServePunch connects to the relay, registers this node, and keeps the relay
// connection alive so that remote peers can discover this node's external address
// via RequestRendezvous and punch directly to this node's listener.
//
// When a RelayRendezvous arrives, this node primes its NAT immediately so the
// initiator's QUIC packets can reach the listener.
//
// Blocks until ctx is cancelled or the relay connection drops.
// The caller should restart ServePunch on error if continuous relay presence is needed.
func (d *Dialer) ServePunch(ctx context.Context, relayAddr string, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	rc, err := relay.Connect(ctx, relayAddr, d.ID, uint32(d.Listener.Port()))
	if err != nil {
		return fmt.Errorf("relay connect: %w", err)
	}
	defer rc.Close()

	logger.Info("connmgr: relay registered", "addr", relayAddr, "external", rc.ExternalAddr())

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case rv, ok := <-rc.Incoming():
			if !ok {
				return fmt.Errorf("connmgr: relay connection lost")
			}
			logger.Info("connmgr: rendezvous inbound",
				"peer", fmt.Sprintf("%x", rv.PeerNodeID[:8]),
				"peer_addr", rv.PeerExternalAddr)
			go nat.PrimeNAT(ctx, d.Listener, rv.PeerExternalAddr)
		}
	}
}
