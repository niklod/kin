package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/niklod/kin/internal/daemon"
	"github.com/stretchr/testify/suite"
)

type CatalogSuite struct {
	suite.Suite
	model CatalogModel
}

func (s *CatalogSuite) SetupTest() {
	s.model = NewCatalogModel()
	s.model.SetFiles(testFiles())
	s.model.SetSize(80, 20)
}

func (s *CatalogSuite) TestCursorDown() {
	s.model, _ = s.model.Update(keyMsg('j'))
	s.Require().Equal(1, s.model.Cursor())
}

func (s *CatalogSuite) TestCursorUp() {
	s.model.cursor = 2
	s.model, _ = s.model.Update(keyMsg('k'))
	s.Require().Equal(1, s.model.Cursor())
}

func (s *CatalogSuite) TestCursorClampsAtEnd() {
	s.model.cursor = len(s.model.files) - 1
	s.model, _ = s.model.Update(keyMsg('j'))
	s.Require().Equal(len(s.model.files)-1, s.model.Cursor())
}

func (s *CatalogSuite) TestCursorClampsAtStart() {
	s.model.cursor = 0
	s.model, _ = s.model.Update(keyMsg('k'))
	s.Require().Equal(0, s.model.Cursor())
}

func (s *CatalogSuite) TestGoToFirst() {
	s.model.cursor = 3
	s.model, _ = s.model.Update(keyMsg('g'))
	s.Require().Equal(0, s.model.Cursor())
}

func (s *CatalogSuite) TestGoToLast() {
	s.model, _ = s.model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	s.Require().Equal(len(s.model.files)-1, s.model.Cursor())
}

func (s *CatalogSuite) TestHalfPageDown() {
	s.model, _ = s.model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	// height/2 = 10, but clamped to last index (4) since only 5 files
	s.Require().Equal(4, s.model.Cursor())
}

func (s *CatalogSuite) TestHalfPageUp() {
	s.model.cursor = 4
	s.model, _ = s.model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	// 4 - 10 = -6, clamped to 0
	s.Require().Equal(0, s.model.Cursor())
}

func (s *CatalogSuite) TestSearchActivates() {
	s.model, _ = s.model.Update(keyMsg('/'))
	s.Require().True(s.model.searching)
}

func (s *CatalogSuite) TestSearchFilters() {
	s.model, _ = s.model.Update(keyMsg('/'))
	s.model, _ = s.model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	s.model, _ = s.model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

	visible := s.model.Visible()
	for _, f := range visible {
		s.Require().Contains(f.Name, "he")
	}
}

func (s *CatalogSuite) TestSearchEscResets() {
	s.model, _ = s.model.Update(keyMsg('/'))
	s.model, _ = s.model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	s.model, _ = s.model.Update(tea.KeyMsg{Type: tea.KeyEsc})

	s.Require().False(s.model.searching)
	s.Require().Equal(len(testFiles()), len(s.model.Visible()))
}

func (s *CatalogSuite) TestToggleSelection() {
	s.model, _ = s.model.Update(keyMsg(' '))
	s.Require().True(s.model.IsSelected(0))

	s.model, _ = s.model.Update(keyMsg(' '))
	s.Require().False(s.model.IsSelected(0))
}

func (s *CatalogSuite) TestSelectedFiles() {
	s.model.selected["aaa"] = true
	s.model.selected["ccc"] = true

	sel := s.model.SelectedFiles()
	s.Require().Len(sel, 2)
}

func (s *CatalogSuite) TestEmptyList() {
	s.model.SetFiles(nil)
	s.Require().Equal(0, s.model.Cursor())
	s.Require().Empty(s.model.Visible())
}

func (s *CatalogSuite) TestViewContainsFileNames() {
	view := CatalogView(s.model, true)
	s.Require().Contains(view, "hello.txt")
	s.Require().Contains(view, "photo.jpg")
}

func TestCatalogSuite(t *testing.T) {
	suite.Run(t, new(CatalogSuite))
}

func testFiles() []daemon.FileInfo {
	return []daemon.FileInfo{
		{FileID: "aaa", Name: "hello.txt", Size: 1024, IsLocal: true},
		{FileID: "bbb", Name: "photo.jpg", Size: 2048000, IsLocal: false},
		{FileID: "ccc", Name: "document.pdf", Size: 512000, IsLocal: true},
		{FileID: "ddd", Name: "notes.md", Size: 256, IsLocal: false},
		{FileID: "eee", Name: "archive.zip", Size: 10240000, IsLocal: false},
	}
}

func keyMsg(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}
