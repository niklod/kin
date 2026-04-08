package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// HelpOverlay displays keybinding reference.
type HelpOverlay struct{}

// NewHelpOverlay creates a help overlay.
func NewHelpOverlay() *HelpOverlay {
	return &HelpOverlay{}
}

// Update handles input. Esc or ? closes the overlay.
func (h *HelpOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if key.Matches(keyMsg, keys.Escape) || key.Matches(keyMsg, keys.Help) {
			return nil, nil
		}
	}
	return h, nil
}

// View renders the help content.
func (h *HelpOverlay) View(width, height int) string {
	bindings := []struct{ key, desc string }{
		{"j/k", "move cursor up/down"},
		{"g/G", "first/last item"},
		{"Ctrl+d/u", "half page down/up"},
		{"/", "search/filter files"},
		{"Esc", "cancel search / close overlay"},
		{"Tab", "switch panel focus"},
		{"Space/x", "toggle file selection"},
		{"D", "download selected files"},
		{"i", "generate invite link"},
		{"J", "join via invite token"},
		{"d", "disconnect peer"},
		{"Enter", "peer details / open"},
		{"p", "show transfer progress"},
		{"?", "toggle this help"},
		{"q", "quit"},
	}

	var b strings.Builder
	b.WriteString(overlayTitleStyle.Render("Keybindings") + "\n\n")

	for _, bind := range bindings {
		k := statusKeyStyle.Render(bind.key)
		b.WriteString("  " + k + "  " + bind.desc + "\n")
	}

	b.WriteString("\n" + dimItemStyle.Render("Press ? or Esc to close"))

	maxW := min(width-4, 50)
	return overlayStyle.Width(maxW).Render(b.String())
}
