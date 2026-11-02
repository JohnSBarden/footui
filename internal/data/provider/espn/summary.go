package espn

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
)

// A match summary is one large response (around 440 KB), so it is fetched
// for a club's last game only; just the parts below are kept.

var _ provider.MatchDetailsProvider = (*Provider)(nil)

// --- wire types ---

type summaryResponse struct {
	Header struct {
		Competitions []struct {
			Status struct {
				Type struct {
					Completed bool `json:"completed"`
				} `json:"type"`
			} `json:"status"`
			Competitors []struct {
				HomeAway string `json:"homeAway"`
				Team     struct {
					ID string `json:"id"`
				} `json:"team"`
			} `json:"competitors"`
		} `json:"competitions"`
	} `json:"header"`
	GameInfo struct {
		Venue struct {
			FullName string `json:"fullName"`
		} `json:"venue"`
		Attendance int `json:"attendance"`
	} `json:"gameInfo"`
	Boxscore struct {
		Teams []struct {
			HomeAway   string `json:"homeAway"`
			Statistics []struct {
				Name         string `json:"name"`
				DisplayValue string `json:"displayValue"`
			} `json:"statistics"`
		} `json:"teams"`
	} `json:"boxscore"`
	KeyEvents []summaryEvent `json:"keyEvents"`
	Rosters   []struct {
		HomeAway  string          `json:"homeAway"`
		Formation string          `json:"formation"`
		Roster    []summaryPlayer `json:"roster"`
	} `json:"rosters"`
	Commentary []struct {
		Time struct {
			DisplayValue string `json:"displayValue"`
		} `json:"time"`
		Text string `json:"text"`
		Play *struct {
			Type struct {
				Text string `json:"text"`
			} `json:"type"`
		} `json:"play"`
	} `json:"commentary"`
}

type summaryEvent struct {
	Type struct {
		Type string `json:"type"`
	} `json:"type"`
	Text   string `json:"text"`
	Period struct {
		Number int `json:"number"`
	} `json:"period"`
	Clock struct {
		DisplayValue string `json:"displayValue"`
	} `json:"clock"`
	Shootout bool `json:"shootout"`
	Team     *struct {
		ID string `json:"id"`
	} `json:"team"`
	Participants []struct {
		Athlete summaryAthlete `json:"athlete"`
	} `json:"participants"`
}

type summaryAthlete struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type summaryPlayer struct {
	Starter        bool           `json:"starter"`
	SubbedIn       bool           `json:"subbedIn"`
	SubbedOut      bool           `json:"subbedOut"`
	Jersey         string         `json:"jersey"`
	FormationPlace string         `json:"formationPlace"`
	Athlete        summaryAthlete `json:"athlete"`
	Position       struct {
		Abbreviation string `json:"abbreviation"`
	} `json:"position"`
	Stats []struct {
		Abbreviation string  `json:"abbreviation"`
		Value        flexNum `json:"value"`
	} `json:"stats"`
	Plays []struct {
		Clock struct {
			DisplayValue string `json:"displayValue"`
		} `json:"clock"`
		Substitution bool `json:"substitution"`
	} `json:"plays"`
}

// --- fetching ---

// MatchDetail implements provider.MatchDetailsProvider. nativeID is
// ESPN's event ID.
func (p *Provider) MatchDetail(ctx context.Context, ref provider.CompetitionRef, nativeID string) (models.MatchDetail, error) {
	slug, ok := slugFor(ref.CompetitionID)
	if !ok {
		return models.MatchDetail{}, fmt.Errorf("espn: competition %q is not covered", ref.CompetitionID)
	}
	var resp summaryResponse
	path := fmt.Sprintf("/site/v2/sports/soccer/%s/summary?event=%s", slug, nativeID)
	if err := p.client.site(ctx, path, &resp); err != nil {
		return models.MatchDetail{}, fmt.Errorf("espn: match %s %s: %w", slug, nativeID, err)
	}
	if len(resp.Header.Competitions) == 0 {
		return models.MatchDetail{}, fmt.Errorf("espn: match %s %s has no teams: %w", slug, nativeID, provider.ErrNotFound)
	}
	return mapSummary(resp, p.now().UTC()), nil
}

