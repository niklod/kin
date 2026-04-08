package ipc

import (
	"github.com/niklod/kin/internal/config"
)

// TryDaemon attempts to connect to a running daemon's socket in cfgDir.
// Returns a connected Client on success, or an error if no daemon is running.
func TryDaemon(cfgDir string) (*Client, error) {
	return Dial(config.SocketPath(cfgDir))
}
