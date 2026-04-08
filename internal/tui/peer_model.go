package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/niklod/kin/internal/daemon"
)

// PeerModel manages the peer list display and navigation.
type PeerModel struct {
	peers  []daemon.PeerInfo
	cursor int
	offset int
	height int
	width  int
}

// SetPeers replaces the peer list.
func (m *PeerModel) SetPeers(peers []daemon.PeerInfo) {
	m.peers = peers
	m.clampCursor()
}

// SetSize updates the available dimensions.
func (m *PeerModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Peers returns the peer list.
func (m PeerModel) Peers() []daemon.PeerInfo {
	return m.peers
}

// Cursor returns the current cursor position.
func (m PeerModel) Cursor() int {
	return m.cursor
}

// SelectedPeer returns the peer at the cursor, or nil if empty.
func (m PeerModel) SelectedPeer() *daemon.PeerInfo {
	if len(m.peers) == 0 || m.cursor >= len(m.peers) {
		return nil
	}
	return &m.peers[m.cursor]
}

// UpdatePeerOnline marks a peer as online or adds it.
func (m *PeerModel) UpdatePeerOnline(info daemon.PeerInfo) {
	for i, p := range m.peers {
		if p.NodeID == info.NodeID {
			m.peers[i].Online = true
			return
		}
	}
	info.Online = true
	m.peers = append(m.peers, info)
}

// UpdatePeerOffline marks a peer as offline.
func (m *PeerModel) UpdatePeerOffline(nodeID string) {
	for i, p := range m.peers {
		if p.NodeID == nodeID {
			m.peers[i].Online = false
			return
		}
	}
}

// Update handles key messages for the peer panel.
func (m PeerModel) Update(msg tea.Msg) (PeerModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch {
	case key.Matches(keyMsg, keys.Down):
		m.cursor++
		m.clampCursor()
		m.scrollToCursor()
	case key.Matches(keyMsg, keys.Up):
		m.cursor--
		m.clampCursor()
		m.scrollToCursor()
	case key.Matches(keyMsg, keys.First):
		m.cursor = 0
		m.scrollToCursor()
	case key.Matches(keyMsg, keys.Last):
		m.cursor = max(0, len(m.peers)-1)
		m.scrollToCursor()
	}
	return m, nil
}

func (m *PeerModel) clampCursor() {
	n := len(m.peers)
	if n == 0 {
		m.cursor = 0
		return
	}
	m.cursor = max(0, min(m.cursor, n-1))
}

func (m *PeerModel) scrollToCursor() {
	if m.height <= 0 {
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.height {
		m.offset = m.cursor - m.height + 1
	}
}
