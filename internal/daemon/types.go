// Package daemon implements the kin daemon process with a Unix socket control interface.
package daemon

import "encoding/json"

// Request is sent from a CLI/TUI client to the daemon.
type Request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is sent from the daemon back to the client.
type Response struct {
	ID    uint64          `json:"id"`
	Error *ErrorInfo      `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// ErrorInfo describes an error returned by the daemon.
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Event is pushed to subscribers for real-time updates.
type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Event type constants.
const (
	EventPeerOnline     = "peer_online"
	EventPeerOffline    = "peer_offline"
	EventCatalogUpdated = "catalog_updated"
	EventRelayStatus    = "relay_status"
)

// Method constants.
const (
	MethodStatus    = "status"
	MethodPeers     = "peers"
	MethodInvite    = "invite"
	MethodJoin      = "join"
	MethodCatalog   = "catalog"
	MethodDownload  = "download"
	MethodSubscribe = "subscribe"
)

// StatusResponse is returned by the "status" method.
type StatusResponse struct {
	NodeID      string `json:"node_id"`
	NodeIDShort string `json:"node_id_short"`
	ListenAddr  string `json:"listen_addr"`
	SharedDir   string `json:"shared_dir"`
	PeerCount   int    `json:"peer_count"`
	RelayAddr   string `json:"relay_addr,omitempty"`
	RelayOnline bool   `json:"relay_online"`
}

// PeerInfo describes a single peer for IPC responses and events.
type PeerInfo struct {
	NodeID      string   `json:"node_id"`
	NodeIDShort string   `json:"node_id_short"`
	TrustState  string   `json:"trust_state"`
	LastSeen    string   `json:"last_seen"`
	Endpoints   []string `json:"endpoints"`
	Online      bool     `json:"online"`
}

// PeersResponse is returned by the "peers" method.
type PeersResponse struct {
	Peers []PeerInfo `json:"peers"`
}

// InviteResponse is returned by the "invite" method.
type InviteResponse struct {
	Token string `json:"token"`
}

// JoinParams are accepted by the "join" method.
type JoinParams struct {
	Token string `json:"token"`
}

// JoinResponse is returned by the "join" method.
type JoinResponse struct {
	PeerNodeID string `json:"peer_node_id"`
}

// FileInfo describes a single catalog entry for IPC responses.
type FileInfo struct {
	FileID      string `json:"file_id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ModTime     string `json:"mod_time"`
	OwnerNodeID string `json:"owner_node_id"`
	IsLocal     bool   `json:"is_local"`
}

// CatalogResponse is returned by the "catalog" method.
type CatalogResponse struct {
	Files []FileInfo `json:"files"`
}

// DownloadParams are accepted by the "download" method.
type DownloadParams struct {
	FileID string `json:"file_id"`
}

// DownloadResponse is returned by the "download" method.
type DownloadResponse struct {
	Path string `json:"path"`
}

// RelayStatusEvent is the data payload for EventRelayStatus.
type RelayStatusEvent struct {
	Online bool   `json:"online"`
	Addr   string `json:"addr"`
}

// PeerOfflineEvent is the data payload for EventPeerOffline.
type PeerOfflineEvent struct {
	NodeID string `json:"node_id"`
}
