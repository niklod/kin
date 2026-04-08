package tui

import (
	"fmt"

	"github.com/niklod/kin/internal/daemon"
)

// StatusBarModel holds the data displayed in the bottom status bar.
type StatusBarModel struct {
	nodeID      string
	peerCount   int
	relayAddr   string
	relayOnline bool
	width       int
}

// SetStatus updates the status bar from a daemon status response.
func (s *StatusBarModel) SetStatus(resp *daemon.StatusResponse) {
	s.nodeID = resp.NodeIDShort
	s.peerCount = resp.PeerCount
	s.relayAddr = resp.RelayAddr
	s.relayOnline = resp.RelayOnline
}

// SetWidth updates the available width for rendering.
func (s *StatusBarModel) SetWidth(w int) {
	s.width = w
}

// SetPeerCount updates the peer count.
func (s *StatusBarModel) SetPeerCount(n int) {
	s.peerCount = n
}

// View renders the status bar.
func (s StatusBarModel) View() string {
	relay := statusKeyStyle.Render("relay:") + "disconnected"
	if s.relayOnline && s.relayAddr != "" {
		relay = statusKeyStyle.Render("relay:") + s.relayAddr
	}

	peers := statusKeyStyle.Render("peers:") + fmt.Sprintf("%d", s.peerCount)

	nodeID := statusKeyStyle.Render("node:") + s.nodeID

	content := fmt.Sprintf(" %s  %s  %s", relay, peers, nodeID)

	return statusBarStyle.Width(s.width).Render(content)
}
