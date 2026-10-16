package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDBPathPrefersFootuiDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)
	want := filepath.Join(home, "footui", "footui.db")
	if got := DefaultDBPath(); got != want {
		t.Fatalf("fresh install: got %s, want %s", got, want)
	}
}

func TestDefaultDBPathKeepsLegacyFootyDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)
	dir := filepath.Join(home, "footui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "footy.db")
	if err := os.WriteFile(legacy, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DefaultDBPath(); got != legacy {
		t.Fatalf("legacy only: got %s, want %s", got, legacy)
	}

	current := filepath.Join(dir, "footui.db")
	if err := os.WriteFile(current, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DefaultDBPath(); got != current {
		t.Fatalf("both present: got %s, want %s", got, current)
	}
}
