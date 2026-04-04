//go:build darwin || linux

package transport

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// listenConfig returns a ListenConfig that sets SO_REUSEPORT on the TCP socket.
// SO_REUSEPORT allows the NAT punch goroutine to bind the same local port for
// an outgoing connect() while the listener is already bound to it.
func listenConfig() net.ListenConfig {
	return net.ListenConfig{
		Control: func(_, _ string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				//nolint:errcheck // best-effort; bind still works without it
				unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})
		},
	}
}
