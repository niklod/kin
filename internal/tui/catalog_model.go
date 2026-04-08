package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/niklod/kin/internal/daemon"
)

// CatalogModel manages the file catalog display and navigation.
type CatalogModel struct {
	files     []daemon.FileInfo
	cursor    int
	offset    int // viewport scroll offset
	selected  map[string]bool // keyed by FileID for stability across refreshes
	search    string
	searching bool
	filtered  []daemon.FileInfo
	height    int
	width     int
}

// NewCatalogModel creates a new catalog model.
func NewCatalogModel() CatalogModel {
	return CatalogModel{
		selected: make(map[string]bool),
	}
}

// SetFiles replaces the file list and resets cursor/filter state.
func (m *CatalogModel) SetFiles(files []daemon.FileInfo) {
	m.files = files
	m.applyFilter()
	m.clampCursor()
}

// SetSize updates the available dimensions.
func (m *CatalogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Visible returns the currently visible file list (filtered or full).
func (m CatalogModel) Visible() []daemon.FileInfo {
	if m.searching && m.search != "" {
		return m.filtered
	}
	return m.files
}

// Cursor returns the current cursor position.
func (m CatalogModel) Cursor() int {
	return m.cursor
}

// SelectedFiles returns all selected file infos.
func (m CatalogModel) SelectedFiles() []daemon.FileInfo {
	var result []daemon.FileInfo
	for _, f := range m.Visible() {
		if m.selected[f.FileID] {
			result = append(result, f)
		}
	}
	return result
}

// IsSelected returns true if the file at index i in the visible list is selected.
func (m CatalogModel) IsSelected(i int) bool {
	visible := m.Visible()
	if i < 0 || i >= len(visible) {
		return false
	}
	return m.selected[visible[i].FileID]
}

// Update handles key messages for the catalog panel.
func (m CatalogModel) Update(msg tea.Msg) (CatalogModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	// In search mode, handle text input.
	if m.searching {
		return m.updateSearch(keyMsg)
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
		m.cursor = max(0, len(m.Visible())-1)
		m.scrollToCursor()
	case key.Matches(keyMsg, keys.HalfDown):
		m.cursor += m.height / 2
		m.clampCursor()
		m.scrollToCursor()
	case key.Matches(keyMsg, keys.HalfUp):
		m.cursor -= m.height / 2
		m.clampCursor()
		m.scrollToCursor()
	case key.Matches(keyMsg, keys.Search):
		m.searching = true
		m.search = ""
	case key.Matches(keyMsg, keys.Select):
		m.toggleSelection()
	}
	return m, nil
}

func (m CatalogModel) updateSearch(keyMsg tea.KeyMsg) (CatalogModel, tea.Cmd) {
	switch {
	case key.Matches(keyMsg, keys.Escape):
		m.searching = false
		m.search = ""
		m.applyFilter()
		m.clampCursor()
	case keyMsg.Type == tea.KeyBackspace:
		if len(m.search) > 0 {
			m.search = m.search[:len(m.search)-1]
			m.applyFilter()
			m.clampCursor()
		}
	case keyMsg.Type == tea.KeyRunes:
		m.search += string(keyMsg.Runes)
		m.applyFilter()
		m.cursor = 0
		m.offset = 0
	case key.Matches(keyMsg, keys.Down):
		m.cursor++
		m.clampCursor()
		m.scrollToCursor()
	case key.Matches(keyMsg, keys.Up):
		m.cursor--
		m.clampCursor()
		m.scrollToCursor()
	}
	return m, nil
}

func (m *CatalogModel) toggleSelection() {
	visible := m.Visible()
	if m.cursor < 0 || m.cursor >= len(visible) {
		return
	}
	fileID := visible[m.cursor].FileID
	if m.selected[fileID] {
		delete(m.selected, fileID)
	} else {
		m.selected[fileID] = true
	}
}

func (m *CatalogModel) applyFilter() {
	if m.search == "" {
		m.filtered = nil
		return
	}
	lower := strings.ToLower(m.search)
	m.filtered = nil
	for _, f := range m.files {
		if strings.Contains(strings.ToLower(f.Name), lower) {
			m.filtered = append(m.filtered, f)
		}
	}
}

func (m *CatalogModel) clampCursor() {
	n := len(m.Visible())
	if n == 0 {
		m.cursor = 0
		return
	}
	m.cursor = max(0, min(m.cursor, n-1))
}

func (m *CatalogModel) scrollToCursor() {
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
