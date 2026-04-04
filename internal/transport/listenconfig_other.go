//go:build !darwin && !linux

package transport

import "net"

// listenConfig returns a default ListenConfig (no SO_REUSEPORT).
// NAT hole punching that requires dialing from the listen port is
// not supported on this platform.
func listenConfig() net.ListenConfig {
	return net.ListenConfig{}
}
