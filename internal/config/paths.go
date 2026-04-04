package config

import (
	"os"
	"path/filepath"
	"runtime"
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

// DefaultSharedDir returns the path to the shared folder (~\Kin).
func DefaultSharedDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Kin"), nil
}
