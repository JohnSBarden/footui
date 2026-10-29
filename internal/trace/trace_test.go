package trace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOffRecordsNothing(t *testing.T) {
	if On() {
		t.Fatal("debug mode should start off")
	}
	called := false
	Watch(func(string, string) { called = true })
	Printf("nobody hears %s", "this")
	if called {
		t.Error("a watcher heard a line with debug mode off")
	}
	_ = Stop()
}

func TestLinesGoToTheWriterAndDebugOnesToWatchers(t *testing.T) {
	var b strings.Builder
	at := time.Date(2026, 10, 28, 9, 4, 5, 250e6, time.UTC)
	done := startWriter(&b, func() time.Time { return at })
	defer done()

	var heard []string
	Watch(func(level, text string) { heard = append(heard, level+" "+text) })
	Printf("GET %s → %d", "x", 200)
	Line("DONE", "Premier League: 20 teams")

	want := "2026-10-28 09:04:05.250 DBUG GET x → 200\n2026-10-28 09:04:05.250 DONE Premier League: 20 teams\n"
	if b.String() != want {
		t.Errorf("file\n%q\nwant\n%q", b.String(), want)
	}
	if len(heard) != 1 || heard[0] != "DBUG GET x → 200" {
		t.Errorf("watchers heard %q, want only the DBUG line", heard)
	}
}

func TestStartRotatesABigFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state", "debug.log")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, maxSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Start(p); err != nil {
		t.Fatal(err)
	}
	if Path() != p || !On() {
		t.Errorf("Path %q On %v after Start", Path(), On())
	}
	if err := Stop(); err != nil {
		t.Fatal(err)
	}
	if On() {
		t.Error("still on after stop")
	}
	old, err := os.Stat(p + ".1")
	if err != nil || old.Size() != maxSize+1 {
		t.Errorf("old log not kept as .1: %v", err)
	}
	body, _ := os.ReadFile(p)
	if !strings.Contains(string(body), "DBUG session start") {
		t.Errorf("new log %q lacks the session line", body)
	}
}
