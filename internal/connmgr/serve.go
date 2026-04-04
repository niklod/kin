package connmgr

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/niklod/kin/internal/relay"
)

// ServePunch connects to the relay, registers this node, and keeps the relay
// connection alive so that remote peers can discover this node's external address
// via RequestRendezvous and punch directly to this node's listener.
//
// When a RelayRendezvous arrives on the relay's Incoming channel, the remote peer
// (the initiator) will be dialing this node's external address using nat.Punch.
// Since the listener already accepts those incoming connections in TLS-server mode,
// no punch-back is required from this side.
//
// Blocks until ctx is cancelled or the relay connection drops.
// The caller should restart ServePunch on error if continuous relay presence is needed.
func (d *Dialer) ServePunch(ctx context.Context, relayAddr string, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	rc, err := relay.Connect(ctx, relayAddr, d.ID, d.LocalPort)
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
			// The remote peer (rv.PeerNodeID) will punch TO our external addr.
			// Our transport.Listener accepts the incoming TCP connection and performs
			// the TLS server-side handshake automatically.
			logger.Info("connmgr: rendezvous inbound",
				"peer", fmt.Sprintf("%x", rv.PeerNodeID[:8]),
				"peer_addr", rv.PeerExternalAddr)
		}
	}
}
