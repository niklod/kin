//go:build darwin || linux

package nat

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// dialControl sets SO_REUSEADDR and SO_REUSEPORT on the outgoing TCP socket.
// SO_REUSEPORT lets us bind the same local port that the kin listener already
// holds, which is required for NAT hole punching (the NAT must see outbound
// traffic from our listen port so it creates the right mapping).
func dialControl(_, _ string, c syscall.RawConn) error {
	return c.Control(func(fd uintptr) {
		//nolint:errcheck // best-effort; the dial still proceeds on failure
		unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
		//nolint:errcheck
		unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
}
