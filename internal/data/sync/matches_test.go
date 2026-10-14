package sync_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/sqlite"
)

// fakeIntl stands in for ESPN's international endpoints and counts every
// request, so request budgets can be asserted.
type fakeIntl struct {
	matches   map[string][]models.Match // "comp/year"
	fail      map[string]error          // "comp/year"
	directory map[string][]models.Team  // gender
	tables    map[string][]models.TeamSeasonStats
	calls     []string
}

func newFakeIntl() *fakeIntl {
	return &fakeIntl{matches: map[string][]models.Match{}, fail: map[string]error{},
		directory: map[string][]models.Team{}, tables: map[string][]models.TeamSeasonStats{}}
}

// nations of the fake world, by ESPN ID.
var fakeNations = map[string]string{"1": "Arcadia", "2": "Borduria", "3": "Carpania", "4": "Dravonia"}

func (f *fakeIntl) teamsOf(ms []models.Match, gender string) []models.Team {
	seen := map[string]bool{}
	var out []models.Team
	for _, m := range ms {
		for _, id := range []string{m.HomeTeamID, m.AwayTeamID} {
			if !seen[id] {
				seen[id] = true
				out = append(out, models.Team{ID: id, Name: fakeNations[strings.TrimPrefix(id, "espn:")], Gender: gender, National: true})
			}
		}
	}
	return out
}

func (f *fakeIntl) CompetitionMatches(_ context.Context, comp provider.CompetitionRef, year int) ([]models.Match, []models.Team, error) {
	key := fmt.Sprintf("%s/%d", comp.CompetitionID, year)
	f.calls = append(f.calls, "matches "+key)
	if err := f.fail[key]; err != nil && !errors.Is(err, provider.ErrIncomplete) {
		return nil, nil, err
	}
	c, _ := sync.FindCompetition(comp.CompetitionID)
	ms := append([]models.Match(nil), f.matches[key]...)
	return ms, f.teamsOf(ms, c.Gender), f.fail[key]
}

func (f *fakeIntl) NationalTeams(_ context.Context, gender string) ([]models.Team, error) {
	f.calls = append(f.calls, "directory "+gender)
	return f.directory[gender], nil
}

func (f *fakeIntl) Standings(_ context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.TeamSeasonStats, error) {
	f.calls = append(f.calls, "standings "+season.SeasonID)
	rows, ok := f.tables[season.SeasonID]
	if !ok {
		return nil, fmt.Errorf("fake: %w", provider.ErrNoTable)
	}
	return rows, nil
}

func (f *fakeIntl) Teams(_ context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Team, error) {
	var out []models.Team
	for _, r := range f.tables[season.SeasonID] {
		out = append(out, models.Team{ID: r.TeamID, Name: fakeNations[strings.TrimPrefix(r.TeamID, "espn:")], Gender: "men", National: true})
	}
	return out, nil
}

func (f *fakeIntl) count(prefix string) int {
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

var matchSeq int

// fixture is a finished or scheduled match between two fake nations.
func fixture(comp string, edition int, kickoff time.Time, home, away string, score ...int) models.Match {
	matchSeq++
	m := models.Match{ID: fmt.Sprintf("espn:%d", 5000+matchSeq), CompetitionID: comp,
		SeasonID: fmt.Sprintf("%s:%d", comp, edition), Kickoff: kickoff, Status: models.MatchScheduled,
		HomeTeamID: "espn:" + home, AwayTeamID: "espn:" + away, Source: "espn", UpdatedAt: kickoff}
	if len(score) == 2 {
		m.Status = models.MatchFullTime
		m.HomeScore, m.AwayScore = &score[0], &score[1]
	}
	return m
}

func at(y int, mo time.Month, d, h int) time.Time { return time.Date(y, mo, d, h, 0, 0, 0, time.UTC) }

func newMatchSyncer(t *testing.T, src sync.MatchSource, now *time.Time) (*sync.MatchSyncer, storage.Repositories) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := sqlite.NewStore(db)
	s := sync.NewMatchSyncer(src, store).WithProviderName("espn")
	s.Now = func() time.Time { return *now }
	s.StaleAfter = 6 * time.Hour
	return s, store
}

