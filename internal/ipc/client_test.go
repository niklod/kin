package ipc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/require"

	"github.com/niklod/kin/internal/daemon"
)

func testSockPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "kin-ipc-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "t.sock")
}

func startTestServer(t *testing.T, sockPath string, fn func(daemon.Request) daemon.Response) *daemon.Server {
	t.Helper()
	handler := &testHandler{fn: fn}
	srv, err := daemon.NewServer(sockPath, handler, slog.Default())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { srv.Close() })
	go srv.Serve(ctx)
	time.Sleep(50 * time.Millisecond)
	return srv
}

func TestClient_Status(t *testing.T) {
	sockPath := testSockPath(t)
	startTestServer(t, sockPath, func(req daemon.Request) daemon.Response {
		return daemon.Response{
			ID:   req.ID,
			Data: mustMarshal(daemon.StatusResponse{NodeID: "deadbeef", PeerCount: 3}),
		}
	})

	client, err := Dial(sockPath)
	require.NoError(t, err)
	defer client.Close()

	status, err := client.Status()
	require.NoError(t, err)
	require.Equal(t, "deadbeef", status.NodeID)
	require.Equal(t, 3, status.PeerCount)
}

func TestClient_Peers(t *testing.T) {
	sockPath := testSockPath(t)
	startTestServer(t, sockPath, func(req daemon.Request) daemon.Response {
		return daemon.Response{
			ID: req.ID,
			Data: mustMarshal(daemon.PeersResponse{
				Peers: []daemon.PeerInfo{
					{NodeID: "aaa", TrustState: "tofu"},
					{NodeID: "bbb", TrustState: "tofu"},
				},
			}),
		}
	})

	client, err := Dial(sockPath)
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Peers()
	require.NoError(t, err)
	require.Len(t, resp.Peers, 2)
	require.Equal(t, "aaa", resp.Peers[0].NodeID)
}

func TestClient_Invite(t *testing.T) {
	sockPath := testSockPath(t)
	startTestServer(t, sockPath, func(req daemon.Request) daemon.Response {
		return daemon.Response{
			ID:   req.ID,
			Data: mustMarshal(daemon.InviteResponse{Token: "kin:test123"}),
		}
	})

	client, err := Dial(sockPath)
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Invite()
	require.NoError(t, err)
	require.Equal(t, "kin:test123", resp.Token)
}

func TestClient_Join(t *testing.T) {
	sockPath := testSockPath(t)
	startTestServer(t, sockPath, func(req daemon.Request) daemon.Response {
		return daemon.Response{
			ID:   req.ID,
			Data: mustMarshal(daemon.JoinResponse{PeerNodeID: "peer123"}),
		}
	})

	client, err := Dial(sockPath)
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Join("kin:sometoken")
	require.NoError(t, err)
	require.Equal(t, "peer123", resp.PeerNodeID)
}

func TestClient_Catalog(t *testing.T) {
	sockPath := testSockPath(t)
	startTestServer(t, sockPath, func(req daemon.Request) daemon.Response {
		return daemon.Response{
			ID: req.ID,
			Data: mustMarshal(daemon.CatalogResponse{
				Files: []daemon.FileInfo{
					{Name: "file1.txt", Size: 1024, IsLocal: true},
				},
			}),
		}
	})

	client, err := Dial(sockPath)
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Catalog()
	require.NoError(t, err)
	require.Len(t, resp.Files, 1)
	require.Equal(t, "file1.txt", resp.Files[0].Name)
	require.True(t, resp.Files[0].IsLocal)
}

func TestClient_Subscribe(t *testing.T) {
	sockPath := testSockPath(t)
	srv := startTestServer(t, sockPath, func(req daemon.Request) daemon.Response {
		return daemon.Response{ID: req.ID}
	})

	client, err := Dial(sockPath)
	require.NoError(t, err)
	defer client.Close()

	events, err := client.Subscribe()
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	srv.Broadcast(daemon.Event{Type: daemon.EventCatalogUpdated})

	select {
	case ev := <-events:
		require.Equal(t, daemon.EventCatalogUpdated, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestClient_ErrorResponse(t *testing.T) {
	sockPath := testSockPath(t)
	startTestServer(t, sockPath, func(req daemon.Request) daemon.Response {
		return daemon.Response{
			ID:    req.ID,
			Error: &daemon.ErrorInfo{Code: "test_err", Message: "something failed"},
		}
	})

	client, err := Dial(sockPath)
	require.NoError(t, err)
	defer client.Close()

	_, err = client.Status()
	require.Error(t, err)
	require.Contains(t, err.Error(), "something failed")
}

func TestClient_Dial_NoDaemon(t *testing.T) {
	_, err := Dial("/tmp/nonexistent-kin-test.sock")
	require.Error(t, err)
}

func TestTryDaemon_NoDaemon(t *testing.T) {
	dir := t.TempDir()
	_, err := TryDaemon(dir)
	require.Error(t, err)
}

type testHandler struct {
	fn func(daemon.Request) daemon.Response
}

func (h *testHandler) Handle(req daemon.Request) daemon.Response {
	return h.fn(req)
}

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
