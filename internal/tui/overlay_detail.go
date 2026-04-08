package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/niklod/kin/internal/daemon"
)

// DetailOverlay shows detailed info about a peer.
type DetailOverlay struct {
	peer daemon.PeerInfo
}

// NewDetailOverlay creates a detail overlay for the given peer.
func NewDetailOverlay(peer daemon.PeerInfo) *DetailOverlay {
	return &DetailOverlay{peer: peer}
}

// Update handles input. Esc or Enter closes.
func (o *DetailOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if key.Matches(keyMsg, keys.Escape) || key.Matches(keyMsg, keys.Enter) {
			return nil, nil
		}
	}
	return o, nil
}

// View renders peer details.
func (o *DetailOverlay) View(width, height int) string {
	p := o.peer
	status := "offline"
	if p.Online {
		status = "online"
	}

	var b strings.Builder
	b.WriteString(overlayTitleStyle.Render("Peer Details") + "\n\n")
	b.WriteString(fmt.Sprintf("  %s  %s\n", statusKeyStyle.Render("NodeID:"), p.NodeID))
	b.WriteString(fmt.Sprintf("  %s  %s\n", statusKeyStyle.Render("Status:"), status))
	b.WriteString(fmt.Sprintf("  %s  %s\n", statusKeyStyle.Render("Trust: "), p.TrustState))
	b.WriteString(fmt.Sprintf("  %s  %s\n", statusKeyStyle.Render("Seen:  "), p.LastSeen))

	if len(p.Endpoints) > 0 {
		b.WriteString(fmt.Sprintf("  %s\n", statusKeyStyle.Render("Endpoints:")))
		for _, ep := range p.Endpoints {
			b.WriteString("    " + ep + "\n")
		}
	}

	b.WriteString("\n" + dimItemStyle.Render("Press Esc to close"))

	maxW := min(width-4, 65)
	return overlayStyle.Width(maxW).Render(b.String())
}
