package sync

import (
	"context"
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/sqlite"
)

var bg = context.Background()

// nativeSource plays back one competition-season in a provider's own IDs.
type nativeSource struct {
	teams   []models.Team
	rows    []models.TeamSeasonStats
	players []models.Player
	lines   []models.PlayerSeasonStats
	colors  []models.TeamColors
}

func (n *nativeSource) Teams(context.Context, provider.CompetitionRef, provider.SeasonRef) ([]models.Team, error) {
	return append([]models.Team(nil), n.teams...), nil
}
func (n *nativeSource) Standings(_ context.Context, c provider.CompetitionRef, s provider.SeasonRef) ([]models.TeamSeasonStats, error) {
	out := append([]models.TeamSeasonStats(nil), n.rows...)
	for i := range out {
		out[i].CompetitionID, out[i].SeasonID = c.CompetitionID, s.SeasonID
	}
	return out, nil
}
func (n *nativeSource) Players(context.Context, provider.CompetitionRef, provider.SeasonRef) ([]models.Player, error) {
	return append([]models.Player(nil), n.players...), nil
}
func (n *nativeSource) AllRosterStats(_ context.Context, c provider.CompetitionRef, s provider.SeasonRef) ([]models.PlayerSeasonStats, error) {
	out := append([]models.PlayerSeasonStats(nil), n.lines...)
	for i := range out {
		out[i].CompetitionID, out[i].SeasonID = c.CompetitionID, s.SeasonID
	}
	return out, nil
}
func (n *nativeSource) TeamColors(context.Context, provider.CompetitionRef, provider.SeasonRef) ([]models.TeamColors, error) {
	return n.colors, nil
}

// club adds a team to the fetched table, in the competition's gender.
func (n *nativeSource) club(espnID, name, gender string) {
	id := "espn:" + espnID
	n.teams = append(n.teams, models.Team{ID: id, Name: name, Gender: gender})
	n.rows = append(n.rows, models.TeamSeasonStats{TeamID: id, Points: 10, Source: "espn"})
}

func (n *nativeSource) player(espnID, name, espnTeam string) {
	id := "espn:" + espnID
	n.players = append(n.players, models.Player{ID: id, Name: name, Gender: "men"})
	n.lines = append(n.lines, models.PlayerSeasonStats{PlayerID: id, TeamID: "espn:" + espnTeam, Appearances: 1, Source: "espn"})
}

func newStore(t *testing.T) storage.Repositories {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return sqlite.NewStore(db)
}

