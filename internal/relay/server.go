// Package relay implements the Kin relay server and client.
//
// The relay is a signaling-only server: it records each connected node's
// external address and coordinates TCP simultaneous-open (hole punching) by
// pushing RelayRendezvous messages to both peers at once. No application
// traffic is forwarded through the relay.
package relay

import (
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/niklod/kin/internal/protocol"
	"github.com/niklod/kin/kinpb"
)

// entry tracks a connected node in the relay registry.
type entry struct {
	nodeID       [32]byte
	publicKey    []byte
	externalAddr string // ip:listen_port
	conn         net.Conn
	mu           sync.Mutex // guards writes to conn
}

func (e *entry) send(env *kinpb.Envelope) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return protocol.WriteMsg(e.conn, env)
}

// Server is the relay signaling server. It is safe for concurrent use.
type Server struct {
	mu       sync.RWMutex
	registry map[[32]byte]*entry // NodeID → entry
	logger   *slog.Logger
}

// NewServer creates a relay Server.
func NewServer(logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		registry: make(map[[32]byte]*entry),
		logger:   logger,
	}
}

// Serve handles a single incoming connection until it closes.
// extIP is the remote IP of the connection as seen by the listener.
func (s *Server) Serve(conn net.Conn, extIP string) {
	defer conn.Close()

	// First message must be RelayRegister.
	env, err := protocol.ReadMsg(conn)
	if err != nil {
		s.logger.Debug("relay: read register", "err", err)
		return
	}
	reg := env.GetRelayRegister()
	if reg == nil {
		s.logger.Debug("relay: expected RelayRegister", "got", fmt.Sprintf("%T", env.Payload))
		return
	}
	if len(reg.NodeId) != 32 || len(reg.PublicKey) != 32 {
		s.logger.Debug("relay: invalid register payload")
		return
	}

	var nodeID [32]byte
	copy(nodeID[:], reg.NodeId)

	// Build external address: relay's view of peer's IP + peer's self-reported listen port.
	extAddr := fmt.Sprintf("%s:%d", extIP, reg.ListenPort)

	e := &entry{
		nodeID:       nodeID,
		publicKey:    reg.PublicKey,
		externalAddr: extAddr,
		conn:         conn,
	}

	s.mu.Lock()
	s.registry[nodeID] = e
	s.mu.Unlock()

	s.logger.Info("relay: node registered",
		"node", fmt.Sprintf("%x", nodeID[:8]),
		"external", extAddr)

	// Confirm registration.
	if err := e.send(&kinpb.Envelope{Payload: &kinpb.Envelope_RelayRegistered{
		RelayRegistered: &kinpb.RelayRegistered{ExternalAddr: extAddr},
	}}); err != nil {
		s.deregister(nodeID)
		return
	}

	// Serve relay messages from this node.
	for {
		env, err := protocol.ReadMsg(conn)
		if err != nil {
			break
		}
		switch p := env.Payload.(type) {
		case *kinpb.Envelope_RelayConnect:
			s.handleConnect(e, p.RelayConnect)
		default:
			s.logger.Debug("relay: unexpected message", "type", fmt.Sprintf("%T", env.Payload))
		}
	}

	s.deregister(nodeID)
	s.logger.Info("relay: node disconnected", "node", fmt.Sprintf("%x", nodeID[:8]))
}

func (s *Server) handleConnect(requester *entry, req *kinpb.RelayConnect) {
	if len(req.TargetNodeId) != 32 {
		requester.send(relayError("invalid_request")) //nolint:errcheck
		return
	}
	var targetID [32]byte
	copy(targetID[:], req.TargetNodeId)

	s.mu.RLock()
	target, ok := s.registry[targetID]
	s.mu.RUnlock()

	if !ok {
		requester.send(relayError("peer_not_registered")) //nolint:errcheck
		s.logger.Debug("relay: connect target not found",
			"requester", fmt.Sprintf("%x", requester.nodeID[:8]),
			"target", fmt.Sprintf("%x", targetID[:8]))
		return
	}

	s.logger.Info("relay: rendezvous",
		"a", fmt.Sprintf("%x", target.nodeID[:8]),
		"b", fmt.Sprintf("%x", requester.nodeID[:8]))

	// Push to both sides simultaneously.
	rendezvousToB := &kinpb.Envelope{Payload: &kinpb.Envelope_RelayRendezvous{
		RelayRendezvous: &kinpb.RelayRendezvous{
			PeerNodeId:      target.nodeID[:],
			PeerPublicKey:   target.publicKey,
			PeerExternalAddr: target.externalAddr,
		},
	}}
	rendezvousToA := &kinpb.Envelope{Payload: &kinpb.Envelope_RelayRendezvous{
		RelayRendezvous: &kinpb.RelayRendezvous{
			PeerNodeId:      requester.nodeID[:],
			PeerPublicKey:   requester.publicKey,
			PeerExternalAddr: requester.externalAddr,
		},
	}}

	// Send to target (A) first so it starts listening before B dials.
	if err := target.send(rendezvousToA); err != nil {
		s.logger.Warn("relay: send rendezvous to target", "err", err)
		requester.send(relayError("peer_unreachable")) //nolint:errcheck
		return
	}
	if err := requester.send(rendezvousToB); err != nil {
		s.logger.Warn("relay: send rendezvous to requester", "err", err)
	}
}

func (s *Server) deregister(nodeID [32]byte) {
	s.mu.Lock()
	delete(s.registry, nodeID)
	s.mu.Unlock()
}

func relayError(reason string) *kinpb.Envelope {
	return &kinpb.Envelope{Payload: &kinpb.Envelope_RelayError{
		RelayError: &kinpb.RelayError{Reason: reason},
	}}
}
