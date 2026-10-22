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
	LogInfo LogLevel = iota
	LogProgress
	LogDone
	// LogError is a failed download, or the source refusing more.
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

const logCapacity = 500

// Log is the in-memory activity log of background downloads, written by
// the root app. Safe for concurrent use.
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

// Add drops a progress line repeating the one before it. A nil log
// records nothing.
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

// FrameMsg is one animation frame for the top screen while it's Animating.
type FrameMsg struct{}

// Animated screens get FrameMsgs only while Animating reports true.
type Animated interface {
	Animating() bool
}
