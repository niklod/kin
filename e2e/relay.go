//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Relay manages a relay server process.
type Relay struct {
	t         *testing.T
	cmd       *exec.Cmd
	addr      string
	configDir string
	stdout    *lineCapture
	stderr    *lineCapture
}

func startRelay(t *testing.T, binary, baseDir string) *Relay {
	t.Helper()

	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	configDir := fmt.Sprintf("%s/relay-config", baseDir)
	require.NoError(t, os.MkdirAll(configDir, 0755))

	r := &Relay{
		t:         t,
		addr:      addr,
		configDir: configDir,
		stdout:    newLineCapture(),
		stderr:    newLineCapture(),
	}

	r.cmd = exec.Command(binary,
		"--listen", addr,
		"--config-dir", configDir,
		"--debug",
	)
	r.cmd.Stdout = r.stdout.Writer()
	r.cmd.Stderr = r.stderr.Writer()

	require.NoError(t, r.cmd.Start(), "start relay")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := r.stdout.WaitForLine(ctx, `^relay running`)
	require.NoError(t, err, "relay did not become ready")

	return r
}

// Addr returns the relay listen address.
func (r *Relay) Addr() string {
	return r.addr
}

// Stop sends SIGINT, waits for the relay to exit, and closes output captures.
func (r *Relay) Stop() {
	stopProcess(r.cmd)
	r.stdout.Close()
	r.stderr.Close()
}

// DumpLogs writes captured stdout/stderr to the test log.
func (r *Relay) DumpLogs() {
	r.t.Logf("=== relay stdout ===\n%s", r.stdout.All())
	r.t.Logf("=== relay stderr ===\n%s", r.stderr.All())
}
