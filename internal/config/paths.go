package config

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// DefaultConfigDir returns the OS-specific config directory for kin.
// macOS/Linux: ~/.config/kin
// Windows:     %APPDATA%\kin
func DefaultConfigDir() (string, error) {
	if runtime.GOOS == "windows" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "kin"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "kin"), nil
}

// DefaultListenAddr is the default address the kin daemon listens on.
const DefaultListenAddr = "0.0.0.0:7777"

// SocketPath returns the path to the daemon control socket within cfgDir.
func SocketPath(cfgDir string) string {
	return filepath.Join(cfgDir, "kin.sock")
}

// DefaultSharedDir returns the path to the shared folder (~\Kin).
func DefaultSharedDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Kin"), nil
}

// LogDir returns the path to the debug log directory within cfgDir.
func LogDir(cfgDir string) string {
	return filepath.Join(cfgDir, "logs")
}

// DebugLogPath returns a timestamped log file path for a new debug session.
func DebugLogPath(cfgDir string) string {
	ts := time.Now().Format("2006-01-02T15-04-05")
	return filepath.Join(LogDir(cfgDir), "kin-debug-"+ts+".log")
}
