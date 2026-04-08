package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/niklod/kin/internal/daemon"
	"github.com/stretchr/testify/suite"
)

type OverlaySuite struct {
	suite.Suite
}

func (s *OverlaySuite) TestHelpOverlay_EscCloses() {
	o := NewHelpOverlay()
	result, _ := o.Update(tea.KeyMsg{Type: tea.KeyEsc})
	s.Require().Nil(result)
}

func (s *OverlaySuite) TestHelpOverlay_QuestionMarkCloses() {
	o := NewHelpOverlay()
	result, _ := o.Update(keyMsg('?'))
	s.Require().Nil(result)
}

func (s *OverlaySuite) TestHelpOverlay_OtherKeyStaysOpen() {
	o := NewHelpOverlay()
	result, _ := o.Update(keyMsg('x'))
	s.Require().NotNil(result)
}

func (s *OverlaySuite) TestHelpOverlay_ViewContainsBindings() {
	o := NewHelpOverlay()
	view := o.View(80, 24)
	s.Require().Contains(view, "j/k")
	s.Require().Contains(view, "quit")
}

func (s *OverlaySuite) TestInviteOverlay_Loading() {
	o := NewInviteOverlay()
	view := o.View(80, 24)
	s.Require().Contains(view, "Generating")
}

func (s *OverlaySuite) TestInviteOverlay_ReceivesToken() {
	o := NewInviteOverlay()
	result, _ := o.Update(inviteResult{
		resp: &daemon.InviteResponse{Token: "kin:test123"},
	})
	s.Require().NotNil(result)

	view := result.View(80, 24)
	s.Require().Contains(view, "kin:test123")
}

func (s *OverlaySuite) TestInviteOverlay_EscCloses() {
	o := NewInviteOverlay()
	result, _ := o.Update(tea.KeyMsg{Type: tea.KeyEsc})
	s.Require().Nil(result)
}

func (s *OverlaySuite) TestConfirmOverlay_YConfirms() {
	called := false
	o := NewConfirmOverlay("Delete?", func() tea.Msg {
		called = true
		return nil
	})
	result, cmd := o.Update(keyMsg('y'))
	s.Require().Nil(result)
	s.Require().NotNil(cmd)
	cmd() // execute the action
	s.Require().True(called)
}

func (s *OverlaySuite) TestConfirmOverlay_NCancels() {
	o := NewConfirmOverlay("Delete?", func() tea.Msg { return nil })
	result, _ := o.Update(keyMsg('n'))
	s.Require().Nil(result)
}

func (s *OverlaySuite) TestDetailOverlay_EscCloses() {
	o := NewDetailOverlay(daemon.PeerInfo{NodeID: "abc123", TrustState: "tofu"})
	result, _ := o.Update(tea.KeyMsg{Type: tea.KeyEsc})
	s.Require().Nil(result)
}

func (s *OverlaySuite) TestDetailOverlay_ViewShowsPeerInfo() {
	o := NewDetailOverlay(daemon.PeerInfo{NodeID: "abc123", TrustState: "tofu"})
	view := o.View(80, 24)
	s.Require().Contains(view, "abc123")
	s.Require().Contains(view, "tofu")
}

func TestOverlaySuite(t *testing.T) {
	suite.Run(t, new(OverlaySuite))
}
