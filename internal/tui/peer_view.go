package tui

import (
	"fmt"
	"strings"

	"github.com/niklod/kin/internal/daemon"
)

// PeerView renders the peer panel.
func PeerView(m PeerModel, focused bool) string {
	var b strings.Builder

	end := min(m.offset+m.height, len(m.peers))

	for i := m.offset; i < end; i++ {
		p := m.peers[i]
		line := formatPeerEntry(p)

		if i == m.cursor {
			line = selectedItemStyle.Width(m.width - 4).Render(line)
		} else if !p.Online {
			line = dimItemStyle.Render(line)
		} else {
			line = normalItemStyle.Render(line)
		}

		b.WriteString(line)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	rendered := strings.Count(b.String(), "\n") + 1
	if len(m.peers) == 0 {
		b.WriteString(dimItemStyle.Render("  (no peers)"))
		rendered = 1
	}
	for i := rendered; i < m.height; i++ {
		b.WriteByte('\n')
	}

	content := b.String()
	header := titleStyle.Render(" Peers") +
		dimItemStyle.Render(fmt.Sprintf("  %d", len(m.peers)))

	border := borderStyle
	if focused {
		border = focusedBorderStyle
	}

	return border.Width(m.width).Render(
		header + "\n" + content,
	)
}

// formatPeerEntry formats a single peer line.
func formatPeerEntry(p daemon.PeerInfo) string {
	status := "offline"
	if p.Online {
		status = "online"
	}

	id := p.NodeIDShort
	if len(id) > 12 {
		id = id[:12]
	}

	return fmt.Sprintf(" %s  %s", id, status)
}
