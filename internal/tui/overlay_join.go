package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// JoinOverlay provides a text input for pasting an invite token.
type JoinOverlay struct {
	input   textinput.Model
	client  DaemonClient
	err     error
	success string
	loading bool
}

// NewJoinOverlay creates a join overlay with a text input.
func NewJoinOverlay(client DaemonClient) *JoinOverlay {
	ti := textinput.New()
	ti.Placeholder = "kin:..."
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 50

	return &JoinOverlay{
		input:  ti,
		client: client,
	}
}

// Update handles input and join results.
func (o *JoinOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, keys.Escape) {
			return nil, nil
		}
		if msg.Type == tea.KeyEnter && !o.loading {
			token := o.input.Value()
			if token == "" {
				o.err = errEmptyToken
				return o, nil
			}
			o.loading = true
			o.err = nil
			return o, doJoin(o.client, token)
		}

	case joinResult:
		o.loading = false
		if msg.err != nil {
			o.err = msg.err
		} else if msg.resp != nil {
			o.success = msg.resp.PeerNodeID
		}
		return o, nil
	}

	// Forward to text input.
	if !o.loading && o.success == "" {
		var cmd tea.Cmd
		o.input, cmd = o.input.Update(msg)
		return o, cmd
	}
	return o, nil
}

// View renders the join overlay.
func (o *JoinOverlay) View(width, height int) string {
	var content string

	if o.success != "" {
		content = overlayTitleStyle.Render("Join") + "\n\n" +
			"Connected to peer " + o.success[:min(16, len(o.success))] + "\n\n" +
			dimItemStyle.Render("Press Esc to close")
	} else {
		content = overlayTitleStyle.Render("Join") + "\n\n" +
			"Paste invite token:\n\n" +
			o.input.View()

		if o.loading {
			content += "\n\n" + dimItemStyle.Render("Connecting...")
		}
		if o.err != nil {
			content += "\n\n" + errorStyle.Render("Error: "+o.err.Error())
		}
		content += "\n\n" + dimItemStyle.Render("Enter=join  Esc=cancel")
	}

	maxW := min(width-4, 60)
	return overlayStyle.Width(maxW).Render(content)
}
