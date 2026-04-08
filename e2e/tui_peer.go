//go:build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/niklod/kin/internal/config"
	"github.com/niklod/kin/internal/daemon"
	"github.com/niklod/kin/internal/ipc"
	"github.com/stretchr/testify/require"
)

const keyCtrlC = "\x03"

const (
	tuiDefaultCols  = 120
	tuiDefaultRows  = 40
	tuiStartTimeout = 15 * time.Second
)

// TUIPeer manages a kin TUI process running in a pseudo-terminal.
// It connects to an existing daemon (started by a regular Peer) and provides
// screen-based assertions and IPC-based verification.
type TUIPeer struct {
	t         *testing.T
	name      string
	binary    string
	configDir string
	sharedDir string
	relayAddr string
	cmd       *exec.Cmd
	ptmx      *os.File
	screen    *screenCapture
	ipcClient *ipc.Client
}

func newTUIPeer(t *testing.T, name, binary string, backingPeer *Peer) *TUIPeer {
	t.Helper()
	return &TUIPeer{
		t:         t,
		name:      name,
		binary:    binary,
		configDir: backingPeer.configDir,
		sharedDir: backingPeer.sharedDir,
		relayAddr: backingPeer.relayAddr,
	}
}

// Start launches the TUI in a PTY connected to the backing daemon.
// Waits until both "Files" and "Peers" panel headers are rendered.
func (tp *TUIPeer) Start() {
	tp.t.Helper()

	args := []string{
		"--config-dir", tp.configDir,
		"--shared-dir", tp.sharedDir,
	}
	if tp.relayAddr != "" {
		args = append(args, "--relay", tp.relayAddr)
	}

	tp.cmd = exec.Command(tp.binary, args...)
	ptmx, err := pty.StartWithSize(tp.cmd, &pty.Winsize{
		Cols: tuiDefaultCols,
		Rows: tuiDefaultRows,
	})
	require.NoError(tp.t, err, "start TUI %s in PTY", tp.name)

	tp.ptmx = ptmx
	tp.screen = newScreenCapture(ptmx, tuiDefaultCols, tuiDefaultRows)

	ctx, cancel := context.WithTimeout(context.Background(), tuiStartTimeout)
	defer cancel()

	_, err = tp.screen.WaitForScreen(ctx, `Files`)
	require.NoError(tp.t, err, "TUI %s: waiting for Files panel", tp.name)

	_, err = tp.screen.WaitForScreen(ctx, `Peers`)
	require.NoError(tp.t, err, "TUI %s: waiting for Peers panel", tp.name)

	tp.connectIPC()
}

func (tp *TUIPeer) connectIPC() {
	tp.t.Helper()
	sockPath := config.SocketPath(tp.configDir)
	client, err := ipc.Dial(sockPath)
	require.NoError(tp.t, err, "TUI %s: connect IPC", tp.name)
	tp.ipcClient = client
}

// SendKeys writes bytes to the TUI via the PTY. Accepts single characters ("q"),
// escape sequences ("\t", "\x1b"), or multi-character strings (e.g., an invite token).
func (tp *TUIPeer) SendKeys(text string) {
	tp.t.Helper()
	_, err := tp.ptmx.Write([]byte(text))
	require.NoError(tp.t, err, "TUI %s: send keys %q", tp.name, text)
}

// WaitForScreen blocks until the screen matches the regex pattern (10s timeout).
func (tp *TUIPeer) WaitForScreen(pattern string) {
	tp.t.Helper()
	tp.WaitForScreenTimeout(pattern, 10*time.Second)
}

// WaitForScreenTimeout blocks until the screen matches the pattern within timeout.
func (tp *TUIPeer) WaitForScreenTimeout(pattern string, timeout time.Duration) {
	tp.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := tp.screen.WaitForScreen(ctx, pattern)
	require.NoError(tp.t, err, "TUI %s: wait for screen %q", tp.name, pattern)
}

// Screen returns the current screen content as text.
func (tp *TUIPeer) Screen() string {
	return tp.screen.Screen()
}

// ScreenContains checks if the current screen matches the pattern without waiting.
func (tp *TUIPeer) ScreenContains(pattern string) bool {
	return tp.screen.ContainsNow(pattern)
}

// IPCStatus returns the daemon's status via direct IPC.
func (tp *TUIPeer) IPCStatus() *daemon.StatusResponse {
	tp.t.Helper()
	resp, err := tp.ipcClient.Status()
	require.NoError(tp.t, err, "TUI %s: IPC status", tp.name)
	return resp
}

// IPCPeers returns the peer list via direct IPC.
func (tp *TUIPeer) IPCPeers() *daemon.PeersResponse {
	tp.t.Helper()
	resp, err := tp.ipcClient.Peers()
	require.NoError(tp.t, err, "TUI %s: IPC peers", tp.name)
	return resp
}

// IPCCatalog returns the catalog via direct IPC.
func (tp *TUIPeer) IPCCatalog() *daemon.CatalogResponse {
	tp.t.Helper()
	resp, err := tp.ipcClient.Catalog()
	require.NoError(tp.t, err, "TUI %s: IPC catalog", tp.name)
	return resp
}

// Stop sends 'q' to quit the TUI, waits for exit, then cleans up.
func (tp *TUIPeer) Stop() {
	if tp.ipcClient != nil {
		tp.ipcClient.Close()
	}

	if tp.cmd != nil && tp.cmd.Process != nil {
		// Try graceful quit with 'q' key.
		_, _ = tp.ptmx.Write([]byte("q"))

		done := make(chan error, 1)
		go func() { done <- tp.cmd.Wait() }()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			// Fallback: Ctrl+C.
			_, _ = tp.ptmx.Write([]byte(keyCtrlC))
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				_ = tp.cmd.Process.Kill()
				<-done
			}
		}
	}

	if tp.ptmx != nil {
		// Closing the PTY unblocks screenCapture.readLoop, required before screen.Close().
		tp.ptmx.Close()
	}
	if tp.screen != nil {
		tp.screen.Close()
	}
}

// DumpScreen logs the current screen content (called on test failure).
func (tp *TUIPeer) DumpScreen() {
	if tp.screen == nil {
		tp.t.Logf("=== %s TUI screen === (not started)", tp.name)
		return
	}
	tp.t.Logf("=== %s TUI screen ===\n%s", tp.name, tp.screen.Screen())
}
