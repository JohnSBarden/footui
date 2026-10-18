package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/provider/understat"
)

// The TUI shows errors on a one-line status bar beneath a screen that
// already says what was being refreshed, so the wrapped chain the CLI
// prints ("sync: fetch teams: giving up after 3 attempts: understat: fetch
// EPL 2026: …") is mostly noise there. The cause is what fits.
func TestTUIErrorKeepsTheCauseAndDropsTheChain(t *testing.T) {
	timeout := &understat.TimeoutError{LeagueKey: "EPL", Season: "2026", Host: "understat.com", After: 15 * time.Second}
	err := fmt.Errorf("sync: fetch teams: %w", &understat.ExhaustedError{Attempts: 3, Last: timeout})

	got := tuiError(err).Error()
	for _, want := range []string{"no response within", "15s", "3 attempts"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q should mention %q", got, want)
		}
	}
	for _, noise := range []string{"sync:", "fetch teams", "EPL"} {
		if strings.Contains(got, noise) {
			t.Errorf("%q should not carry the wrapping chain (%q)", got, noise)
		}
	}
}

func TestTUIErrorPassesUnknownErrorsThrough(t *testing.T) {
	err := errors.New("database is locked")
	if got := tuiError(err); got != err {
		t.Errorf("tuiError(%v) = %v, want the error unchanged", err, got)
	}
}
