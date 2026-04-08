package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
)

// maxDebugLogs is the number of debug log files to keep.
const maxDebugLogs = 5

// SetupDebugLog creates a debug log file and returns a logger that writes to it.
// When alsoStderr is true, the logger writes to both the file and stderr.
// The caller must defer closing the returned file.
func SetupDebugLog(cfgDir string, alsoStderr bool) (*slog.Logger, *os.File, error) {
	logsDir := LogDir(cfgDir)
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}

	pruneOldLogs(logsDir, maxDebugLogs-1)

	logPath := DebugLogPath(cfgDir)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, nil, fmt.Errorf("create log file: %w", err)
	}

	var w io.Writer = f
	if alsoStderr {
		w = io.MultiWriter(f, os.Stderr)
	}

	logger := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Info("debug logging started", "log_file", logPath)

	return logger, f, nil
}

// pruneOldLogs removes old debug log files, keeping only the newest `keep`.
func pruneOldLogs(logsDir string, keep int) {
	entries, err := filepath.Glob(filepath.Join(logsDir, "kin-debug-*.log"))
	if err != nil || len(entries) <= keep {
		return
	}
	sort.Strings(entries)
	for _, path := range entries[:len(entries)-keep] {
		os.Remove(path)
	}
}