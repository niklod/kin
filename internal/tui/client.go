package tui

import "github.com/niklod/kin/internal/daemon"

// DaemonClient provides RPC access to the running kin daemon.
// Defined on the consumer side per project convention.
type DaemonClient interface {
	Status() (*daemon.StatusResponse, error)
	Peers() (*daemon.PeersResponse, error)
	Invite() (*daemon.InviteResponse, error)
	Join(token string) (*daemon.JoinResponse, error)
	Catalog() (*daemon.CatalogResponse, error)
	Download(fileID string) (*daemon.DownloadResponse, error)
	Close() error
}