// seedClub caches a club as understat would have, with one player.
func seedClub(t *testing.T, repos storage.Repositories, compID, country, teamID, name string, players ...[2]string) {
	t.Helper()
	comp := models.Competition{ID: compID, Name: compID, Country: country, Tier: 1, Gender: "men"}
	season := NewSeason(compID, "2025")
	must(t, repos.UpsertCompetition(bg, comp))
	must(t, repos.UpsertSeason(bg, season))
	must(t, repos.UpsertTeam(bg, models.Team{ID: teamID, Name: name, Country: country, Gender: "men"}))
	must(t, repos.LinkTeamCompetitionSeason(bg, teamID, compID, season.ID))
	for _, p := range players {
		must(t, repos.UpsertPlayer(bg, models.Player{ID: p[0], Name: p[1], Gender: "men"}))
		must(t, repos.UpsertPlayerSeasonStats(bg, models.PlayerSeasonStats{
			PlayerID: p[0], TeamID: teamID, CompetitionID: compID, SeasonID: season.ID, Source: "understat"}))
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func syncOne(t *testing.T, repos storage.Repositories, src Source, compID string) {
	t.Helper()
	comp, ok := FindCompetition(compID)
	if !ok {
		t.Fatalf("%s not covered", compID)
	}
	s := New(src, repos).WithProviderName("espn")
	if _, err := s.SyncCompetitionSeason(bg, comp, NewSeason(compID, "2025")); err != nil {
		t.Fatalf("sync %s: %v", compID, err)
	}
}

// teamsIn returns the canonical team IDs in a competition-season's table.
func teamsIn(t *testing.T, repos storage.Repositories, compID string) map[string]bool {
	t.Helper()
	rows, err := repos.Standings(bg, compID, NewSeason(compID, "2025").ID)
	must(t, err)
	out := map[string]bool{}
	for _, r := range rows {
		out[r.TeamID] = true
	}
	return out
}

func TestEuropeanClubsJoinTheClubsAlreadyCached(t *testing.T) {
	repos := newStore(t)
	seedClub(t, repos, "eng-premier-league", "England", "eng-arsenal", "Arsenal", [2]string{"understat-1", "Bukayo Saka"})
	seedClub(t, repos, "fra-ligue-1", "France", "fra-paris-saint-germain", "Paris Saint Germain")
	seedClub(t, repos, "ita-serie-a", "Italy", "ita-inter", "Inter")
	seedClub(t, repos, "ita-serie-a", "Italy", "ita-roma", "Roma")

	src := &nativeSource{}
	src.club("359", "Arsenal", "men")
	src.club("160", "Paris Saint-Germain", "men")
	src.club("110", "Internazionale", "men")
	src.club("104", "AS Roma", "men")
	src.club("570", "Club Brugge", "men")
	src.player("9001", "Bukayo Saka", "359")
	syncOne(t, repos, src, "eur-champions-league")

	got := teamsIn(t, repos, "eur-champions-league")
	for _, want := range []string{"eng-arsenal", "fra-paris-saint-germain", "ita-inter", "ita-roma", "int-club-brugge"} {
		if !got[want] {
			t.Errorf("table missing %s; has %v", want, got)
		}
	}

	// Saka's Champions League line belongs to the same player as his
	// Premier League one.
	history, err := repos.PlayerHistory(bg, "understat-1")
	must(t, err)
	if len(history) != 2 {
		t.Errorf("Saka has %d season lines, want league + Champions League on one player", len(history))
	}

	// The cached club keeps understat's name; ESPN's spelling doesn't
	// overwrite it.
	psg, _ := repos.GetTeam(bg, "fra-paris-saint-germain")
	if psg.Name != "Paris Saint Germain" || psg.Country != "France" {
		t.Errorf("cached club rewritten: %+v", psg)
	}
}

func TestResolutionIsRecordedAndStableAcrossRefreshes(t *testing.T) {
	repos := newStore(t)
	seedClub(t, repos, "eng-premier-league", "England", "eng-arsenal", "Arsenal")
	src := &nativeSource{}
	src.club("359", "Arsenal", "men")
	syncOne(t, repos, src, "eur-champions-league")

	id, err := repos.ExternalRef(bg, "team", "espn", "359")
	if err != nil || id != "eng-arsenal" {
		t.Fatalf("mapping recorded as %q, %v", id, err)
	}
	// ESPN renaming the club later doesn't make it a new one.
	src.teams[0].Name = "Arsenal FC London"
	syncOne(t, repos, src, "eur-champions-league")
	if got := teamsIn(t, repos, "eur-champions-league"); len(got) != 1 || !got["eng-arsenal"] {
		t.Errorf("after a rename the table has %v", got)
	}
}

// Across Europe only exact names count: with PSG not cached, Paris FC
// must not absorb it.
func TestEuropeanMatchingNeverGuessesAcrossClubs(t *testing.T) {
	repos := newStore(t)
	seedClub(t, repos, "fra-ligue-1", "France", "fra-paris-fc", "Paris FC")
	seedClub(t, repos, "rus-premier-league", "Russia", "rus-dinamo-moscow", "Dinamo Moscow")

	src := &nativeSource{}
	src.club("160", "Paris Saint-Germain", "men")
	src.club("597", "Dinamo Zagreb", "men")
	syncOne(t, repos, src, "eur-europa-league")

	got := teamsIn(t, repos, "eur-europa-league")
	if got["fra-paris-fc"] || got["rus-dinamo-moscow"] {
		t.Errorf("a different club was matched: %v", got)
	}
	if !got["int-paris-saint-germain"] || !got["int-dinamo-zagreb"] {
		t.Errorf("unmatched clubs should be new: %v", got)
	}
}

func TestDomesticClubsFollowedBetweenDivisions(t *testing.T) {
	repos := newStore(t)
	seedClub(t, repos, "eng-premier-league", "England", "eng-coventry", "Coventry")
	seedClub(t, repos, "eng-premier-league", "England", "eng-sheffield-united", "Sheffield United")

	src := &nativeSource{}
	src.club("1", "Coventry City", "men")
	src.club("2", "Sheffield Wednesday", "men")
	syncOne(t, repos, src, "eng-championship")

	got := teamsIn(t, repos, "eng-championship")
	if !got["eng-coventry"] {
		t.Errorf("Coventry City should be the cached Coventry: %v", got)
	}
	if got["eng-sheffield-united"] || !got["eng-sheffield-wednesday"] {
		t.Errorf("Sheffield Wednesday must be its own club: %v", got)
	}
	wed, _ := repos.GetTeam(bg, "eng-sheffield-wednesday")
	if wed.Country != "England" {
		t.Errorf("a new domestic club should get its country: %+v", wed)
	}
}

// A club the provider already knows by another of its IDs is another
// club, however its name reduces: Miami FC is a subset of Inter Miami,
// and Atlanta United 2 loses its digit to become Atlanta United exactly.
// The same name under a second ID is the provider listing one club
// twice, and still joins it.
func TestAClubTheProviderKnowsByAnotherIDIsAnotherClub(t *testing.T) {
	repos := newStore(t)
	mls := &nativeSource{}
	mls.club("20232", "Inter Miami CF", "men")
	mls.club("18418", "Atlanta United FC", "men")
	mls.club("183", "Columbus Crew", "men")
	syncOne(t, repos, mls, "usa-mls")

	usl := &nativeSource{}
	usl.club("18159", "Miami FC", "men")
	usl.club("18989", "Atlanta United 2", "men")
	usl.club("99183", "Columbus Crew", "men")
	syncOne(t, repos, usl, "usa-usl-championship")

	got := teamsIn(t, repos, "usa-usl-championship")
	for _, id := range []string{"usa-inter-miami-cf", "usa-atlanta-united-fc"} {
		if got[id] {
			t.Errorf("%s was given another club's season: %v", id, got)
		}
	}
	for _, id := range []string{"usa-miami-fc", "usa-atlanta-united-2", "usa-columbus-crew"} {
		if !got[id] {
			t.Errorf("want %s in the USL table: %v", id, got)
		}
	}
}

// A refresh replaces the season: a club no longer in it, or one that has
// since been resolved to another canonical club, leaves nothing behind.
func TestARefreshReplacesTheSeason(t *testing.T) {
	repos := newStore(t)
	first := &nativeSource{}
	first.club("1", "Coventry City", "men")
	first.club("2", "Sheffield Wednesday", "men")
	first.player("10", "Haji Wright", "1")
	first.player("20", "Barry Bannan", "2")
	syncOne(t, repos, first, "eng-championship")

	second := &nativeSource{}
	second.club("1", "Coventry City", "men")
	second.player("10", "Haji Wright", "1")
	syncOne(t, repos, second, "eng-championship")

	if got := teamsIn(t, repos, "eng-championship"); len(got) != 1 || !got["eng-coventry-city"] {
		t.Errorf("table after the refresh: %v", got)
	}
	history, err := repos.TeamHistory(bg, "eng-sheffield-wednesday")
	must(t, err)
	if len(history) != 0 {
		t.Errorf("Wednesday keeps %d season lines", len(history))
	}
	lines, err := repos.SeasonPlayerLines(bg, "eng-championship", NewSeason("eng-championship", "2025").ID)
	must(t, err)
	if len(lines) != 1 {
		t.Errorf("%d squad lines, want Wright's alone", len(lines))
	}

	// A fetch with no clubs leaves the season as it was.
	syncOne(t, repos, &nativeSource{}, "eng-championship")
	if got := teamsIn(t, repos, "eng-championship"); len(got) != 1 {
		t.Errorf("an empty fetch emptied the season: %v", got)
	}
}

func TestWomensSidesNeverMergeWithTheMensClub(t *testing.T) {
	repos := newStore(t)
	seedClub(t, repos, "eng-premier-league", "England", "eng-arsenal", "Arsenal")
	src := &nativeSource{}
	src.club("4581", "Arsenal", "women")
	syncOne(t, repos, src, "eng-wsl")

	got := teamsIn(t, repos, "eng-wsl")
	if got["eng-arsenal"] || !got["eng-arsenal-women"] {
		t.Errorf("WSL Arsenal resolved to %v, want its own eng-arsenal-women", got)
	}
}

func TestPlayersMatchOnlyWhenUnambiguous(t *testing.T) {
	repos := newStore(t)
	seedClub(t, repos, "eng-premier-league", "England", "eng-arsenal", "Arsenal",
		[2]string{"understat-10", "Gabriel Jesus"}, [2]string{"understat-11", "Gabriel Magalhães"},
		[2]string{"understat-12", "Martin Ødegaard"})

	src := &nativeSource{}
	src.club("359", "Arsenal", "men")
	src.player("1", "Gabriel", "359")         // two Gabriels: ambiguous
	src.player("2", "Martin Odegaard", "359") // accents differ only
	syncOne(t, repos, src, "eur-champions-league")

	if id, _ := repos.ExternalRef(bg, "player", "espn", "1"); id != "espn-1" {
		t.Errorf("an ambiguous name must not be guessed: resolved to %q", id)
	}
	if id, _ := repos.ExternalRef(bg, "player", "espn", "2"); id != "understat-12" {
		t.Errorf("Martin Odegaard should be the cached Ødegaard, got %q", id)
	}
}

func TestProviderColorsFillGapsButNeverReplace(t *testing.T) {
	repos := newStore(t)
	seedClub(t, repos, "eng-premier-league", "England", "eng-arsenal", "Arsenal")
	must(t, repos.UpsertTeamColors(bg, models.TeamColors{TeamID: "eng-arsenal", PrimaryHex: "#EF0107", Source: "seed:v1"}))

	src := &nativeSource{colors: []models.TeamColors{
		{TeamID: "espn:359", PrimaryHex: "#111111", Source: "espn"},
		{TeamID: "espn:570", PrimaryHex: "#0055A4", Source: "espn"},
		{TeamID: "espn:999", PrimaryHex: "#222222", Source: "espn"}, // not in the table
	}}
	src.club("359", "Arsenal", "men")
	src.club("570", "Club Brugge", "men")
	syncOne(t, repos, src, "eur-champions-league")

	if c, _ := repos.GetTeamColors(bg, "eng-arsenal"); c.PrimaryHex != "#EF0107" {
		t.Errorf("curated colors replaced: %+v", c)
	}
	if c, err := repos.GetTeamColors(bg, "int-club-brugge"); err != nil || c.PrimaryHex != "#0055A4" {
		t.Errorf("new club should take the provider's colors: %+v, %v", c, err)
	}
}

// understatSource plays back a table the way understat reports it.
func understatSource(clubs ...[2]string) *nativeSource {
	src := &nativeSource{}
	for _, c := range clubs {
		id := "understat:" + c[0]
		src.teams = append(src.teams, models.Team{ID: id, Name: c[1], Country: "England", Gender: "men"})
		src.rows = append(src.rows, models.TeamSeasonStats{TeamID: id, Points: 10, Source: "understat"})
	}
	return src
}

func syncUnderstat(t *testing.T, repos storage.Repositories, src Source) {
	t.Helper()
	comp, _ := FindCompetition("eng-premier-league")
	s := New(src, repos).WithProviderName("understat")
	if _, err := s.SyncCompetitionSeason(bg, comp, NewSeason("eng-premier-league", "2025")); err != nil {
		t.Fatal(err)
	}
}

// Download order mustn't matter: a club ESPN cached first (from the
// Championship) is the same club understat later reports in the Premier
// League.
func TestUnderstatJoinsClubsAnotherProviderCachedFirst(t *testing.T) {
	repos := newStore(t)
	src := &nativeSource{}
	src.club("375", "Leicester City", "men")
	syncOne(t, repos, src, "eng-championship")

	syncUnderstat(t, repos, understatSource([2]string{"eng-leicester", "Leicester"}))

	got := teamsIn(t, repos, "eng-premier-league")
	if !got["eng-leicester-city"] || got["eng-leicester"] {
		t.Errorf("Premier League Leicester resolved to %v, want the club ESPN created", got)
	}
}

// For everything understat has always fetched, IDs come out exactly as
// before — existing databases and curated seeds keep lining up.
func TestUnderstatKeepsItsCanonicalIDs(t *testing.T) {
	repos := newStore(t)
	src := understatSource([2]string{"eng-arsenal", "Arsenal"}, [2]string{"eng-ipswich", "Ipswich"})
	src.players = []models.Player{{ID: "understat:7322", Name: "Bukayo Saka", Gender: "men"}}
	src.lines = []models.PlayerSeasonStats{{PlayerID: "understat:7322", TeamID: "understat:eng-arsenal", Source: "understat"}}
	syncUnderstat(t, repos, src)
	syncUnderstat(t, repos, src) // and again, over its own data

	got := teamsIn(t, repos, "eng-premier-league")
	if len(got) != 2 || !got["eng-arsenal"] || !got["eng-ipswich"] {
		t.Errorf("teams %v, want understat's canonical IDs unchanged", got)
	}
	if _, err := repos.GetPlayer(bg, "understat-7322"); err != nil {
		t.Errorf("Saka should keep understat-7322: %v", err)
	}
}

func TestNormalizeClub(t *testing.T) {
	for in, want := range map[string]string{
		"Paris Saint-Germain":  "paris saint germain",
		"AS Monaco":            "monaco",
		"Mainz 05":             "mainz",
		"SC Freiburg":          "freiburg",
		"Atlético Madrid":      "atletico madrid",
		"Internazionale":       "inter",
		"Zenit St. Petersburg": "zenit saint petersburg",
	} {
		if got := normalizeClub(in); got != want {
			t.Errorf("normalizeClub(%q) = %q, want %q", in, got, want)
		}
	}
}
