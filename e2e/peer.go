//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Peer manages a single kin process.
type Peer struct {
	t          *testing.T
	name       string
	binary     string
	configDir  string
	sharedDir  string
	listenAddr string
	relayAddr  string
	cmd        *exec.Cmd
	stdout     *lineCapture
	stderr     *lineCapture
}

func newPeer(t *testing.T, name, binary, baseDir, relayAddr string) *Peer {
	t.Helper()

	port := freePort(t)
	peerDir := filepath.Join(baseDir, name)
	configDir := filepath.Join(peerDir, "config")
	sharedDir := filepath.Join(peerDir, "shared")
	require.NoError(t, os.MkdirAll(configDir, 0755))
	require.NoError(t, os.MkdirAll(sharedDir, 0755))

	return &Peer{
		t:          t,
		name:       name,
		binary:     binary,
		configDir:  configDir,
		sharedDir:  sharedDir,
		listenAddr: fmt.Sprintf("127.0.0.1:%d", port),
		relayAddr:  relayAddr,
	}
}

// Start launches `kin run` as a long-running process.
func (p *Peer) Start() {
	p.t.Helper()

	p.stdout = newLineCapture()
	p.stderr = newLineCapture()

	args := []string{
		"--config-dir", p.configDir,
		"--shared-dir", p.sharedDir,
		"--listen", p.listenAddr,
		"--debug",
	}
	if p.relayAddr != "" {
		args = append(args, "--relay", p.relayAddr)
	}
	args = append(args, "run")

	p.cmd = exec.Command(p.binary, args...)
	p.cmd.Stdout = p.stdout.Writer()
	p.cmd.Stderr = p.stderr.Writer()
	require.NoError(p.t, p.cmd.Start(), "start peer %s", p.name)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := p.stdout.WaitForLine(ctx, `^kin running`)
	require.NoError(p.t, err, "peer %s did not become ready", p.name)
}

// Invite runs `kin invite` and returns the invite token.
func (p *Peer) Invite() string {
	p.t.Helper()

	args := []string{
		"--config-dir", p.configDir,
		"--listen", p.listenAddr,
	}
	if p.relayAddr != "" {
		args = append(args, "--relay", p.relayAddr)
	}
	args = append(args, "invite")

	out, err := runCmd(p.t, 5*time.Second, p.binary, args...)
	require.NoError(p.t, err, "invite failed for %s: %s", p.name, out)

	token := strings.TrimSpace(out)
	require.True(p.t, strings.HasPrefix(token, "kin:"), "unexpected invite output: %s", token)
	return token
}

// Join runs `kin join <token>` and waits for the connection to complete.
func (p *Peer) Join(token string) {
	p.t.Helper()

	args := []string{
		"--config-dir", p.configDir,
		"--listen", "127.0.0.1:0",
		"join", token,
	}

	out, err := runCmd(p.t, 30*time.Second, p.binary, args...)
	require.NoError(p.t, err, "join failed for %s: %s", p.name, out)
	require.Contains(p.t, out, "connected to", "join output missing 'connected to': %s", out)
}

// StatusOutput holds parsed `kin status` results.
type StatusOutput struct {
	NodeID    string
	PeerCount int
}

// Status runs `kin status` and parses the output.
func (p *Peer) Status() StatusOutput {
	p.t.Helper()

	args := []string{
		"--config-dir", p.configDir,
		"status",
	}

	out, err := runCmd(p.t, 5*time.Second, p.binary, args...)
	require.NoError(p.t, err, "status failed for %s: %s", p.name, out)

	result := StatusOutput{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "NodeID:") {
			result.NodeID = strings.TrimSpace(strings.TrimPrefix(line, "NodeID:"))
		}
		if strings.HasPrefix(line, "Peers:") {
			countStr := strings.TrimSpace(strings.TrimPrefix(line, "Peers:"))
			if n, err := strconv.Atoi(countStr); err == nil {
				result.PeerCount = n
			}
		}
	}
	return result
}

// WriteFile writes a file to the peer's shared directory.
func (p *Peer) WriteFile(name, content string) {
	p.t.Helper()
	path := filepath.Join(p.sharedDir, name)
	require.NoError(p.t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(p.t, os.WriteFile(path, []byte(content), 0644))
}

// HasFile checks whether a file exists in the peer's shared directory.
func (p *Peer) HasFile(name string) bool {
	_, err := os.Stat(filepath.Join(p.sharedDir, name))
	return err == nil
}

// StderrContains checks whether any stderr line matches the pattern.
func (p *Peer) StderrContains(pattern string) bool {
	if p.stderr == nil {
		return false
	}
	return p.stderr.Contains(pattern)
}

// Stop sends SIGINT, waits for the peer to exit, and closes output captures.
func (p *Peer) Stop() {
	stopProcess(p.cmd)
	if p.stdout != nil {
		p.stdout.Close()
	}
	if p.stderr != nil {
		p.stderr.Close()
	}
}

// DumpLogs writes captured stdout/stderr to the test log.
func (p *Peer) DumpLogs() {
	if p.stdout != nil {
		p.t.Logf("=== %s stdout ===\n%s", p.name, p.stdout.All())
	}
	if p.stderr != nil {
		p.t.Logf("=== %s stderr ===\n%s", p.name, p.stderr.All())
	}
}
