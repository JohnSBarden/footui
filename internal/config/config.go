// Package config resolves footui's on-disk locations, following the XDG
// base directory convention, and reads the user's settings file.
package config

import (
	"os"
	"path/filepath"
)

// DefaultDBPath returns $XDG_DATA_HOME/footui/footy.db, falling back to
// ~/.local/share/footui/footy.db when XDG_DATA_HOME is unset.
func DefaultDBPath() string {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "footui", "footy.db")
}
