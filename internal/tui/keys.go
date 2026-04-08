package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Quit      key.Binding
	Tab       key.Binding
	Help      key.Binding
	Up        key.Binding
	Down      key.Binding
	First     key.Binding
	Last      key.Binding
	HalfUp    key.Binding
	HalfDown  key.Binding
	Enter     key.Binding
	Search    key.Binding
	Escape    key.Binding
	Select    key.Binding
	Download  key.Binding
	Progress  key.Binding
	Invite    key.Binding
	Join      key.Binding
	Disconnect key.Binding
}

var keys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch panel"),
	),
	Help: key.NewBinding(
		key.WithKeys("?", "f1"),
		key.WithHelp("?", "help"),
	),
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("k/↑", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j/↓", "down"),
	),
	First: key.NewBinding(
		key.WithKeys("g", "home"),
		key.WithHelp("g", "first"),
	),
	Last: key.NewBinding(
		key.WithKeys("G", "end"),
		key.WithHelp("G", "last"),
	),
	HalfUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "half page up"),
	),
	HalfDown: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "half page down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter", "l"),
		key.WithHelp("enter/l", "open/select"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	),
	Select: key.NewBinding(
		key.WithKeys(" ", "x"),
		key.WithHelp("space/x", "select"),
	),
	Download: key.NewBinding(
		key.WithKeys("D"),
		key.WithHelp("D", "download"),
	),
	Progress: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "progress"),
	),
	Invite: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "invite"),
	),
	Join: key.NewBinding(
		key.WithKeys("J"),
		key.WithHelp("J", "join"),
	),
	Disconnect: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "disconnect"),
	),
}
