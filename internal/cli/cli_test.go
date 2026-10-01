package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage/sqlite"
)

// run executes the real root command with args, returning its combined
// output. Commands that launch the TUI are excluded from these tests —
// they need a terminal — but everything else runs exactly as it does for a
// user.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer

	// Never read the developer's own settings file. A test that wants one
	// sets FOOTY_CONFIG itself before calling run.
	if os.Getenv("FOOTY_CONFIG") == "" {
		t.Setenv("FOOTY_CONFIG", filepath.Join(t.TempDir(), "none.yaml"))
	}

	cmd := NewRootCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return buf.String(), err
}

// tempDB returns a path in a temp dir; the file is created on first use.
func tempDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "footy.db")
}

func TestCompetitionsListsEverySyncableCompetition(t *testing.T) {
	out, err := run(t, "competitions")
	if err != nil {
		t.Fatalf("competitions: %v", err)
	}

	for _, want := range []string{"eng-premier-league", "esp-la-liga", "ger-bundesliga",
		"ita-serie-a", "fra-ligue-1", "rus-premier-league"} {
		if !strings.Contains(out, want) {
			t.Errorf("competitions output missing %q\n---\n%s", want, out)
		}
	}
}

func TestVersionPrintsSomething(t *testing.T) {
	out, err := run(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Error("version printed nothing")
	}
}

// A dry run must touch neither the network nor the database.
func TestRefreshDryRunReportsThePlanWithoutFetching(t *testing.T) {
	db := tempDB(t)

	out, err := run(t, "refresh", "--all", "--years", "2", "--dry-run", "--db", db)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(out, "nothing will be fetched") {
		t.Errorf("a dry run must say it isn't doing anything\n---\n%s", out)
	}
	// Every covered competition has at least two seasons of history.
	if want := fmt.Sprintf("%d competition-season", 2*len(sync.Scope)); !strings.Contains(out, want) {
		t.Errorf("expected %q (every competition x 2 seasons)\n---\n%s", want, out)
	}
	// The plan is printed before the store is opened, so no file appears.
	if _, err := sqlite.Open(db); err != nil {
		t.Fatalf("opening the db afterwards should still work: %v", err)
	}
}

func TestRefreshRejectsAnOutOfScopeCompetition(t *testing.T) {
	out, err := run(t, "refresh", "--competition", "ger-frauen-bundesliga", "--dry-run", "--db", tempDB(t))
	if err == nil {
		t.Fatal("expected an error for a competition footy doesn't cover")
	}
	combined := out + err.Error()
	if !strings.Contains(combined, "eng-premier-league") {
		t.Errorf("the error should list what is supported, got: %v", err)
	}
}

func TestColorsOnAnEmptyCacheSaysSoRatherThanReportingZeroOfZero(t *testing.T) {
	out, err := run(t, "colors", "--db", tempDB(t))
	if err != nil {
		t.Fatalf("colors: %v", err)
	}
	if !strings.Contains(out, "No teams cached") {
		t.Errorf("expected an empty-cache message\n---\n%s", out)
	}
	if !strings.Contains(out, "footy refresh") {
		t.Errorf("the message should name the command that fixes it\n---\n%s", out)
	}
}

