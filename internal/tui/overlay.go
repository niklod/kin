package tui

import tea "github.com/charmbracelet/bubbletea"

// Overlay is a modal dialog that captures all input while active.
// Returning nil from Update signals the overlay should close.
type Overlay interface {
	Update(msg tea.Msg) (Overlay, tea.Cmd)
	View(width, height int) string
}
