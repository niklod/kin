package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/niklod/kin/internal/daemon"
)

type panel int

const (
	panelCatalog panel = iota
	panelPeers
)

// Model is the top-level bubbletea model.
type Model struct {
	client  DaemonClient
	eventCh <-chan daemon.Event

	catalog  CatalogModel
	peers    PeerModel
	status   StatusBarModel
	progress ProgressModel

	focus    panel
	overlay  Overlay
	width    int
	height   int
	err      error
	quitting bool
}

// NewModel creates the top-level model with the given client and event channel.
func NewModel(client DaemonClient, eventCh <-chan daemon.Event) Model {
	return Model{
		client:  client,
		eventCh: eventCh,
		catalog: NewCatalogModel(),
		focus:   panelCatalog,
	}
}

// Init issues initial data fetch commands.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchStatus(m.client),
		fetchPeers(m.client),
		fetchCatalog(m.client),
		waitForEvent(m.eventCh),
	)
}

// Update processes messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case statusResult:
		if msg.err == nil && msg.resp != nil {
			m.status.SetStatus(msg.resp)
		} else if msg.err != nil {
			m.err = msg.err
		}
		return m, nil

	case peersResult:
		if msg.err != nil {
			m.err = msg.err
		} else if msg.resp != nil {
			m.peers.SetPeers(msg.resp.Peers)
			m.status.SetPeerCount(len(msg.resp.Peers))
		}
		return m, nil

	case catalogResult:
		if msg.err != nil {
			m.err = msg.err
		} else if msg.resp != nil {
			m.catalog.SetFiles(msg.resp.Files)
		}
		return m, nil

	case daemonEvent:
		return m.handleEvent(msg.Event)

	case daemonDisconnected:
		m.err = errDaemonDisconnected
		return m, nil

	case inviteResult:
		return m.handleOverlayResult(msg)

	case joinResult:
		return m.handleOverlayResult(msg)

	case downloadResult:
		m.progress.MarkDone(msg.fileID, msg.err)
		if msg.err == nil {
			return m, fetchCatalog(m.client)
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C always quits.
	if msg.Type == tea.KeyCtrlC {
		m.quitting = true
		return m, tea.Quit
	}

	// Overlay captures all keys when active.
	if m.overlay != nil {
		overlay, cmd := m.overlay.Update(msg)
		m.overlay = overlay
		return m, cmd
	}

	// q quits when not searching.
	if key.Matches(msg, keys.Quit) && !m.catalog.searching {
		m.quitting = true
		return m, tea.Quit
	}

	// Tab switches focus.
	if key.Matches(msg, keys.Tab) {
		if m.focus == panelCatalog {
			m.focus = panelPeers
		} else {
			m.focus = panelCatalog
		}
		return m, nil
	}

	// Help overlay.
	if key.Matches(msg, keys.Help) {
		m.overlay = NewHelpOverlay()
		return m, nil
	}

	// Toggle progress view.
	if key.Matches(msg, keys.Progress) {
		m.progress.Toggle()
		return m, nil
	}

	// Download selected files.
	if key.Matches(msg, keys.Download) {
		return m.startDownloads()
	}

	// Invite (global — works from either panel).
	if key.Matches(msg, keys.Invite) {
		m.overlay = NewInviteOverlay()
		return m, doInvite(m.client)
	}

	// Join (global).
	if key.Matches(msg, keys.Join) {
		m.overlay = NewJoinOverlay(m.client)
		return m, nil
	}

	// Route to focused panel.
	var cmd tea.Cmd
	switch m.focus {
	case panelCatalog:
		m.catalog, cmd = m.catalog.Update(msg)
	case panelPeers:
		m.peers, cmd = m.peers.Update(msg)

		// Peer-specific overlays.
		if key.Matches(msg, keys.Enter) {
			if p := m.peers.SelectedPeer(); p != nil {
				m.overlay = NewDetailOverlay(*p)
			}
		}
		if key.Matches(msg, keys.Disconnect) {
			if p := m.peers.SelectedPeer(); p != nil {
				m.overlay = NewConfirmOverlay(
					"Remove peer "+p.NodeIDShort+"?",
					func() tea.Msg { return nil },
				)
			}
		}
	}
	return m, cmd
}

func (m Model) startDownloads() (tea.Model, tea.Cmd) {
	selected := m.catalog.SelectedFiles()
	if len(selected) == 0 {
		return m, nil
	}

	var cmds []tea.Cmd
	for _, f := range selected {
		if !f.IsLocal {
			m.progress.AddTransfer(f)
			cmds = append(cmds, doDownload(m.client, f.FileID))
		}
	}

	if len(cmds) > 0 {
		m.progress.visible = true
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleEvent(ev daemon.Event) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case daemon.EventPeerOnline:
		if info, ok := decodePeerInfo(ev.Data); ok {
			m.peers.UpdatePeerOnline(info)
			m.status.SetPeerCount(len(m.peers.peers))
		}
	case daemon.EventPeerOffline:
		if off, ok := decodePeerOffline(ev.Data); ok {
			m.peers.UpdatePeerOffline(off.NodeID)
		}
	case daemon.EventCatalogUpdated:
		return m, tea.Batch(fetchCatalog(m.client), waitForEvent(m.eventCh))
	}
	return m, waitForEvent(m.eventCh)
}

func (m Model) handleOverlayResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.overlay != nil {
		overlay, cmd := m.overlay.Update(msg)
		m.overlay = overlay
		return m, cmd
	}
	return m, nil
}

// View renders the full TUI.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	if m.err != nil {
		return errorStyle.Render("Error: "+m.err.Error()) + "\n\n" +
			dimItemStyle.Render("Press q to quit")
	}

	// Render panels (dimensions already set by recalcLayout via WindowSizeMsg).
	catalogView := CatalogView(m.catalog, m.focus == panelCatalog)
	peerView := PeerView(m.peers, m.focus == panelPeers)
	statusView := m.status.View()

	panels := lipgloss.JoinHorizontal(lipgloss.Top, catalogView, peerView)
	progressView := m.progress.View()

	var parts []string
	parts = append(parts, panels)
	if progressView != "" {
		parts = append(parts, progressView)
	}
	parts = append(parts, statusView)
	screen := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Render overlay on top if active.
	if m.overlay != nil {
		overlayView := m.overlay.View(m.width, m.height)
		screen = lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			overlayView,
			lipgloss.WithWhitespaceChars(" "),
		)
	}

	return screen
}

func (m *Model) recalcLayout() {
	catalogW, peerW := m.panelWidths()
	panelH := m.height - statusBarHeight - 2
	m.catalog.SetSize(catalogW, panelH)
	m.peers.SetSize(peerW, panelH)
	m.status.SetWidth(m.width)
	m.progress.SetWidth(m.width)
}

func (m Model) panelWidths() (int, int) {
	if m.width < minWidth {
		return m.width, 0
	}
	catalogW := m.width * catalogWidthPct / 100
	peerW := m.width - catalogW
	return catalogW, peerW
}
