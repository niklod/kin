package relay

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/niklod/kin/internal/identity"
	"github.com/niklod/kin/internal/protocol"
	"github.com/niklod/kin/kinpb"
)

// RendezvousInfo carries the peer's identity and external address for hole punching.
type RendezvousInfo struct {
	PeerNodeID       [32]byte
	PeerPublicKey    []byte
	PeerExternalAddr string // "ip:port"
}

// ErrPeerNotRegistered is returned when the target node is not registered with the relay.
var ErrPeerNotRegistered = errors.New("relay: peer not registered")

// ErrRelayDisconnected is returned when the relay connection drops before a response arrives.
var ErrRelayDisconnected = errors.New("relay: connection lost")

// Client connects to a relay server, registers a node, and coordinates rendezvous.
type Client struct {
	id           *identity.Identity
	listenPort   uint32
	conn         net.Conn
	externalAddr string

	mu       sync.Mutex
	pending  map[[32]byte]chan *kinpb.RelayRendezvous // targetNodeID → waiter
	incoming chan *RendezvousInfo                     // unmatched incoming rendezvous
	closed   bool
}

// Connect dials the relay at addr (TLS, InsecureSkipVerify), sends RelayRegister,
// and waits for RelayRegistered. Returns a ready Client.
func Connect(ctx context.Context, addr string, id *identity.Identity, listenPort uint32) (*Client, error) {
	slog.Debug("relay: dialing", "addr", addr, "listen_port", listenPort,
		"node_id", fmt.Sprintf("%x", id.NodeID[:8]))

	dialer := &tls.Dialer{
		Config: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // relay cert not pinned in MVP
			MinVersion:         tls.VersionTLS13,
		},
	}
	rawConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		slog.Debug("relay: TCP dial failed", "addr", addr, "err", err)
		return nil, fmt.Errorf("relay connect: %w", err)
	}
	slog.Debug("relay: TCP connected", "addr", addr)

	// Send registration.
	if err := protocol.WriteMsg(rawConn, &kinpb.Envelope{Payload: &kinpb.Envelope_RelayRegister{
		RelayRegister: &kinpb.RelayRegister{
			NodeId:     id.NodeID[:],
			PublicKey:  id.PubKey,
			ListenPort: listenPort,
		},
	}}); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("relay register: %w", err)
	}

	// Wait for confirmation.
	env, err := protocol.ReadMsg(rawConn)
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("relay register response: %w", err)
	}
	reg := env.GetRelayRegistered()
	if reg == nil {
		rawConn.Close()
		return nil, fmt.Errorf("relay: expected RelayRegistered, got %T", env.Payload)
	}

	slog.Debug("relay: registered", "external_addr", reg.ExternalAddr)

	c := &Client{
		id:           id,
		listenPort:   listenPort,
		conn:         rawConn,
		externalAddr: reg.ExternalAddr,
		pending:      make(map[[32]byte]chan *kinpb.RelayRendezvous),
		incoming:     make(chan *RendezvousInfo, 16),
	}
	go c.readLoop()
	return c, nil
}

// Incoming returns a channel that receives RelayRendezvous messages from peers
// that were not awaited by RequestRendezvous or WaitForRendezvous.
// The channel is closed when the relay connection is lost or Close is called.
func (c *Client) Incoming() <-chan *RendezvousInfo { return c.incoming }

// ExternalAddr returns the external "ip:port" as seen by the relay.
func (c *Client) ExternalAddr() string { return c.externalAddr }

// RequestRendezvous asks the relay to connect this node with targetNodeID.
// Blocks until RelayRendezvous or an error arrives (or ctx is cancelled).
func (c *Client) RequestRendezvous(ctx context.Context, targetNodeID [32]byte) (*RendezvousInfo, error) {
	slog.Debug("relay: requesting rendezvous", "target", fmt.Sprintf("%x", targetNodeID[:8]))

	ch := make(chan *kinpb.RelayRendezvous, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrRelayDisconnected
	}
	c.pending[targetNodeID] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, targetNodeID)
		c.mu.Unlock()
	}()

	if err := protocol.WriteMsg(c.conn, &kinpb.Envelope{Payload: &kinpb.Envelope_RelayConnect{
		RelayConnect: &kinpb.RelayConnect{TargetNodeId: targetNodeID[:]},
	}}); err != nil {
		return nil, fmt.Errorf("relay connect request: %w", err)
	}

	select {
	case <-ctx.Done():
		slog.Debug("relay: rendezvous wait cancelled", "err", ctx.Err())
		return nil, ctx.Err()
	case rv, ok := <-ch:
		if !ok {
			slog.Debug("relay: rendezvous channel closed (relay disconnected)")
			return nil, ErrRelayDisconnected
		}
		if len(rv.PeerNodeId) != 32 {
			return nil, fmt.Errorf("relay: invalid rendezvous peer_node_id length")
		}
		slog.Debug("relay: rendezvous received",
			"peer", fmt.Sprintf("%x", rv.PeerNodeId[:8]),
			"peer_external_addr", rv.PeerExternalAddr)
		return rendezvousFromProto(rv), nil
	}
}

