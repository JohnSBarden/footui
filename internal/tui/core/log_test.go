package core

import (
	"strconv"
	"testing"
	"time"
)

func TestLogDropsARepeatedProgressLine(t *testing.T) {
	at := time.Date(2026, 10, 20, 14, 3, 22, 0, time.UTC)
	l := NewLog(func() time.Time { return at })
	l.Add(LogInfo, "start Premier League")
	l.Add(LogProgress, "season 2025")
	l.Add(LogProgress, "season 2025")
	l.Add(LogDone, "Premier League: 20 teams")
	l.Add(LogInfo, "   ")

	got := l.Entries()
	if len(got) != 3 {
		t.Fatalf("entries %+v, want the repeat and the blank dropped", got)
	}
	if want := "14:03:22 DONE Premier League: 20 teams"; got[2].String() != want {
		t.Errorf("line %q, want %q", got[2].String(), want)
	}
}

func TestLogKeepsTheNewestEntries(t *testing.T) {
	l := NewLog(nil)
	for i := range logCapacity + 10 {
		l.Add(LogInfo, strconv.Itoa(i))
	}
	got := l.Entries()
	if len(got) != logCapacity || got[0].Text != "10" {
		t.Errorf("kept %d entries from %q", len(got), got[0].Text)
	}
}

func TestANilLogRecordsNothing(t *testing.T) {
	var l *Log
	l.Add(LogError, "nobody hears this")
	if l.Entries() != nil {
		t.Error("a nil log should have no entries")
	}
}
