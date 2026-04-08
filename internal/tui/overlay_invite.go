package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// InviteOverlay displays a generated invite token.
type InviteOverlay struct {
	token   string
	err     error
	loading bool
}

// NewInviteOverlay creates an invite overlay in loading state.
func NewInviteOverlay() *InviteOverlay {
	return &InviteOverlay{loading: true}
}

// Update handles input and invite results.
func (o *InviteOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, keys.Escape) {
			return nil, nil
		}
	case inviteResult:
		o.loading = false
		if msg.err != nil {
			o.err = msg.err
		} else if msg.resp != nil {
			o.token = msg.resp.Token
		}
	}
	return o, nil
}

// View renders the invite overlay.
func (o *InviteOverlay) View(width, height int) string {
	var content string

	if o.loading {
		content = overlayTitleStyle.Render("Invite") + "\n\n" +
			dimItemStyle.Render("Generating...")
	} else if o.err != nil {
		content = overlayTitleStyle.Render("Invite") + "\n\n" +
			errorStyle.Render("Error: "+o.err.Error())
	} else {
		content = overlayTitleStyle.Render("Invite") + "\n\n" +
			"Share this token with a peer:\n\n" +
			o.token
	}

	content += "\n\n" + dimItemStyle.Render("Press Esc to close")

	maxW := min(width-4, 70)
	return overlayStyle.Width(maxW).Render(content)
}
