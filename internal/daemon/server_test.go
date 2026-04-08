package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"log/slog"
)

// testSockPath returns a short socket path under /tmp to avoid macOS 104-byte limit.
func testSockPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "kin-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "t.sock")
}

func TestServer_StatusMethod(t *testing.T) {
	sockPath := testSockPath(t)

	handler := &stubHandler{
		fn: func(req Request) Response {
			return okResponse(req.ID, StatusResponse{
				NodeID:    "abc123",
				PeerCount: 2,
			})
		},
	}

	srv, err := NewServer(sockPath, handler, slog.Default())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err)
	defer conn.Close()

	req := Request{ID: 1, Method: MethodStatus}
	require.NoError(t, WriteJSON(conn, req))

	var resp Response
	require.NoError(t, ReadJSON(conn, &resp))
	require.Equal(t, uint64(1), resp.ID)
	require.Nil(t, resp.Error)
	require.NotNil(t, resp.Data)

	cancel()
}

func TestServer_Subscribe_ReceivesEvents(t *testing.T) {
	sockPath := testSockPath(t)

	srv, err := NewServer(sockPath, &stubHandler{fn: func(req Request) Response {
		return okResponse(req.ID, nil)
	}}, slog.Default())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err)
	defer conn.Close()

	require.NoError(t, WriteJSON(conn, Request{ID: 1, Method: MethodSubscribe}))

	var ack Response
	require.NoError(t, ReadJSON(conn, &ack))
	require.Equal(t, uint64(1), ack.ID)
	require.Nil(t, ack.Error)

	time.Sleep(50 * time.Millisecond)

	srv.Broadcast(Event{Type: EventPeerOnline, Data: marshalData(PeerInfo{NodeID: "abc"})})

	var event Event
	require.NoError(t, ReadJSON(conn, &event))
	require.Equal(t, EventPeerOnline, event.Type)

	cancel()
}

func TestServer_StaleSocket_Overwritten(t *testing.T) {
	sockPath := testSockPath(t)

	require.NoError(t, os.WriteFile(sockPath, []byte("stale"), 0600))

	srv, err := NewServer(sockPath, &stubHandler{fn: func(req Request) Response {
		return okResponse(req.ID, nil)
	}}, slog.Default())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err)
	conn.Close()

	cancel()
	srv.Close()
}

func TestServer_LiveSocket_RejectsSecondDaemon(t *testing.T) {
	sockPath := testSockPath(t)

	srv1, err := NewServer(sockPath, &stubHandler{fn: func(req Request) Response {
		return okResponse(req.ID, nil)
	}}, slog.Default())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv1.Serve(ctx)
	time.Sleep(50 * time.Millisecond)

	_, err = NewServer(sockPath, nil, slog.Default())
	require.Error(t, err)
	require.Contains(t, err.Error(), "already running")

	cancel()
	srv1.Close()
}

func TestServer_Close_RemovesSocket(t *testing.T) {
	sockPath := testSockPath(t)

	srv, err := NewServer(sockPath, &stubHandler{fn: func(req Request) Response {
		return okResponse(req.ID, nil)
	}}, slog.Default())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go srv.Serve(ctx)
	time.Sleep(50 * time.Millisecond)

	cancel()
	srv.Close()
	time.Sleep(50 * time.Millisecond)

	_, err = os.Stat(sockPath)
	require.True(t, os.IsNotExist(err), "socket file should be removed after Close")
}

func TestServer_MultipleRequests_SameConnection(t *testing.T) {
	sockPath := testSockPath(t)

	callCount := 0
	handler := &stubHandler{
		fn: func(req Request) Response {
			callCount++
			return okResponse(req.ID, StatusResponse{PeerCount: callCount})
		},
	}

	srv, err := NewServer(sockPath, handler, slog.Default())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err)
	defer conn.Close()

	for i := uint64(1); i <= 3; i++ {
		require.NoError(t, WriteJSON(conn, Request{ID: i, Method: MethodStatus}))
		var resp Response
		require.NoError(t, ReadJSON(conn, &resp))
		require.Equal(t, i, resp.ID)
	}
	require.Equal(t, 3, callCount)

	cancel()
}

func TestServer_UnknownMethod(t *testing.T) {
	sockPath := testSockPath(t)

	handler := &stubHandler{
		fn: func(req Request) Response {
			return errResponse(req.ID, "unknown_method", fmt.Sprintf("unknown method: %s", req.Method))
		},
	}

	srv, err := NewServer(sockPath, handler, slog.Default())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err)
	defer conn.Close()

	require.NoError(t, WriteJSON(conn, Request{ID: 1, Method: "nonexistent"}))

	var resp Response
	require.NoError(t, ReadJSON(conn, &resp))
	require.NotNil(t, resp.Error)
	require.Equal(t, "unknown_method", resp.Error.Code)

	cancel()
}

// stubHandler wraps a function to satisfy RequestHandler.
type stubHandler struct {
	fn func(Request) Response
}

func (s *stubHandler) Handle(req Request) Response {
	return s.fn(req)
}