func TestColorsReportsCoverageAndNamesTheGaps(t *testing.T) {
	db := tempDB(t)
	conn, err := sqlite.Open(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	store := sqlite.NewStore(conn)
	ctx := context.Background()
	for _, team := range []models.Team{
		{ID: "eng-arsenal", Name: "Arsenal", Gender: "men"},
		{ID: "eng-not-curated", Name: "Not Curated FC", Gender: "men"},
	} {
		if err := store.UpsertTeam(ctx, team); err != nil {
			t.Fatalf("UpsertTeam: %v", err)
		}
	}
	conn.Close()

	out, err := run(t, "colors", "--db", db)
	if err != nil {
		t.Fatalf("colors: %v", err)
	}
	if !strings.Contains(out, "1 of 2 cached teams") {
		t.Errorf("expected a coverage count\n---\n%s", out)
	}
	// Naming the gap is the point — that list is how someone knows what to
	// add to the seed file.
	if !strings.Contains(out, "eng-not-curated") {
		t.Errorf("expected the uncolored club named\n---\n%s", out)
	}
}

func TestUnknownCommandFails(t *testing.T) {
	if _, err := run(t, "nonsense"); err == nil {
		t.Error("expected an error for an unknown command")
	}
}

func TestScreenCommandsRequireAName(t *testing.T) {
	for _, cmd := range []string{"league", "team", "player"} {
		if _, err := run(t, cmd); err == nil {
			t.Errorf("%s with no argument should be an error", cmd)
		}
	}
}

func TestBareFootyRejectsStrayArguments(t *testing.T) {
	if _, err := run(t, "some-team-name"); err == nil {
		t.Error("a bare argument should be rejected rather than silently ignored")
	}
}

func TestGlobalFlagsAreAvailableOnSubcommands(t *testing.T) {
	// --db is persistent; if it weren't, this would fail as an unknown flag.
	if _, err := run(t, "colors", "--db", tempDB(t), "--no-color"); err != nil {
		t.Errorf("persistent flags should work on subcommands: %v", err)
	}
}

func TestCompletionsAreAvailableForTheUsualShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		out, err := run(t, "completion", shell)
		if err != nil {
			t.Errorf("completion %s: %v", shell, err)
		}
		if len(out) < 100 {
			t.Errorf("completion %s produced %d bytes, expected a real script", shell, len(out))
		}
	}
}

func TestStatusOnAnEmptyCacheSaysHowToPopulateIt(t *testing.T) {
	out, err := run(t, "status", "--db", tempDB(t))
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "Nothing cached yet") {
		t.Errorf("expected an empty-cache message\n---\n%s", out)
	}
	if !strings.Contains(out, "footy refresh") {
		t.Errorf("the message should name the command that fixes it\n---\n%s", out)
	}
}

func TestStatusReportsWhatIsCachedAndItsFreshness(t *testing.T) {
	db := tempDB(t)
	conn, err := sqlite.Open(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	store := sqlite.NewStore(conn)
	ctx := context.Background()

	comp := models.Competition{ID: "eng-premier-league", Name: "Premier League",
		Country: "England", Tier: 1, Gender: "men", Format: "league"}
	if err := store.UpsertCompetition(ctx, comp); err != nil {
		t.Fatalf("UpsertCompetition: %v", err)
	}
	if err := store.UpsertSeason(ctx, models.Season{
		ID: "eng-premier-league:2025", CompetitionID: comp.ID, Label: "2025"}); err != nil {
		t.Fatalf("UpsertSeason: %v", err)
	}
	if err := store.RecordSyncRun(ctx, models.SyncRun{
		Provider: "understat", CompetitionID: comp.ID, SeasonID: "eng-premier-league:2025",
		StartedAt: time.Now().Add(-2 * time.Hour), FinishedAt: time.Now().Add(-2 * time.Hour),
		Status: models.SyncStatusOK,
	}); err != nil {
		t.Fatalf("RecordSyncRun: %v", err)
	}
	conn.Close()

	out, err := run(t, "status", "--db", db)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, want := range []string{"eng-premier-league", "2025-26", "2 hours ago"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q\n---\n%s", want, out)
		}
	}
}

// A failed refresh must be reported as a failure, not as data that is
// merely a bit old.
func TestStatusFlagsCompetitionsWhoseLastRefreshFailed(t *testing.T) {
	db := tempDB(t)
	conn, err := sqlite.Open(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	store := sqlite.NewStore(conn)
	ctx := context.Background()

	comp := models.Competition{ID: "esp-la-liga", Name: "La Liga",
		Country: "Spain", Tier: 1, Gender: "men", Format: "league"}
	if err := store.UpsertCompetition(ctx, comp); err != nil {
		t.Fatalf("UpsertCompetition: %v", err)
	}
	if err := store.RecordSyncRun(ctx, models.SyncRun{
		Provider: "understat", CompetitionID: comp.ID, SeasonID: "esp-la-liga:2025",
		StartedAt: time.Now().Add(-time.Hour), FinishedAt: time.Now().Add(-time.Hour),
		Status: models.SyncStatusFailed, Error: "unreachable",
	}); err != nil {
		t.Fatalf("RecordSyncRun: %v", err)
	}
	conn.Close()

	out, err := run(t, "status", "--db", db)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "FAILED") {
		t.Errorf("expected the failure flagged\n---\n%s", out)
	}
	if !strings.Contains(out, "last failed to refresh") {
		t.Errorf("expected a summary line about the failure\n---\n%s", out)
	}
}
