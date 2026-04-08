// Package ipc provides a client for communicating with the kin daemon via
// its Unix domain socket.
package ipc

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/niklod/kin/internal/daemon"
)

// Client communicates with a running kin daemon over a Unix socket.
type Client struct {
	conn  net.Conn
	mu    sync.Mutex
	nextID atomic.Uint64
}

// Dial connects to the daemon socket at the given path.
func Dial(sockPath string) (*Client, error) {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("ipc dial: %w", err)
	}
	return &Client{conn: conn}, nil
}

// Close closes the connection to the daemon.
func (c *Client) Close() error {
	return c.conn.Close()
}

// call sends a request and reads the response. It is safe for concurrent use.
func (c *Client) call(method string, params any) (*daemon.Response, error) {
	id := c.nextID.Add(1)

	var rawParams json.RawMessage
	if params != nil {
		var err error
		rawParams, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("ipc: marshal params: %w", err)
		}
	}

	req := daemon.Request{
		ID:     id,
		Method: method,
		Params: rawParams,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := daemon.WriteJSON(c.conn, req); err != nil {
		return nil, err
	}

	var resp daemon.Response
	if err := daemon.ReadJSON(c.conn, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// callAndDecode sends an RPC and decodes the response into T.
func callAndDecode[T any](c *Client, method string, params any) (*T, error) {
	resp, err := c.call(method, params)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("daemon: %s: %s", resp.Error.Code, resp.Error.Message)
	}
	var result T
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("ipc: unmarshal %s: %w", method, err)
	}
	return &result, nil
}

// Status returns the daemon's current status.
func (c *Client) Status() (*daemon.StatusResponse, error) {
	return callAndDecode[daemon.StatusResponse](c, daemon.MethodStatus, nil)
}

// Peers returns the list of known peers.
func (c *Client) Peers() (*daemon.PeersResponse, error) {
	return callAndDecode[daemon.PeersResponse](c, daemon.MethodPeers, nil)
}

// Invite generates a new invite token.
func (c *Client) Invite() (*daemon.InviteResponse, error) {
	return callAndDecode[daemon.InviteResponse](c, daemon.MethodInvite, nil)
}

// Join sends a join request with the given invite token.
func (c *Client) Join(token string) (*daemon.JoinResponse, error) {
	return callAndDecode[daemon.JoinResponse](c, daemon.MethodJoin, daemon.JoinParams{Token: token})
}

// Catalog returns all catalog entries.
func (c *Client) Catalog() (*daemon.CatalogResponse, error) {
	return callAndDecode[daemon.CatalogResponse](c, daemon.MethodCatalog, nil)
}

// Download requests a file download by file ID.
func (c *Client) Download(fileID string) (*daemon.DownloadResponse, error) {
	return callAndDecode[daemon.DownloadResponse](c, daemon.MethodDownload, daemon.DownloadParams{FileID: fileID})
}

// Subscribe sends a subscribe request and returns a channel that receives
// events. The channel is closed when the connection is lost.
// Subscribe takes over the connection — do not call other methods after this.
func (c *Client) Subscribe() (<-chan daemon.Event, error) {
	id := c.nextID.Add(1)
	req := daemon.Request{
		ID:     id,
		Method: daemon.MethodSubscribe,
	}

	if err := daemon.WriteJSON(c.conn, req); err != nil {
		return nil, err
	}

	var resp daemon.Response
	if err := daemon.ReadJSON(c.conn, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("daemon: %s: %s", resp.Error.Code, resp.Error.Message)
	}

	ch := make(chan daemon.Event, 64)
	go func() {
		defer close(ch)
		for {
			var event daemon.Event
			if err := daemon.ReadJSON(c.conn, &event); err != nil {
				return
			}
			// Non-blocking send: drop events if consumer is too slow.
			select {
			case ch <- event:
			default:
			}
		}
	}()
	return ch, nil
}
