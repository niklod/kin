package config

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/suite"
)

type DebugLogSuite struct {
	suite.Suite
	tmpDir string
}

func (s *DebugLogSuite) SetupTest() {
	s.tmpDir = s.T().TempDir()
}

func (s *DebugLogSuite) TestSetupDebugLog_CreatesFileAndLogs() {
	logger, f, err := SetupDebugLog(s.tmpDir, false)
	s.Require().NoError(err)
	defer f.Close()

	logger.Info("test message", "key", "value")

	s.Require().NoError(f.Sync())

	content, err := os.ReadFile(f.Name())
	s.Require().NoError(err)
	s.Contains(string(content), "debug logging started")
	s.Contains(string(content), "test message")
	s.Contains(string(content), "key=value")
}

func (s *DebugLogSuite) TestSetupDebugLog_CreatesLogDir() {
	_, f, err := SetupDebugLog(s.tmpDir, false)
	s.Require().NoError(err)
	defer f.Close()

	info, err := os.Stat(LogDir(s.tmpDir))
	s.Require().NoError(err)
	s.True(info.IsDir())
}

func (s *DebugLogSuite) TestSetupDebugLog_AlsoStderr() {
	stderrFile := filepath.Join(s.tmpDir, "fake-stderr")
	origStderr := os.Stderr
	defer func() { os.Stderr = origStderr }()

	fakeStderr, err := os.Create(stderrFile)
	s.Require().NoError(err)
	os.Stderr = fakeStderr

	logger, f, err := SetupDebugLog(s.tmpDir, true)
	s.Require().NoError(err)
	defer f.Close()

	logger.Info("multi test")

	s.Require().NoError(f.Sync())
	s.Require().NoError(fakeStderr.Sync())
	fakeStderr.Close()

	fileContent, err := os.ReadFile(f.Name())
	s.Require().NoError(err)
	s.Contains(string(fileContent), "multi test")

	stderrContent, err := os.ReadFile(stderrFile)
	s.Require().NoError(err)
	s.Contains(string(stderrContent), "multi test")
}

func (s *DebugLogSuite) TestPruneOldLogs() {
	logsDir := LogDir(s.tmpDir)
	s.Require().NoError(os.MkdirAll(logsDir, 0750))

	names := []string{
		"kin-debug-2026-01-01T10-00-00.log",
		"kin-debug-2026-01-02T10-00-00.log",
		"kin-debug-2026-01-03T10-00-00.log",
		"kin-debug-2026-01-04T10-00-00.log",
		"kin-debug-2026-01-05T10-00-00.log",
		"kin-debug-2026-01-06T10-00-00.log",
		"kin-debug-2026-01-07T10-00-00.log",
	}
	for _, name := range names {
		s.Require().NoError(os.WriteFile(filepath.Join(logsDir, name), []byte("log"), 0644))
	}

	pruneOldLogs(logsDir, 4)

	remaining, err := filepath.Glob(filepath.Join(logsDir, "kin-debug-*.log"))
	s.Require().NoError(err)
	sort.Strings(remaining)

	s.Require().Len(remaining, 4)
	for i, path := range remaining {
		s.Equal(names[3+i], filepath.Base(path))
	}
}

func (s *DebugLogSuite) TestPruneOldLogs_FewerThanKeep() {
	logsDir := LogDir(s.tmpDir)
	s.Require().NoError(os.MkdirAll(logsDir, 0750))

	s.Require().NoError(os.WriteFile(
		filepath.Join(logsDir, "kin-debug-2026-01-01T10-00-00.log"), []byte("log"), 0644,
	))

	pruneOldLogs(logsDir, 4)

	remaining, err := filepath.Glob(filepath.Join(logsDir, "kin-debug-*.log"))
	s.Require().NoError(err)
	s.Len(remaining, 1)
}

func TestDebugLogSuite(t *testing.T) {
	suite.Run(t, new(DebugLogSuite))
}