// DayMatches implements provider.MatchDetailsProvider, from the league's
// scoreboard for that day.
func (p *Provider) DayMatches(ctx context.Context, ref provider.CompetitionRef, day time.Time) ([]models.Match, []models.Team, error) {
	slug, ok := slugFor(ref.CompetitionID)
	if !ok {
		return nil, nil, fmt.Errorf("espn: competition %q is not covered", ref.CompetitionID)
	}
	events, err := p.scoreboard(ctx, slug, day.UTC().Format("20060102"))
	if err != nil {
		return nil, nil, err
	}
	matches, teams := mapScoreboard(events, competitionFor(ref), p.now().UTC())
	return matches, teams, nil
}

// --- mapping ---

func mapSummary(r summaryResponse, fetched time.Time) models.MatchDetail {
	comp := r.Header.Competitions[0]
	d := models.MatchDetail{
		Final:      comp.Status.Type.Completed,
		Venue:      r.GameInfo.Venue.FullName,
		Attendance: r.GameInfo.Attendance,
		FetchedAt:  fetched,
	}
	sides := map[string]string{} // ESPN team ID → side
	for _, c := range comp.Competitors {
		sides[c.Team.ID] = c.HomeAway
	}

	for _, t := range r.Boxscore.Teams {
		if s := side(&d, t.HomeAway); s != nil {
			for _, st := range t.Statistics {
				setStat(&s.Stats, st.Name, st.DisplayValue)
			}
		}
	}
	for _, ro := range r.Rosters {
		if s := side(&d, ro.HomeAway); s != nil {
			s.Formation = ro.Formation
			s.Lineup = mapLineup(ro.Roster)
		}
	}
	for _, e := range r.KeyEvents {
		if ev, ok := mapEvent(e, sides); ok {
			d.Events = append(d.Events, ev)
		}
	}
	// ESPN lists them in order, but not always; a stable sort keeps a
	// sub's pairs and same-minute cards as listed.
	slices.SortStableFunc(d.Events, func(a, b models.MatchEvent) int {
		if a.Period != b.Period {
			return a.Period - b.Period
		}
		if a.Minute != b.Minute {
			return a.Minute - b.Minute
		}
		return a.Added - b.Added
	})
	for _, c := range r.Commentary {
		text := strings.TrimSpace(c.Text)
		if text == "" {
			continue
		}
		line := models.CommentaryLine{Clock: tidyClock(c.Time.DisplayValue), Text: text}
		if c.Play != nil {
			line.Kind = c.Play.Type.Text
		}
		d.Commentary = append(d.Commentary, line)
	}
	return d
}

func side(d *models.MatchDetail, homeAway string) *models.MatchSide {
	switch homeAway {
	case "home":
		return &d.Home
	case "away":
		return &d.Away
	}
	return nil
}

// setStat files one boxscore statistic. Percentages ESPN derives (passPct
// and the like) are left out: the counts are kept and the share computed.
func setStat(s *models.MatchDetailStats, name, value string) {
	if name == "possessionPct" {
		if v, err := strconv.ParseFloat(value, 64); err == nil && v >= 0 && v <= 100 {
			s.Possession = &v
		}
		return
	}
	target := statField(s, name)
	if target == nil {
		return
	}
	if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && v >= 0 && v < 2000 {
		*target = &v
	}
}

func statField(s *models.MatchDetailStats, name string) **int {
	switch name {
	case "totalShots":
		return &s.Shots
	case "shotsOnTarget":
		return &s.ShotsOnTarget
	case "blockedShots":
		return &s.BlockedShots
	case "totalPasses":
		return &s.Passes
	case "accuratePasses":
		return &s.AccuratePasses
	case "totalCrosses":
		return &s.Crosses
	case "accurateCrosses":
		return &s.AccurateCrosses
	case "totalLongBalls":
		return &s.LongBalls
	case "accurateLongBalls":
		return &s.AccurateLongBalls
	case "totalTackles":
		return &s.Tackles
	case "effectiveTackles":
		return &s.TacklesWon
	case "interceptions":
		return &s.Interceptions
	case "totalClearance":
		return &s.Clearances
	case "saves":
		return &s.Saves
	case "offsides":
		return &s.Offsides
	case "wonCorners":
		return &s.Corners
	case "foulsCommitted":
		return &s.Fouls
	case "yellowCards":
		return &s.YellowCards
	case "redCards":
		return &s.RedCards
	}
	return nil
}

