//go:build e2e

package e2e

import (
	"context"
	"time"
)

// Key escape sequences for TUI test interaction.
const (
	tuiKeyTab   = "\t"
	tuiKeyEsc   = "\x1b"
	tuiKeyEnter = "\r"
)

func (s *E2ESuite) TestTUI_InitAndPanelDisplay() {
	d := s.cluster.NewPeer("t-init")
	d.Start()

	tui := s.cluster.NewTUIPeer("t-init-v", d)
	tui.Start()

	screen := tui.Screen()
	s.Require().Contains(screen, "Files", "catalog panel header missing")
	s.Require().Contains(screen, "Peers", "peer panel header missing")
	s.Require().Contains(screen, "(no files)", "empty catalog placeholder missing")
	s.Require().Contains(screen, "(no peers)", "empty peers placeholder missing")
}

func (s *E2ESuite) TestTUI_StatusBar() {
	d := s.cluster.NewPeer("t-stat")
	d.Start()

	tui := s.cluster.NewTUIPeer("t-stat-v", d)
	tui.Start()

	tui.WaitForScreen(`node:`)
	tui.WaitForScreen(`peers:0`)

	status := tui.IPCStatus()
	s.Require().Contains(tui.Screen(), status.NodeIDShort[:8],
		"status bar should show node ID prefix")
}

func (s *E2ESuite) TestTUI_RelayConnection() {
	d := s.cluster.NewPeer("t-relay")
	d.Start()

	tui := s.cluster.NewTUIPeer("t-relay-v", d)
	tui.Start()

	tui.WaitForScreen(`relay:`)
	s.Require().False(
		tui.ScreenContains(`disconnected`),
		"relay should not show as disconnected",
	)
}

func (s *E2ESuite) TestTUI_PanelNavigation() {
	d := s.cluster.NewPeer("t-nav")
	d.Start()

	tui := s.cluster.NewTUIPeer("t-nav-v", d)
	tui.Start()

	tui.SendKeys(tuiKeyTab)
	time.Sleep(100 * time.Millisecond)
	s.Require().True(tui.ScreenContains(`Files`), "catalog panel visible after tab")
	s.Require().True(tui.ScreenContains(`Peers`), "peer panel visible after tab")

	tui.SendKeys(tuiKeyTab)
	time.Sleep(100 * time.Millisecond)
	s.Require().True(tui.ScreenContains(`Files`), "catalog panel visible after second tab")
	s.Require().True(tui.ScreenContains(`Peers`), "peer panel visible after second tab")
}

func (s *E2ESuite) TestTUI_HelpOverlay() {
	d := s.cluster.NewPeer("t-help")
	d.Start()

	tui := s.cluster.NewTUIPeer("t-help-v", d)
	tui.Start()

	tui.SendKeys("?")
	tui.WaitForScreen(`Keybindings`)
	s.Require().True(tui.ScreenContains(`j/k`), "help should show j/k binding")
	s.Require().True(tui.ScreenContains(`quit`), "help should show quit binding")

	tui.SendKeys(tuiKeyEsc)
	tui.WaitForScreen(`Files`)
	s.Require().False(tui.ScreenContains(`Keybindings`),
		"help overlay should be closed")
}

func (s *E2ESuite) TestTUI_InviteGeneration() {
	d := s.cluster.NewPeer("t-inv")
	d.Start()

	tui := s.cluster.NewTUIPeer("t-inv-v", d)
	tui.Start()

	tui.SendKeys("i")
	tui.WaitForScreen(`Invite`)
	tui.WaitForScreenTimeout(`kin:`, 10*time.Second)

	screen := tui.Screen()
	s.Require().Contains(screen, "Share this token", "invite overlay should show instructions")
	s.Require().Contains(screen, "kin:", "invite overlay should contain token")

	tui.SendKeys(tuiKeyEsc)
	tui.WaitForScreen(`Files`)
}

func (s *E2ESuite) TestTUI_JoinViaInvite() {
	alice := s.cluster.NewPeer("t-ja")
	bob := s.cluster.NewPeer("t-jb")

	alice.Start()
	bob.Start()

	tui := s.cluster.NewTUIPeer("t-jb-v", bob)
	tui.Start()

	token := alice.Invite()

	tui.SendKeys("J")
	tui.WaitForScreen(`Paste invite token`)

	// Send token followed by Enter after a brief pause for input processing.
	tui.SendKeys(token)
	time.Sleep(200 * time.Millisecond)
	tui.SendKeys(tuiKeyEnter)

	tui.WaitForScreenTimeout(`Connected to peer`, 30*time.Second)

	tui.SendKeys(tuiKeyEsc)

	tui.WaitForScreen(`peers:1`)

	peers := tui.IPCPeers()
	s.Require().Len(peers.Peers, 1, "bob should have one peer after join")
}

func (s *E2ESuite) TestTUI_CatalogRealtimeUpdate() {
	d := s.cluster.NewPeer("t-cat")
	d.WriteFile("readme.txt", "hello from alice")
	d.Start()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := d.stderr.WaitForLine(ctx, `indexed.*readme\.txt`)
	s.Require().NoError(err, "watcher did not index readme.txt")

	tui := s.cluster.NewTUIPeer("t-cat-v", d)
	tui.Start()

	tui.WaitForScreen(`readme\.txt`)

	d.WriteFile("notes.md", "new notes content")

	tui.WaitForScreenTimeout(`notes\.md`, 10*time.Second)
}

func (s *E2ESuite) TestTUI_RemotePeerCatalog() {
	alice := s.cluster.NewPeer("t-ra")
	bob := s.cluster.NewPeer("t-rb")

	alice.WriteFile("shared-doc.txt", "shared content")
	alice.Start()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := alice.stderr.WaitForLine(ctx, `indexed.*shared-doc\.txt`)
	s.Require().NoError(err, "watcher did not index shared-doc.txt")

	bob.Start()

	tui := s.cluster.NewTUIPeer("t-rb-v", bob)
	tui.Start()

	token := alice.Invite()
	bob.Join(token)

	tui.WaitForScreenTimeout(`shared-doc\.txt`, 15*time.Second)
	s.Require().True(tui.ScreenContains(`remote`),
		"remote file should show 'remote' label")
}

func (s *E2ESuite) TestTUI_PeerOnlineOffline() {
	alice := s.cluster.NewPeer("t-oa")
	bob := s.cluster.NewPeer("t-ob")

	alice.Start()

	tui := s.cluster.NewTUIPeer("t-oa-v", alice)
	tui.Start()

	s.Require().True(tui.ScreenContains(`\(no peers\)`),
		"should show no peers initially")

	token := alice.Invite()
	bob.Start()
	bob.Join(token)

	// Verify peer comes online in TUI via the peer_online event.
	tui.WaitForScreenTimeout(`online`, 15*time.Second)

	// Verify the peer count updated in the status bar.
	screen := tui.Screen()
	s.Require().Contains(screen, "Peers  1", "peer panel should show 1 peer")
}
