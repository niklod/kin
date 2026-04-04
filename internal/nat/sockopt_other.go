//go:build !darwin && !linux

package nat

import "syscall"

// dialControl is a no-op on platforms without SO_REUSEPORT.
// NAT hole punching still attempts a direct dial but cannot guarantee
// the source port matches the kin listen port.
func dialControl(_, _ string, c syscall.RawConn) error {
	return nil
}
