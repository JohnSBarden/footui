// Package config resolves footui's XDG paths and reads the settings file.
package config

import (
	"os"
	"path/filepath"
)

// DefaultDBPath returns $XDG_DATA_HOME/footui/footui.db (default
// ~/.local/share). A legacy footy.db beside it is used if footui.db
// doesn't exist yet.
func DefaultDBPath() string {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(dataHome, "footui")
	current := filepath.Join(dir, "footui.db")
	if _, err := os.Stat(current); err != nil {
		legacy := filepath.Join(dir, "footy.db")
		if _, err := os.Stat(legacy); err == nil {
			return legacy
		}
	}
	return current
}

// DebugLogPath returns $XDG_STATE_HOME/footui/debug.log (default
// ~/.local/state).
func DebugLogPath() string {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "footui", "debug.log")
}
