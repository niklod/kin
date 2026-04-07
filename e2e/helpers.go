//go:build e2e

package e2e

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// freePort returns an available TCP port on loopback.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// runCmd executes a one-shot command with a timeout and returns its combined output.
func runCmd(t *testing.T, timeout time.Duration, name string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// stopProcess sends SIGINT to the process and waits up to 5 seconds
// for it to exit. If it does not exit in time, it is killed.
func stopProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	_ = cmd.Process.Signal(syscall.SIGINT)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

// lineCapture captures process output line by line and supports waiting for patterns.
type lineCapture struct {
	mu      sync.Mutex
	lines   []string
	waiters []*lineWaiter
	pw      *io.PipeWriter
}

type lineWaiter struct {
	pattern *regexp.Regexp
	ch      chan string
}

func newLineCapture() *lineCapture {
	return &lineCapture{}
}

// Writer returns an io.Writer that feeds lines into the capture.
func (lc *lineCapture) Writer() io.Writer {
	pr, pw := io.Pipe()
	lc.pw = pw
	go lc.scan(pr)
	return pw
}

// Close closes the pipe writer, unblocking the scan goroutine.
func (lc *lineCapture) Close() {
	if lc.pw != nil {
		lc.pw.Close()
	}
}

func (lc *lineCapture) scan(r io.Reader) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		lc.addLine(s.Text())
	}
}

func (lc *lineCapture) addLine(line string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	lc.lines = append(lc.lines, line)

	remaining := lc.waiters[:0]
	for _, w := range lc.waiters {
		if w.pattern.MatchString(line) {
			select {
			case w.ch <- line:
			default:
			}
		} else {
			remaining = append(remaining, w)
		}
	}
	lc.waiters = remaining
}

// WaitForLine blocks until a line matching the regex pattern appears or ctx expires.
func (lc *lineCapture) WaitForLine(ctx context.Context, pattern string) (string, error) {
	re := regexp.MustCompile(pattern)

	lc.mu.Lock()
	for _, line := range lc.lines {
		if re.MatchString(line) {
			lc.mu.Unlock()
			return line, nil
		}
	}
	w := &lineWaiter{pattern: re, ch: make(chan string, 1)}
	lc.waiters = append(lc.waiters, w)
	lc.mu.Unlock()

	select {
	case line := <-w.ch:
		return line, nil
	case <-ctx.Done():
		return "", fmt.Errorf("waiting for pattern %q: %w", pattern, ctx.Err())
	}
}

// Contains checks whether any captured line matches the regex pattern.
func (lc *lineCapture) Contains(pattern string) bool {
	re := regexp.MustCompile(pattern)
	lc.mu.Lock()
	defer lc.mu.Unlock()
	for _, line := range lc.lines {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

// All returns all captured lines joined with newlines.
func (lc *lineCapture) All() string {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if len(lc.lines) == 0 {
		return ""
	}
	return strings.Join(lc.lines, "\n") + "\n"
}
