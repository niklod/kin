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

func (s *E2ESuite) TestCatalogExchange_LivePushAfterFileChange() {
	alice := s.cluster.NewPeer("alice-live")
	bob := s.cluster.NewPeer("bob-live")

	alice.WriteFile("initial.txt", "before connection")
	alice.Start()
	bob.Start()

	// Wait for alice's watcher to index the initial file.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := alice.stderr.WaitForLine(ctx, `indexed.*initial\.txt`)
	s.Require().NoError(err, "watcher did not index initial.txt in time")

	// Connect bob to alice.
	token := alice.Invite()
	bob.Join(token)

	// Wait for bob to receive alice's initial catalog.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	_, err = bob.stderr.WaitForLine(ctx2, `received catalog offer.*accepted=1`)
	s.Require().NoError(err, "bob should have received alice's catalog")

	// Now alice adds a NEW file AFTER connection is established.
	alice.WriteFile("new_after_connect.txt", "live push test")

	// Wait for alice's watcher to index the new file.
	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel3()
	_, err = alice.stderr.WaitForLine(ctx3, `indexed.*new_after_connect\.txt`)
	s.Require().NoError(err, "watcher did not index new_after_connect.txt in time")

	// Bob should receive a broadcast catalog offer with 2 files.
	ctx4, cancel4 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel4()
	_, err = bob.stderr.WaitForLine(ctx4, `received catalog offer.*accepted=2`)
	s.Require().NoError(err, "bob should have received updated catalog with 2 files via live push")
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
