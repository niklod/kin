package daemon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

// RequestHandler processes IPC requests.
type RequestHandler interface {
	Handle(req Request) Response
}

// Server listens on a Unix domain socket and dispatches IPC requests to a handler.
type Server struct {
	ln      net.Listener
	handler RequestHandler
	logger  *slog.Logger

	mu          sync.Mutex
	subscribers []*subscriber
	closed      bool
}

// NewServer creates a server bound to the given socket path.
// If a stale socket file exists, it is removed before binding.
func NewServer(sockPath string, h RequestHandler, logger *slog.Logger) (*Server, error) {
	if err := removeStaleSocket(sockPath); err != nil {
		return nil, fmt.Errorf("daemon: stale socket: %w", err)
	}
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("daemon: listen: %w", err)
	}
	// Restrict socket to owning user only.
	if err := os.Chmod(sockPath, 0600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("daemon: chmod socket: %w", err)
	}
	return &Server{
		ln:      ln,
		handler: h,
		logger:  logger,
	}, nil
}

// Serve accepts connections until ctx is cancelled. Blocks until done.
func (s *Server) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		s.Close()
	}()

	for {
		conn, err := s.ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			s.logger.Warn("daemon: accept", "err", err)
			continue
		}
		go s.handleConn(conn)
	}
}

// Close shuts down the listener, removes the socket, and closes all subscribers.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	subs := s.subscribers
	s.subscribers = nil
	s.mu.Unlock()

	for _, sub := range subs {
		sub.close()
	}
	err := s.ln.Close()
	// Best-effort removal of the socket file.
	if addr, ok := s.ln.Addr().(*net.UnixAddr); ok {
		_ = os.Remove(addr.Name)
	}
	return err
}

// Broadcast sends an event to all active subscribers.
func (s *Server) Broadcast(event Event) {
	s.mu.Lock()
	subs := make([]*subscriber, len(s.subscribers))
	copy(subs, s.subscribers)
	s.mu.Unlock()

	for _, sub := range subs {
		if err := sub.send(event); err != nil {
			s.removeSub(sub)
			sub.close()
		}
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	for {
		var req Request
		if err := ReadJSON(conn, &req); err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) && !isConnClosed(err) {
				s.logger.Debug("daemon: read request", "err", err)
			}
			return
		}

		if req.Method == MethodSubscribe {
			s.handleSubscribe(conn, req.ID)
			return // subscribe takes over the connection
		}

		resp := s.handler.Handle(req)
		if err := WriteJSON(conn, resp); err != nil {
			s.logger.Debug("daemon: write response", "err", err)
			return
		}
	}
}

func (s *Server) handleSubscribe(conn net.Conn, reqID uint64) {
	resp := Response{ID: reqID}
	if err := WriteJSON(conn, resp); err != nil {
		s.logger.Debug("daemon: write subscribe ack", "err", err)
		return
	}

	sub := &subscriber{conn: conn, owned: true}
	s.mu.Lock()
	s.subscribers = append(s.subscribers, sub)
	s.mu.Unlock()

	// Block until the connection closes (client reads will return EOF).
	// The subscriber now owns the connection — handleConn's defer conn.Close()
	// will run after this returns, which is fine since sub.close() is idempotent.
	buf := make([]byte, 1)
	for {
		if _, err := conn.Read(buf); err != nil {
			break
		}
	}

	s.removeSub(sub)
}

func (s *Server) removeSub(sub *subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.subscribers {
		if existing == sub {
			s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
			return
		}
	}
}

const subscriberWriteTimeout = 5 * time.Second

// subscriber wraps a connection used for event streaming.
type subscriber struct {
	mu     sync.Mutex
	conn   net.Conn
	owned  bool // true if this subscriber owns the connection lifecycle
	closed bool
}

func (sub *subscriber) send(event Event) error {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.closed {
		return net.ErrClosed
	}
	// Set a write deadline so a slow/stalled client cannot block Broadcast.
	if err := sub.conn.SetWriteDeadline(time.Now().Add(subscriberWriteTimeout)); err != nil {
		return err
	}
	return WriteJSON(sub.conn, event)
}

func (sub *subscriber) close() {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.closed {
		return
	}
	sub.closed = true
	sub.conn.Close()
}

// removeStaleSocket removes a leftover socket file from a previous crash.
// If the socket is live (another daemon is listening), it returns an error.
func removeStaleSocket(path string) error {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	// Try connecting to see if a daemon is already running.
	conn, err := net.Dial("unix", path)
	if err == nil {
		conn.Close()
		return fmt.Errorf("another daemon is already running (socket %s is live)", path)
	}

	// Connection refused — stale socket, safe to remove.
	return os.Remove(path)
}

// isConnClosed returns true for errors that indicate a closed connection.
func isConnClosed(err error) bool {
	return errors.Is(err, net.ErrClosed)
}

// marshalData marshals v into json.RawMessage for use in Response.Data.
func marshalData(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// errResponse creates an error Response.
func errResponse(id uint64, code, message string) Response {
	return Response{
		ID:    id,
		Error: &ErrorInfo{Code: code, Message: message},
	}
}

// okResponse creates a success Response with data.
func okResponse(id uint64, v any) Response {
	return Response{
		ID:   id,
		Data: marshalData(v),
	}
}

// hexNodeID returns the full lowercase hex encoding of a 32-byte node ID.
func hexNodeID(id [32]byte) string {
	return hex.EncodeToString(id[:])
}

// BroadcastPeerOnline sends a peer-online event to all subscribers.
func (s *Server) BroadcastPeerOnline(peerID [32]byte) {
	nodeID := hexNodeID(peerID)
	s.Broadcast(Event{
		Type: EventPeerOnline,
		Data: marshalData(PeerInfo{
			NodeID:      nodeID,
			NodeIDShort: shortID(nodeID),
			Online:      true,
		}),
	})
}

// BroadcastPeerOffline sends a peer-offline event to all subscribers.
func (s *Server) BroadcastPeerOffline(peerID [32]byte) {
	s.Broadcast(Event{
		Type: EventPeerOffline,
		Data: marshalData(PeerOfflineEvent{NodeID: hexNodeID(peerID)}),
	})
}
