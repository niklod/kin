//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Cluster manages a test cluster of relay + peer processes.
type Cluster struct {
	t           *testing.T
	kinBinary   string
	relayBinary string
	relay       *Relay
	baseDir     string
	peers       []*Peer
}

// NewCluster builds kin and relay binaries and starts a relay server.
func NewCluster(t *testing.T) *Cluster {
	t.Helper()

	baseDir := t.TempDir()
	binDir := filepath.Join(baseDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))

	repoRoot := findRepoRoot(t)

	kinBinary := filepath.Join(binDir, "kin")
	relayBinary := filepath.Join(binDir, "relay")

	buildBinary(t, repoRoot, "./cmd/kin", kinBinary)
	buildBinary(t, repoRoot, "./cmd/relay", relayBinary)

	relay := startRelay(t, relayBinary, baseDir)

	return &Cluster{
		t:           t,
		kinBinary:   kinBinary,
		relayBinary: relayBinary,
		relay:       relay,
		baseDir:     baseDir,
	}
}

// NewPeer creates a new peer with isolated config and shared directories.
// The peer is automatically registered for cleanup.
func (c *Cluster) NewPeer(name string) *Peer {
	p := newPeer(c.t, name, c.kinBinary, c.baseDir, c.relay.Addr())
	c.peers = append(c.peers, p)
	return p
}

// Cleanup stops all peers and the relay, and dumps logs on test failure.
func (c *Cluster) Cleanup() {
	for _, p := range c.peers {
		if c.t.Failed() {
			p.DumpLogs()
		}
		p.Stop()
	}
	if c.t.Failed() {
		c.relay.DumpLogs()
	}
	c.relay.Stop()
}

func buildBinary(t *testing.T, repoRoot, pkg, output string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", output, pkg)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build %s failed: %s", pkg, out)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod)")
		}
		dir = parent
	}
}
