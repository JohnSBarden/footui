// Package trace is footui's debug log. Any layer can write to it; while
// debug mode is off every call is a no-op. When on, lines go to a file a
// user can attach to a bug report, and to whoever is watching (the TUI's
// log panel).
package trace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// maxSize is how big the file may grow before the next session starts a
// fresh one, keeping the old as path.1.
const maxSize = 5 << 20

var (
	mu       sync.Mutex
	out      io.Writer
	closer   io.Closer
	path     string
	watchers []func(level, text string)
	clock    = time.Now
)

// Start turns debug mode on, appending to the file at p, until Stop.
func Start(p string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("debug log: %w", err)
	}
	if info, err := os.Stat(p); err == nil && info.Size() > maxSize {
		_ = os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("debug log: %w", err)
	}
	mu.Lock()
	out, closer, path = f, f, p
	mu.Unlock()
	Printf("session start (pid %d)", os.Getpid())
	return nil
}

// Stop turns debug mode off and closes the file. Safe when it's off.
func Stop() error {
	mu.Lock()
	defer mu.Unlock()
	c := closer
	out, closer, path, watchers = nil, nil, "", nil
	if c == nil {
		return nil
	}
	return c.Close()
}

// startWriter is Start for tests: lines go to w.
func startWriter(w io.Writer, now func() time.Time) func() error {
	mu.Lock()
	out, closer, path, clock = w, nil, "", now
	mu.Unlock()
	return func() error {
		err := Stop()
		mu.Lock()
		clock = time.Now
		mu.Unlock()
		return err
	}
}

// On reports whether debug mode is on, so a caller can skip building an
// expensive line.
func On() bool {
	mu.Lock()
	defer mu.Unlock()
	return out != nil
}

// Path is the debug log's file, or "" when debug mode is off.
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return path
}

// Watch sends every later line to fn as well. fn must not call back into
// trace.
func Watch(fn func(level, text string)) {
	mu.Lock()
	defer mu.Unlock()
	watchers = append(watchers, fn)
}

// Printf records a DBUG line.
func Printf(format string, args ...any) {
	Line("DBUG", fmt.Sprintf(format, args...))
}

// Line records text under level, a four-letter tag like the activity
// log's ("INFO", "FAIL"). The file gets every line; watchers get only
// DBUG ones, since the rest come from them.
func Line(level, text string) {
	mu.Lock()
	if out == nil {
		mu.Unlock()
		return
	}
	fmt.Fprintf(out, "%s %s %s\n", clock().Format("2006-01-02 15:04:05.000"), level, text)
	var ws []func(level, text string)
	if level == "DBUG" {
		ws = watchers
	}
	mu.Unlock()
	for _, w := range ws {
		w(level, text)
	}
}
