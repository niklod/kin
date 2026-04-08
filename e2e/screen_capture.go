//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/hinshun/vt10x"
)

var errScreenTimeout = errors.New("screen did not match pattern within deadline")

// screenCapture maintains a VT10x virtual terminal state machine fed by PTY output.
// It interprets ANSI escape codes (including alternate screen buffer) and provides
// the current rendered screen content for assertions.
type screenCapture struct {
	term vt10x.Terminal
	cols int
	rows int
	done chan struct{}
}

func newScreenCapture(r io.Reader, cols, rows int) *screenCapture {
	term := vt10x.New(vt10x.WithSize(cols, rows))
	sc := &screenCapture{
		term: term,
		cols: cols,
		rows: rows,
		done: make(chan struct{}),
	}
	go sc.readLoop(r)
	return sc
}

func (sc *screenCapture) readLoop(r io.Reader) {
	defer close(sc.done)
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_, _ = sc.term.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// Screen returns the current rendered screen content as plain text.
// Each row is trimmed of trailing spaces and joined with newlines.
// Thread-safe: vt10x.Terminal.Write (called by readLoop) and Lock/Unlock
// both use the same internal mutex, serializing access.
func (sc *screenCapture) Screen() string {
	sc.term.Lock()
	defer sc.term.Unlock()

	var lines []string
	for y := 0; y < sc.rows; y++ {
		var row strings.Builder
		for x := 0; x < sc.cols; x++ {
			g := sc.term.Cell(x, y)
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}
			row.WriteRune(ch)
		}
		lines = append(lines, strings.TrimRight(row.String(), " "))
	}
	return strings.Join(lines, "\n")
}

// WaitForScreen blocks until the screen content matches the given regex pattern,
// or ctx expires. Polls at 50ms intervals.
func (sc *screenCapture) WaitForScreen(ctx context.Context, pattern string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid screen pattern %q: %w", pattern, err)
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		screen := sc.Screen()
		if re.MatchString(screen) {
			return screen, nil
		}
		select {
		case <-ctx.Done():
			return screen, fmt.Errorf("%w: %q\n%s", errScreenTimeout, pattern, screen)
		case <-ticker.C:
		}
	}
}

// ContainsNow returns true if the current screen content matches the pattern
// without waiting.
func (sc *screenCapture) ContainsNow(pattern string) bool {
	re := regexp.MustCompile(pattern)
	return re.MatchString(sc.Screen())
}

// Close waits for the reader goroutine to finish.
func (sc *screenCapture) Close() {
	<-sc.done
}
