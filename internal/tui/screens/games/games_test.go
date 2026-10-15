package games_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/games"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

var club = models.Team{ID: "eng-wessex", Name: "Wessex", Country: "England", Gender: "men"}

func ip(n int) *int         { return &n }
func fp(f float64) *float64 { return &f }

// seed stores 32 finished games for the club, alternating home and away
// against two fictional opponents, newest on 2026-03-31; the newest has
// ESPN's box score, the one before understat's xG.
func seed(t *testing.T) storage.Repositories {
	t.Helper()
	ctx := context.Background()
	repos := storagetest.New(t)
	comp := models.Competition{ID: "eng-championship", Name: "Championship", ShortName: "CH", Country: "England", Tier: 2, Gender: "men"}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(repos.UpsertCompetition(ctx, comp))
	must(repos.UpsertSeason(ctx, models.Season{ID: comp.ID + ":2025", CompetitionID: comp.ID, Label: "2025"}))
	for _, tm := range []models.Team{club, {ID: "eng-fenmoor", Name: "Fenmoor", Country: "England", Gender: "men"},
		{ID: "eng-calder", Name: "Calder", Country: "England", Gender: "men"}} {
		must(repos.UpsertTeam(ctx, tm))
	}
	var ms []models.Match
	for i := range 32 {
		opp := "eng-fenmoor"
		if i%2 == 1 {
			opp = "eng-calder"
		}
		m := models.Match{ID: fmt.Sprintf("g%02d", i), CompetitionID: comp.ID, SeasonID: comp.ID + ":2025",
			Kickoff: time.Date(2026, 3, 31, 15, 0, 0, 0, time.UTC).AddDate(0, 0, -i), Status: models.MatchFullTime,
			HomeTeamID: club.ID, AwayTeamID: opp, HomeScore: ip(i % 3), AwayScore: ip(1), Source: "test", UpdatedAt: time.Now()}
		if i%2 == 1 {
			m.HomeTeamID, m.AwayTeamID = opp, club.ID
		}
		ms = append(ms, m)
	}
	ms[0].HomeStats = &models.MatchTeamStats{Possession: fp(57), Shots: ip(15), ShotsOnTarget: ip(6), Corners: ip(7), Fouls: ip(9)}
	ms[0].AwayStats = &models.MatchTeamStats{Possession: fp(43), Shots: ip(8), ShotsOnTarget: ip(2), Corners: ip(3), Fouls: ip(12)}
	ms[1].HomeStats = &models.MatchTeamStats{XG: fp(1.42)}
	ms[1].AwayStats = &models.MatchTeamStats{XG: fp(0.37)}
	must(repos.UpsertMatches(ctx, ms))
	return repos
}

func open(t *testing.T, repos storage.Repositories) (core.Screen, *core.Deps) {
	t.Helper()
	deps := &core.Deps{Ctx: context.Background(), Repos: repos, Theme: styles.Plain()}
	s := tuitest.Settle(t, games.New(*deps, club))
	return tuitest.Resize(t, s, 110, 60), deps
}

func TestTheGamesScreenListsTheLastThirty(t *testing.T) {
	s, _ := open(t, seed(t))
	got := s.(*games.Model).Games()
	if len(got) != games.Limit || got[0].ID != "g00" || got[29].ID != "g29" {
		t.Fatalf("%d games, first %s", len(got), got[0].ID)
	}
	out := s.View()
	for _, want := range []string{"Wessex", "30 games", "Fenmoor", "Calder", "2026-03-31", "xG 0.4–1.4"} {
		if !strings.Contains(out, want) {
			t.Errorf("view lacks %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "2026-03-01") {
		t.Error("the 31st game is listed")
	}
}

func TestEnterOpensTheBoxScore(t *testing.T) {
	s, _ := open(t, seed(t))
	s = tuitest.Key(t, s, "enter")
	if s.(*games.Model).Expanded() != "g00" {
		t.Fatal("not expanded")
	}
	out := s.View()
	for _, want := range []string{"Possession", "57% – 43%", "Shots", "15 – 8", "Fouls", "9 – 12"} {
		if !strings.Contains(out, want) {
			t.Errorf("box score lacks %q\n---\n%s", want, out)
		}
	}
	// understat's match has only xG: its box score is just that line.
	s = tuitest.Key(t, s, "down")
	s = tuitest.Key(t, s, "enter")
	out = s.View()
	if !strings.Contains(out, "0.37 – 1.42") || strings.Contains(out, "Possession") {
		t.Errorf("xG-only box score, from the club's side\n---\n%s", out)
	}
	s = tuitest.Key(t, s, "enter")
	if s.(*games.Model).Expanded() != "" {
		t.Error("enter again should close it")
	}
}

func TestOOpensTheOpponent(t *testing.T) {
	repos := seed(t)
	deps := core.Deps{Ctx: context.Background(), Repos: repos, Theme: styles.Plain()}
	var opened models.Team
	deps.Nav.Team = func(_ core.Deps, tm models.Team, _, _ string) core.Screen { opened = tm; return nil }
	s := tuitest.Resize(t, tuitest.Settle(t, games.New(deps, club)), 110, 60)
	s = tuitest.Key(t, s, "down")
	_, cmd := s.Update(keyO())
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok || opened.ID != "eng-calder" || opened.Name != "Calder" {
		t.Errorf("opened %+v", opened)
	}
}

func keyO() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")} }

func TestNoGamesSaysHowToFetchThem(t *testing.T) {
	s, _ := open(t, storagetest.New(t))
	if out := s.View(); !strings.Contains(out, "No games cached") {
		t.Errorf("view\n---\n%s", out)
	}
	if req, ok := s.(core.RefreshRequester).RefreshRequest(); !ok || req.TeamID != club.ID {
		t.Errorf("refresh %+v", req)
	}
}
