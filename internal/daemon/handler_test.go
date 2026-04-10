package daemon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsRoutableListenAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		routable bool
	}{
		{name: "ipv6_wildcard", addr: "[::]:7777", routable: false},
		{name: "ipv4_wildcard", addr: "0.0.0.0:7777", routable: false},
		{name: "empty_host", addr: ":7777", routable: false},
		{name: "routable_ipv4", addr: "192.168.1.5:7777", routable: true},
		{name: "loopback", addr: "127.0.0.1:7777", routable: true},
		{name: "routable_ipv6", addr: "[::1]:7777", routable: true},
		{name: "invalid", addr: "not-an-addr", routable: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.routable, isRoutableListenAddr(tt.addr))
		})
	}
}
