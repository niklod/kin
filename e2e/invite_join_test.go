//go:build e2e

package e2e

func (s *E2ESuite) TestInviteJoin_DirectEndpoint() {
	alice := s.cluster.NewPeer("alice")
	bob := s.cluster.NewPeer("bob")

	alice.Start()

	token := alice.Invite()
	s.Require().NotEmpty(token)

	bob.Join(token)

	bobStatus := bob.Status()
	s.Require().Equal(1, bobStatus.PeerCount, "bob should see alice as a peer")
}

func (s *E2ESuite) TestInviteJoin_ViaRelay() {
	alice := s.cluster.NewPeer("alice-relay")
	bob := s.cluster.NewPeer("bob-relay")

	alice.Start()

	token := alice.Invite()
	s.Require().Contains(token, "kin:")

	bob.Join(token)

	bobStatus := bob.Status()
	s.Require().Equal(1, bobStatus.PeerCount, "bob should see alice as a peer after relay join")
}

func (s *E2ESuite) TestInviteJoin_StatusShowsNodeIDs() {
	alice := s.cluster.NewPeer("alice-ids")
	bob := s.cluster.NewPeer("bob-ids")

	alice.Start()

	aliceStatus := alice.Status()
	s.Require().NotEmpty(aliceStatus.NodeID, "alice should have a NodeID")

	token := alice.Invite()
	bob.Join(token)

	bobStatus := bob.Status()
	s.Require().NotEmpty(bobStatus.NodeID, "bob should have a NodeID")
	s.Require().NotEqual(aliceStatus.NodeID, bobStatus.NodeID, "peers must have different NodeIDs")
}
