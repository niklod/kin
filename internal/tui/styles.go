package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Panel borders.
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	focusedBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("69"))

	// Status bar.
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	statusKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("69")).
			Bold(true)

	// Catalog items.
	selectedItemStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("230"))

	normalItemStyle = lipgloss.NewStyle()

	dimItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	// Overlay.
	overlayStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("69")).
			Padding(1, 2)

	overlayTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("69"))

	// General.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("69"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	checkboxOn  = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("[x]")
	checkboxOff = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[ ]")
)

const (
	catalogWidthPct = 70
	minWidth        = 80
	statusBarHeight = 1
)
