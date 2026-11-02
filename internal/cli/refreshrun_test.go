package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider/understat"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
)

// The TUI's one-line status bar shows only the cause, not the CLI's
// whole wrapped error chain.
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

// A first history pull says how big it is before the first season lands.
func TestHistoryScopeEstimatesByProvider(t *testing.T) {
	ctx := context.Background()
	store := storagetest.New(t)
	mls, _ := sync.FindCoverage("usa-mls")
	epl, _ := sync.FindCoverage("eng-premier-league")
	jobs := []sync.Job{
		{Competition: mls.Competition, Seasons: []string{"2026", "2025", "2024", "2023", "2022", "2021"}},
		{Competition: epl.Competition, Seasons: []string{"2025", "2024"}},
	}
	if got, want := historyScope(ctx, store, jobs), "8 seasons to fetch — about 3 minutes"; got != want {
		t.Errorf("scope %q, want %q", got, want)
	}

	// 2024 was synced after it finished, so it's left alone.
	if err := store.RecordSyncRun(ctx, models.SyncRun{Provider: "understat", CompetitionID: epl.Competition.ID,
		SeasonID: "eng-premier-league:2024", StartedAt: time.Now(), FinishedAt: time.Now(), Status: models.SyncStatusOK}); err != nil {
		t.Fatal(err)
	}
	if got, want := historyScope(ctx, store, jobs[1:]), "1 season to fetch, 1 cached — under a minute"; got != want {
		t.Errorf("scope %q, want %q", got, want)
	}
}

func TestTUIErrorPassesUnknownErrorsThrough(t *testing.T) {
	err := errors.New("database is locked")
	if got := tuiError(err); got != err {
		t.Errorf("tuiError(%v) = %v, want the error unchanged", err, got)
	}
}