// WaitForRendezvous registers a listener for an unsolicited RelayRendezvous
// (i.e., when a remote peer initiates via RequestRendezvous targeting this node).
// The returned channel receives at most one value; call before the remote peer
// sends its RelayConnect for correct timing.
func (c *Client) WaitForRendezvous(ctx context.Context, fromNodeID [32]byte) <-chan *RendezvousInfo {
	ch := make(chan *RendezvousInfo, 1)
	rawCh := make(chan *kinpb.RelayRendezvous, 1)

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		close(ch)
		return ch
	}
	c.pending[fromNodeID] = rawCh
	c.mu.Unlock()

	go func() {
		defer func() {
			c.mu.Lock()
			delete(c.pending, fromNodeID)
			c.mu.Unlock()
		}()
		select {
		case <-ctx.Done():
			close(ch)
		case rv, ok := <-rawCh:
			if !ok || rv == nil {
				close(ch)
				return
			}
			ch <- rendezvousFromProto(rv)
		}
	}()
	return ch
}

// Close tears down the relay connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closeLocked() {
		return c.conn.Close()
	}
	return nil
}

// readLoop dispatches incoming relay messages to pending waiters.
func (c *Client) readLoop() {
	for {
		env, err := protocol.ReadMsg(c.conn)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				// Connection error — close all waiters.
			}
			c.closeAllPending()
			return
		}
		switch p := env.Payload.(type) {
		case *kinpb.Envelope_RelayRendezvous:
			c.dispatchRendezvous(p.RelayRendezvous)
		case *kinpb.Envelope_RelayError:
			c.dispatchError(p.RelayError)
		}
	}
}

func (c *Client) dispatchRendezvous(rv *kinpb.RelayRendezvous) {
	if len(rv.PeerNodeId) != 32 {
		return
	}
	var peerID [32]byte
	copy(peerID[:], rv.PeerNodeId)

	slog.Debug("relay: dispatch rendezvous",
		"peer", fmt.Sprintf("%x", peerID[:8]),
		"peer_external_addr", rv.PeerExternalAddr)

	c.mu.Lock()
	defer c.mu.Unlock()

	if ch, ok := c.pending[peerID]; ok {
		slog.Debug("relay: routed to waiter", "peer", fmt.Sprintf("%x", peerID[:8]))
		select {
		case ch <- rv:
		default:
		}
		return
	}

	// No specific waiter — deliver to the general incoming channel.
	if c.closed {
		return
	}
	select {
	case c.incoming <- rendezvousFromProto(rv):
	default: // buffer full; peer will time out
	}
}

func (c *Client) dispatchError(re *kinpb.RelayError) {
	// The relay doesn't echo back the target NodeID, so we can't route the error
	// to a specific waiter. Close all pending waiters so callers unblock with
	// ErrRelayDisconnected and can retry.
	slog.Debug("relay: relay error received", "reason", re.Reason)
	c.closeAllPending()
}

func (c *Client) closeAllPending() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
}

// closeLocked tears down all pending channels and closes incoming.
// Returns true if this call performed the teardown (false if already closed).
// Caller must hold c.mu.
func (c *Client) closeLocked() bool {
	if c.closed {
		return false
	}
	c.closed = true
	for k, ch := range c.pending {
		close(ch)
		delete(c.pending, k)
	}
	close(c.incoming)
	return true
}

// rendezvousFromProto converts a protobuf RelayRendezvous to a RendezvousInfo.
// Caller must have already verified len(rv.PeerNodeId) == 32.
func rendezvousFromProto(rv *kinpb.RelayRendezvous) *RendezvousInfo {
	var peerID [32]byte
	copy(peerID[:], rv.PeerNodeId)
	return &RendezvousInfo{
		PeerNodeID:       peerID,
		PeerPublicKey:    rv.PeerPublicKey,
		PeerExternalAddr: rv.PeerExternalAddr,
	}
}
