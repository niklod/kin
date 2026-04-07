//go:build e2e

package e2e

import (
	"context"
	"time"
)

func (s *E2ESuite) TestCatalogExchange_AfterJoin() {
	alice := s.cluster.NewPeer("alice-cat")
	bob := s.cluster.NewPeer("bob-cat")

	alice.WriteFile("hello.txt", "hello from alice")
	alice.Start()

	// Wait for watcher to index the file (500ms debounce + processing).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := alice.stderr.WaitForLine(ctx, `indexed.*hello\.txt`)
	s.Require().NoError(err, "watcher did not index hello.txt in time")

	token := alice.Invite()
	bob.Join(token)

	s.Require().True(
		alice.StderrContains(`sending catalog offer`),
		"alice should have sent catalog offer",
	)
}

func (s *E2ESuite) TestCatalogExchange_EmptyCatalog() {
	alice := s.cluster.NewPeer("alice-empty")
	bob := s.cluster.NewPeer("bob-empty")

	alice.Start()

	token := alice.Invite()
	bob.Join(token)

	bobStatus := bob.Status()
	s.Require().Equal(1, bobStatus.PeerCount, "join should succeed even with empty catalogs")
}