func mapLineup(roster []summaryPlayer) []models.LineupPlayer {
	out := make([]models.LineupPlayer, 0, len(roster))
	for _, r := range roster {
		p := models.LineupPlayer{
			Name:     r.Athlete.DisplayName,
			PlayerID: playerRef(r.Athlete.ID),
			Shirt:    r.Jersey,
			Position: r.Position.Abbreviation,
			Starter:  r.Starter,
		}
		if r.Starter {
			p.Place, _ = strconv.Atoi(r.FormationPlace)
		}
		for _, pl := range r.Plays {
			if !pl.Substitution {
				continue
			}
			minute, _ := parseClock(pl.Clock.DisplayValue)
			switch {
			case r.SubbedIn && p.On == 0:
				p.On = max(minute, 1)
			case r.SubbedOut:
				p.Off = max(minute, 1)
			}
		}
		for _, st := range r.Stats {
			if !st.Value.Valid {
				continue
			}
			n := int(st.Value.Value)
			switch st.Abbreviation {
			case "G":
				p.Goals = n
			case "A":
				p.Assists = n
			case "OG":
				p.OwnGoals = n
			case "SV":
				p.Saves = n
			case "YC":
				p.Yellow = n > 0
			case "RC":
				p.Red = n > 0
			}
		}
		out = append(out, p)
	}
	// Starters by formation place, keeper first, then the bench as ESPN
	// lists it.
	slices.SortStableFunc(out, func(a, b models.LineupPlayer) int {
		switch {
		case a.Starter != b.Starter:
			if a.Starter {
				return -1
			}
			return 1
		case a.Starter:
			return a.Place - b.Place
		}
		return 0
	})
	return out
}

func playerRef(espnID string) string {
	if espnID == "" {
		return ""
	}
	return idPrefix + espnID
}

// mapEvent keeps goals, cards, substitutions and the two whistles; delays
// and kick-offs are dropped.
func mapEvent(e summaryEvent, sides map[string]string) (models.MatchEvent, bool) {
	ev := models.MatchEvent{Period: e.Period.Number, Text: strings.TrimSpace(e.Text)}
	ev.Minute, ev.Added = parseClock(e.Clock.DisplayValue)
	if e.Shootout {
		ev.Period = 5
	}
	if e.Team != nil {
		ev.Side = sides[e.Team.ID]
	}
	who := func(i int) (string, string) {
		if i >= len(e.Participants) {
			return "", ""
		}
		a := e.Participants[i].Athlete
		return a.DisplayName, playerRef(a.ID)
	}
	ev.Player, ev.PlayerID = who(0)
	ev.Other, ev.OtherID = who(1)

	t := e.Type.Type
	switch {
	case t == "own-goal":
		ev.Kind = models.EventOwnGoal
		ev.Other, ev.OtherID = "", ""
	case t == "penalty---scored":
		ev.Kind = models.EventPenaltyGoal
	case strings.HasPrefix(t, "penalty---"):
		ev.Kind = models.EventPenaltyMiss
		ev.Other, ev.OtherID = "", ""
	case t == "goal" || strings.HasPrefix(t, "goal---"):
		ev.Kind = models.EventGoal
	case t == "yellow-card":
		ev.Kind = models.EventYellow
	case t == "red-card":
		ev.Kind = models.EventRed
		if strings.HasPrefix(strings.ToLower(ev.Text), "second yellow") {
			ev.Kind = models.EventSecondYellow
		}
	case t == "substitution":
		ev.Kind = models.EventSub
	case t == "halftime":
		ev.Kind, ev.Side = models.EventHalfTime, models.SideNone
	case t == "end-regular-time" || t == "end-extra-time" || t == "final":
		ev.Kind, ev.Side = models.EventFullTime, models.SideNone
	default:
		return models.MatchEvent{}, false
	}
	if ev.IsCard() {
		ev.Other, ev.OtherID = "", ""
	}
	if ev.Side == models.SideNone && ev.Kind != models.EventHalfTime && ev.Kind != models.EventFullTime {
		// A goal, card or sub with no team can't be placed on the timeline.
		return models.MatchEvent{}, false
	}
	return ev, true
}

// parseClock reads ESPN's "45'+2'" as 45 and 2; "" is 0, 0.
func parseClock(s string) (minute, added int) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "'", "")
	base, extra, _ := strings.Cut(s, "+")
	minute, _ = strconv.Atoi(base)
	added, _ = strconv.Atoi(extra)
	return minute, added
}

// tidyClock writes ESPN's "45'+2'" the way the rest of footui does: "45+2'".
func tidyClock(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return models.MinuteLabel(parseClock(s))
}
