package tui

import tea "github.com/charmbracelet/bubbletea"

// ConfirmOverlay shows a y/n confirmation dialog.
type ConfirmOverlay struct {
	message string
	action  func() tea.Msg
}

// NewConfirmOverlay creates a confirmation dialog.
func NewConfirmOverlay(message string, action func() tea.Msg) *ConfirmOverlay {
	return &ConfirmOverlay{
		message: message,
		action:  action,
	}
}

// Update handles y/n input.
func (o *ConfirmOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil
	}
	switch keyMsg.String() {
	case "y", "Y":
		return nil, func() tea.Msg { return o.action() }
	case "n", "N", "esc":
		return nil, nil
	}
	return o, nil
}

// View renders the confirmation dialog.
func (o *ConfirmOverlay) View(width, height int) string {
	content := overlayTitleStyle.Render("Confirm") + "\n\n" +
		o.message + "\n\n" +
		statusKeyStyle.Render("y") + " yes  " +
		statusKeyStyle.Render("n") + " no"

	maxW := min(width-4, 50)
	return overlayStyle.Width(maxW).Render(content)
}