// A small world: friendlies and qualifiers both years, a World Cup in
// 2026, every nation of the four playing.
func seedWorld(f *fakeIntl) {
	f.matches["int-friendly/2025"] = []models.Match{
		fixture("int-friendly", 2025, at(2025, 3, 21, 19), "1", "2", 2, 0),
		fixture("int-friendly", 2025, at(2025, 6, 6, 19), "3", "4", 1, 1),
	}
	f.matches["wcq-uefa/2025"] = []models.Match{
		fixture("wcq-uefa", 2025, at(2025, 9, 5, 19), "1", "3", 1, 0),
		fixture("wcq-uefa", 2025, at(2025, 10, 9, 19), "2", "4", 0, 3),
	}
	f.matches["world-cup/2026"] = []models.Match{
		fixture("world-cup", 2026, at(2026, 6, 14, 19), "1", "4", 2, 2),
		fixture("world-cup", 2026, at(2026, 6, 15, 19), "2", "3", 0, 1),
	}
	f.matches["int-friendly/2026"] = []models.Match{
		fixture("int-friendly", 2026, at(2026, 3, 27, 19), "1", "3", 0, 1),
	}
}

func matchesOf(t *testing.T, repos storage.Repositories, team string) []models.Match {
	t.Helper()
	ms, err := repos.MatchesForTeam(context.Background(), team, models.MatchFilter{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

// The success criterion: following a nation costs about thirty requests
// for two years, and following a second one afterwards costs none.
func TestFollowingTwoNationsStaysWithinBudget(t *testing.T) {
	src := newFakeIntl()
	seedWorld(src)
	now := at(2026, 10, 13, 12)
	s, repos := newMatchSyncer(t, src, &now)

	years := sync.RecentYears(now, 2)
	results := s.SyncNations(context.Background(), "men", years, nil)
	first := len(src.calls)
	if first > 30 {
		t.Errorf("first nation cost %d requests, want at most 30", first)
	}
	if first != len(sync.NationUnits("men", years)) || first != len(results) {
		t.Errorf("%d requests for %d units", first, len(results))
	}
	if sum := sync.Summarize(results); sum.Failed != 0 || sum.Totals.Matches != 7 {
		t.Errorf("summary %+v", sum)
	}
	if got := len(matchesOf(t, repos, "nat-arcadia")); got != 4 {
		t.Errorf("Arcadia has %d matches, want 4", got)
	}

	// A second nation, moments later: every unit is shared and fresh.
	now = now.Add(time.Minute)
	results = s.SyncNations(context.Background(), "men", years, nil)
	if extra := len(src.calls) - first; extra != 0 {
		t.Errorf("second nation cost %d requests, want 0", extra)
	}
	if sum := sync.Summarize(results); sum.Fresh != len(results) {
		t.Errorf("summary %+v, want every unit fresh", sum)
	}
	if got := len(matchesOf(t, repos, "nat-dravonia")); got != 3 {
		t.Errorf("Dravonia has %d matches, want 3", got)
	}
}

// A year that has ended, fetched after it ended, is never fetched again —
// not after a week, not with stale_after off.
func TestAFinishedYearIsNeverFetchedAgain(t *testing.T) {
	src := newFakeIntl()
	seedWorld(src)
	now := at(2026, 2, 1, 12)
	s, _ := newMatchSyncer(t, src, &now)
	s.SyncNations(context.Background(), "men", []int{2025}, nil)
	before := len(src.calls)

	for _, later := range []time.Duration{time.Hour, 30 * 24 * time.Hour, 400 * 24 * time.Hour} {
		now = at(2026, 2, 1, 12).Add(later)
		s.StaleAfter = 0
		s.SyncNations(context.Background(), "men", []int{2025}, nil)
	}
	if n := len(src.calls) - before; n != 0 {
		t.Errorf("%d requests re-fetching a finished year", n)
	}
}

// A year fetched while it was still going is fetched once more after it
// ends, to pick up its last results — then never again.
func TestAYearFetchedMidwayIsCompletedOnceItEnds(t *testing.T) {
	src := newFakeIntl()
	seedWorld(src)
	now := at(2025, 12, 15, 12)
	s, _ := newMatchSyncer(t, src, &now)
	units := len(sync.NationUnits("men", []int{2025}))
	s.SyncNations(context.Background(), "men", []int{2025}, nil)

	now = at(2026, 1, 3, 12)
	s.SyncNations(context.Background(), "men", []int{2025}, nil)
	now = at(2026, 3, 3, 12)
	s.SyncNations(context.Background(), "men", []int{2025}, nil)
	if got := len(src.calls); got != 2*units {
		t.Errorf("%d requests, want %d: the year once midway, once after it ended", got, 2*units)
	}
}

// The current year is fetched again only when it's stale and a stored
// match has kicked off since — or once a week, for new fixtures.
func TestTheCurrentYearIsRefetchedOnlyWhenMatchesHaveBeenPlayed(t *testing.T) {
	src := newFakeIntl()
	start := at(2026, 11, 10, 12)
	src.matches["int-friendly/2026"] = []models.Match{
		fixture("int-friendly", 2026, at(2026, 3, 27, 19), "1", "3", 0, 1),
		fixture("int-friendly", 2026, start.Add(20*time.Hour), "1", "2"), // scheduled
	}
	now := start
	s, _ := newMatchSyncer(t, src, &now)
	unit := []sync.Unit{{Competition: mustComp(t, "int-friendly"), Year: 2026}}
	s.SyncUnits(context.Background(), unit, nil)
	// From here on ESPN has the scheduled match's result; footy only
	// learns it by fetching again.
	played := &src.matches["int-friendly/2026"][1]
	played.Status = models.MatchFullTime
	h, a := 1, 1
	played.HomeScore, played.AwayScore = &h, &a

	for _, tc := range []struct {
		after time.Duration
		want  int
		why   string
	}{
		{4 * time.Hour, 1, "younger than stale_after"},
		{10 * time.Hour, 1, "stale, but nothing has kicked off"},
		{22 * time.Hour, 2, "stale, and the scheduled match has kicked off"},
		{30 * time.Hour, 2, "fetched 8 hours ago; nothing scheduled since"},
		{22*time.Hour + 8*24*time.Hour, 3, "a week since the last fetch"},
	} {
		now = start.Add(tc.after)
		s.SyncUnits(context.Background(), unit, nil)
		if got := src.count("matches"); got != tc.want {
			t.Errorf("after %s (%s): %d requests in total, want %d", tc.after, tc.why, got, tc.want)
		}
	}
}

func mustComp(t *testing.T, id string) models.Competition {
	t.Helper()
	c, ok := sync.FindCompetition(id)
	if !ok {
		t.Fatalf("%s not covered", id)
	}
	return c
}

// Rate limiting stops everything at once and is logged as such; other
// failures cost only their own unit.
func TestRateLimitingStopsTheQueueAndOtherFailuresDont(t *testing.T) {
	src := newFakeIntl()
	seedWorld(src)
	units := []sync.Unit{
		{Competition: mustComp(t, "int-friendly"), Year: 2025},
		{Competition: mustComp(t, "wcq-uefa"), Year: 2025},
		{Competition: mustComp(t, "world-cup"), Year: 2026},
		{Competition: mustComp(t, "int-friendly"), Year: 2026},
	}
	src.fail["int-friendly/2025"] = errors.New("connection reset")
	src.fail["world-cup/2026"] = fmt.Errorf("espn: %w", provider.ErrRateLimited)
	now := at(2026, 10, 13, 12)
	s, repos := newMatchSyncer(t, src, &now)

	results := s.SyncUnits(context.Background(), units, nil)
	if len(src.calls) != 3 {
		t.Errorf("%d requests, want 3: the failure doesn't stop the run, the rate limit does", len(src.calls))
	}
	sum := sync.Summarize(results)
	if sum.Succeeded != 1 || sum.Failed != 2 || sum.Skipped != 1 {
		t.Errorf("summary %+v", sum)
	}
	if got := len(matchesOf(t, repos, "nat-borduria")); got != 1 {
		t.Errorf("the unit that worked should have landed: %d matches", got)
	}
	run, err := repos.LatestYearRun(context.Background(), "world-cup", 2026)
	if err != nil || run.Status != models.SyncStatusRateLimited {
		t.Errorf("rate-limited unit logged as %+v, %v", run, err)
	}
	failed, _ := repos.LatestYearRun(context.Background(), "int-friendly", 2025)
	if failed.Succeeded() || failed.Error == "" {
		t.Errorf("failed unit logged as %+v", failed)
	}

	// The failure is retried, but not straight away.
	delete(src.fail, "int-friendly/2025")
	now = now.Add(time.Minute)
	s.SyncUnits(context.Background(), units[:1], nil)
	now = now.Add(15 * time.Minute)
	s.SyncUnits(context.Background(), units[:1], nil)
	if got := src.count("matches int-friendly/2025"); got != 2 {
		t.Errorf("failed unit fetched %d times, want once more after the backoff", got)
	}
}

// Fetching the same year again updates matches in place by ID: a
// rescheduled fixture moves, a played one gets its score, nothing is
// duplicated.
func TestRefetchingUpdatesMatchesInPlace(t *testing.T) {
	src := newFakeIntl()
	m := fixture("int-friendly", 2026, at(2026, 11, 14, 19), "1", "2")
	src.matches["int-friendly/2026"] = []models.Match{m}
	now := at(2026, 11, 1, 12)
	s, repos := newMatchSyncer(t, src, &now)
	unit := []sync.Unit{{Competition: mustComp(t, "int-friendly"), Year: 2026}}
	s.SyncUnits(context.Background(), unit, nil)

	moved := m
	moved.Kickoff = at(2026, 11, 15, 17)
	moved.Status = models.MatchFullTime
	two, one := 2, 1
	moved.HomeScore, moved.AwayScore = &two, &one
	src.matches["int-friendly/2026"] = []models.Match{moved}
	now = at(2026, 11, 16, 12)
	s.SyncUnits(context.Background(), unit, nil)

	got := matchesOf(t, repos, "nat-arcadia")
	if len(got) != 1 {
		t.Fatalf("%d matches, want the one, updated", len(got))
	}
	if !got[0].Kickoff.Equal(moved.Kickoff) || got[0].Status != models.MatchFullTime || *got[0].HomeScore != 2 {
		t.Errorf("match %+v", got[0])
	}
	if got[0].ID != "espn-"+strings.TrimPrefix(m.ID, "espn:") {
		t.Errorf("canonical match ID %q", got[0].ID)
	}
}

func TestAnIncompleteYearIsStoredButNotTakenAsComplete(t *testing.T) {
	src := newFakeIntl()
	seedWorld(src)
	src.fail["int-friendly/2025"] = fmt.Errorf("espn: %w", provider.ErrIncomplete)
	now := at(2026, 2, 1, 12)
	s, repos := newMatchSyncer(t, src, &now)
	results := s.SyncUnits(context.Background(), []sync.Unit{{Competition: mustComp(t, "int-friendly"), Year: 2025}}, nil)
	if !results[0].Failed() || results[0].Result.Matches != 2 {
		t.Errorf("result %+v: the matches land, the unit still counts as failed", results[0])
	}
	if len(matchesOf(t, repos, "nat-arcadia")) != 1 {
		t.Error("the fetched matches should be stored")
	}
	now = now.Add(time.Hour)
	s.SyncUnits(context.Background(), []sync.Unit{{Competition: mustComp(t, "int-friendly"), Year: 2025}}, nil)
	if src.count("matches") != 2 {
		t.Error("an incomplete year must be fetched again")
	}
}

// Nations get "nat-" IDs that a directory seeded first and the match lists
// agree on, the women's side is separate, and a club with a nation's name
// is never merged with it.
func TestNationsResolveToOneIdentity(t *testing.T) {
	src := newFakeIntl()
	seedWorld(src)
	src.directory["men"] = []models.Team{{ID: "espn:1", Name: "Arcadia", Gender: "men"}, {ID: "espn:9", Name: "Elbonia", Gender: "men"}}
	src.directory["women"] = []models.Team{{ID: "espn:101", Name: "Arcadia", Gender: "women"}}
	now := at(2026, 10, 13, 12)
	s, repos := newMatchSyncer(t, src, &now)
	ctx := context.Background()
	if err := repos.UpsertTeam(ctx, models.Team{ID: "eng-arcadia", Name: "Arcadia", Country: "England", Gender: "men"}); err != nil {
		t.Fatal(err)
	}

	for _, g := range []string{"men", "women"} {
		if _, err := s.SeedNationalTeams(ctx, g); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := repos.NationalTeams(ctx, "")
	ids := map[string]bool{}
	for _, n := range all {
		ids[n.ID] = true
	}
	for _, want := range []string{"nat-arcadia", "nat-elbonia", "nat-arcadia-women"} {
		if !ids[want] {
			t.Errorf("missing %s in %v", want, ids)
		}
	}

	s.SyncNations(ctx, "men", []int{2025}, nil)
	if got := len(matchesOf(t, repos, "nat-arcadia")); got != 2 {
		t.Errorf("Arcadia's 2025 matches: %d, want 2 under the directory's ID", got)
	}
	if got := len(matchesOf(t, repos, "eng-arcadia")); got != 0 {
		t.Error("a club named Arcadia must not get the nation's matches")
	}
	if club, _ := repos.GetTeam(ctx, "eng-arcadia"); club.National {
		t.Error("the club must stay a club")
	}
	men, _ := repos.NationalTeams(ctx, "men")
	if len(men) != 5 {
		t.Errorf("%d men's nations, want the directory's two and three more from matches", len(men))
	}
}

// A tournament edition costs one match request and one standings request
// when its named year holds the final. One played over New Year needs
// the next year too.
func TestSyncEditionFetchesTheNextYearOnlyWithoutAFinal(t *testing.T) {
	src := newFakeIntl()
	final := fixture("euro", 2024, at(2024, 7, 14, 19), "1", "2", 2, 1)
	final.Round = "Final"
	src.matches["euro/2024"] = []models.Match{fixture("euro", 2024, at(2024, 6, 14, 19), "1", "3", 1, 0), final}
	p1, p2 := 1, 2
	src.tables["euro:2024"] = []models.TeamSeasonStats{
		{TeamID: "espn:1", CompetitionID: "euro", SeasonID: "euro:2024", GroupLabel: "Group A", Position: &p1, Points: 3, Source: "espn"},
		{TeamID: "espn:3", CompetitionID: "euro", SeasonID: "euro:2024", GroupLabel: "Group A", Position: &p2, Source: "espn"},
	}
	src.matches["afcon/2025"] = []models.Match{fixture("afcon", 2025, at(2025, 12, 22, 19), "3", "4", 1, 1)}
	src.matches["afcon/2026"] = []models.Match{fixture("afcon", 2025, at(2026, 1, 18, 19), "3", "4", 0, 0)}
	now := at(2026, 10, 13, 12)
	s, repos := newMatchSyncer(t, src, &now)
	ctx := context.Background()

	results := s.SyncEdition(ctx, mustComp(t, "euro"), "2024", nil)
	if strings.Join(src.calls, "; ") != "matches euro/2024; standings euro:2024" {
		t.Errorf("calls %v", src.calls)
	}
	if sum := sync.Summarize(results); sum.Failed != 0 || sum.Succeeded != 2 {
		t.Errorf("summary %+v", sum)
	}
	table, _ := repos.Standings(ctx, "euro", "euro:2024")
	if len(table) != 2 || table[0].TeamID != "nat-arcadia" || table[0].GroupLabel != "Group A" {
		t.Errorf("table %+v", table)
	}

	src.calls = nil
	s.SyncEdition(ctx, mustComp(t, "afcon"), "2025", nil)
	if strings.Join(src.calls, "; ") != "matches afcon/2025; matches afcon/2026; standings afcon:2025" {
		t.Errorf("calls %v", src.calls)
	}
	ms, _ := repos.MatchesForCompetitionSeason(ctx, "afcon", "afcon:2025")
	if len(ms) != 2 {
		t.Errorf("AFCON 2025 has %d matches, want both years'", len(ms))
	}

	// Again, with everything finished and fetched: nothing.
	src.calls = nil
	s.SyncEdition(ctx, mustComp(t, "euro"), "2024", nil)
	s.SyncEdition(ctx, mustComp(t, "afcon"), "2025", nil)
	if len(src.calls) != 0 {
		t.Errorf("re-syncing finished editions made requests: %v", src.calls)
	}
}

// A year of a qualifier brings in the tables of whichever editions were
// played in it.
func TestSyncEditionOfAQualifierTablesEveryEditionPlayed(t *testing.T) {
	src := newFakeIntl()
	src.matches["wcq-uefa/2025"] = []models.Match{
		fixture("wcq-uefa", 2025, at(2025, 3, 21, 19), "1", "2", 1, 0),
		fixture("wcq-uefa", 2024, at(2025, 3, 24, 19), "3", "4", 1, 0),
	}
	now := at(2026, 10, 13, 12)
	s, _ := newMatchSyncer(t, src, &now)
	s.SyncEdition(context.Background(), mustComp(t, "wcq-uefa"), "2025", nil)
	if src.count("standings") != 2 {
		t.Errorf("calls %v, want both editions' tables", src.calls)
	}
}

func TestNationUnitsSkipYearsATournamentCantBePlayedIn(t *testing.T) {
	units := sync.NationUnits("men", []int{2025})
	var ids []string
	for _, u := range units {
		ids = append(ids, u.Competition.ID)
	}
	got := strings.Join(ids, " ")
	if strings.Contains(got, "world-cup") || strings.Contains(got, "asian-cup") {
		t.Errorf("2025 units %s: no World Cup or Asian Cup was (or spilled into) 2025", got)
	}
	for _, want := range []string{"euro", "gold-cup", "afcon", "int-friendly", "wcq-uefa", "nations-league"} {
		if !strings.Contains(" "+got+" ", " "+want+" ") {
			t.Errorf("2025 units %s: missing %s", got, want)
		}
	}
	if w := sync.NationUnits("women", []int{2025}); len(w) != 3 {
		t.Errorf("%d women's units for 2025", len(w))
	}
}
