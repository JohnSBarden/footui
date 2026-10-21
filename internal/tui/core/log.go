package core

import (
	"strings"
	"sync"
	"time"
)

// LogLevel says what kind of thing a log entry records.
type LogLevel int

// Log levels.
const (
	// LogInfo is something starting, queueing or stopping.
	LogInfo LogLevel = iota
	// LogProgress is a running download's progress line.
	LogProgress
	// LogDone is a download that finished.
	LogDone
	// LogError is a download that failed, or the source refusing more.
	LogError
)

// String is the level as it reads in a log line.
func (l LogLevel) String() string {
	switch l {
	case LogProgress:
		return "PROG"
	case LogDone:
		return "DONE"
	case LogError:
		return "FAIL"
	}
	return "INFO"
}

// LogEntry is one line of the activity log.
type LogEntry struct {
	At    time.Time
	Level LogLevel
	Text  string
}

// String is the entry as one readable line: "14:03:22 DONE Premier League: 20 teams".
func (e LogEntry) String() string {
	return e.At.Format("15:04:05") + " " + e.Level.String() + " " + e.Text
}

// logCapacity is how many entries the log keeps; older ones fall off.
const logCapacity = 500

// Log is what footui has been doing in the background — downloads
// starting, their progress, how each ended — kept in memory so a screen
// can show it. It is shared by every screen through Deps, written by the
// root app, and safe to use from any goroutine. It is the start of a
// debug mode: a file sink can be added here without touching the
// writers.
type Log struct {
	mu      sync.Mutex
	entries []LogEntry
	clock   func() time.Time
}

// NewLog returns an empty log. clock stands in for time.Now; nil is the
// real clock.
func NewLog(clock func() time.Time) *Log {
	if clock == nil {
		clock = time.Now
	}
	return &Log{clock: clock}
}

// Add records text at level. A progress line repeating the one before it
// is dropped, since a slow host can report the same step many times. A
// nil log records nothing, so writers needn't check.
func (l *Log) Add(level LogLevel, text string) {
	if l == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if n := len(l.entries); n > 0 && level == LogProgress && l.entries[n-1].Level == LogProgress && l.entries[n-1].Text == text {
		return
	}
	l.entries = append(l.entries, LogEntry{At: l.clock(), Level: level, Text: text})
	if over := len(l.entries) - logCapacity; over > 0 {
		l.entries = append(l.entries[:0], l.entries[over:]...)
	}
}

// Entries is a copy of the log, oldest first.
func (l *Log) Entries() []LogEntry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]LogEntry(nil), l.entries...)
}

// FrameMsg is one animation frame, sent by the root app to the screen on
// top while it reports it is Animating.
type FrameMsg struct{}

// Animated is implemented by screens with something moving on them (the
// dashboard's selected card). The root app only ticks frames while the
// screen on top says it is animating, so nothing redraws for nothing.
type Animated interface {
	Animating() bool
}
