// Package config resolves footui's on-disk locations, following the XDG
// base directory convention, and reads the user's settings file.
package config

import (
	"os"
	"path/filepath"
)

// DefaultDBPath returns $XDG_DATA_HOME/footui/footui.db, falling back to
// ~/.local/share/footui/footui.db when XDG_DATA_HOME is unset. A database
// written before the command was renamed, footy.db beside it, is used
// when footui.db doesn't exist yet, so nobody's cache goes missing.
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
