package tui

import (
	"testing"

	"github.com/niklod/kin/internal/daemon"
	"github.com/stretchr/testify/suite"
)

type PeerSuite struct {
	suite.Suite
	model PeerModel
}

func (s *PeerSuite) SetupTest() {
	s.model = PeerModel{}
	s.model.SetPeers(testPeers())
	s.model.SetSize(30, 10)
}

func (s *PeerSuite) TestCursorDown() {
	s.model, _ = s.model.Update(keyMsg('j'))
	s.Require().Equal(1, s.model.Cursor())
}

func (s *PeerSuite) TestCursorUp() {
	s.model.cursor = 1
	s.model, _ = s.model.Update(keyMsg('k'))
	s.Require().Equal(0, s.model.Cursor())
}

func (s *PeerSuite) TestCursorClampsAtEnd() {
	s.model.cursor = len(s.model.peers) - 1
	s.model, _ = s.model.Update(keyMsg('j'))
	s.Require().Equal(len(s.model.peers)-1, s.model.Cursor())
}

func (s *PeerSuite) TestSelectedPeer() {
	s.model.cursor = 1
	p := s.model.SelectedPeer()
	s.Require().NotNil(p)
	s.Require().Equal("bbb", p.NodeID)
}

func (s *PeerSuite) TestSelectedPeerEmpty() {
	s.model.SetPeers(nil)
	s.Require().Nil(s.model.SelectedPeer())
}

func (s *PeerSuite) TestUpdatePeerOnline() {
	s.model.UpdatePeerOnline(daemon.PeerInfo{NodeID: "aaa"})
	s.Require().True(s.model.peers[0].Online)
}

func (s *PeerSuite) TestUpdatePeerOffline() {
	s.model.peers[0].Online = true
	s.model.UpdatePeerOffline("aaa")
	s.Require().False(s.model.peers[0].Online)
}

func (s *PeerSuite) TestUpdatePeerOnlineNewPeer() {
	s.model.UpdatePeerOnline(daemon.PeerInfo{NodeID: "new"})
	s.Require().Len(s.model.peers, 3)
	s.Require().True(s.model.peers[2].Online)
}

func (s *PeerSuite) TestViewContainsPeerIDs() {
	view := PeerView(s.model, true)
	s.Require().Contains(view, "aaa")
	s.Require().Contains(view, "bbb")
}

func TestPeerSuite(t *testing.T) {
	suite.Run(t, new(PeerSuite))
}

func testPeers() []daemon.PeerInfo {
	return []daemon.PeerInfo{
		{NodeID: "aaa", NodeIDShort: "aaa", TrustState: "tofu", Online: true},
		{NodeID: "bbb", NodeIDShort: "bbb", TrustState: "tofu", Online: false},
	}
}
