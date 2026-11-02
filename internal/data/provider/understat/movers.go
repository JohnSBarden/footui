package understat

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// A mid-season mover's league line is one aggregate under a comma-joined
// team_title ("Barcelona,Atletico Madrid"). Their player page splits it
// by club, at one request a mover.

// playerData is the part of getPlayerData/{id} used here.
type playerData struct {
	Groups struct {
		Season []clubSeason `json:"season"`
	} `json:"groups"`
}

// clubSeason is one season at one club, in every league they played in.
type clubSeason struct {
	Season      string `json:"season"`
	Team        string `json:"team"`
	Games       string `json:"games"`
	Time        string `json:"time"`
	Goals       string `json:"goals"`
	Assists     string `json:"assists"`
	XG          string `json:"xG"`
	XA          string `json:"xA"`
	Shots       string `json:"shots"`
	KeyPasses   string `json:"key_passes"`
	NPG         string `json:"npg"`
	NPxG        string `json:"npxG"`
	YellowCards string `json:"yellow"`
	RedCards    string `json:"red"`
}

func (c *Client) fetchPlayerData(ctx context.Context, playerID string) (*playerData, error) {
	c.mu.Lock()
	cached, ok := c.players[playerID]
	c.mu.Unlock()
	if ok {
		return cached, nil
	}
	url := fmt.Sprintf("%s/getPlayerData/%s", c.baseURL, playerID)
	body, err := c.getURL(ctx, url, fmt.Sprintf("%s/player/%s", c.baseURL, playerID), "player", playerID)
	if err != nil {
		return nil, err
	}
	var data playerData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("understat: decode player %s: %w", playerID, err)
	}
	c.mu.Lock()
	c.players[playerID] = &data
	c.mu.Unlock()
	return &data, nil
}

// splitMovers replaces each mover's aggregate line with one per club,
// from their player page. A mover whose page can't be fetched, or doesn't
// split cleanly, keeps the aggregate under their latest club; a missing
// split shouldn't fail a season.
func (p *Provider) splitMovers(ctx context.Context, data *leagueData, compID, label string, rows []models.PlayerSeasonStats) []models.PlayerSeasonStats {
	out := rows[:0:0]
	for i, pl := range data.Players {
		clubs := strings.Split(pl.TeamTitle, ",")
		if len(clubs) < 2 || ctx.Err() != nil {
			out = append(out, rows[i])
			continue
		}
		page, err := p.client.fetchPlayerData(ctx, pl.ID)
		if err != nil {
			out = append(out, rows[i])
			continue
		}
		split := splitLine(page, rows[i], compID, label, clubs)
		if len(split) < 2 {
			out = append(out, rows[i])
			continue
		}
		out = append(out, split...)
	}
	return out
}

// splitLine is agg as one line per club of clubs that the page has a
// season-label line for.
func splitLine(page *playerData, agg models.PlayerSeasonStats, compID, label string, clubs []string) []models.PlayerSeasonStats {
	for i := range clubs {
		clubs[i] = strings.TrimSpace(clubs[i])
	}
	var out []models.PlayerSeasonStats
	for _, cs := range page.Groups.Season {
		if cs.Season != label || !slices.Contains(clubs, cs.Team) {
			continue
		}
		line := agg
		line.TeamID = teamID(compID, cs.Team)
		line.Appearances, _ = strconv.Atoi(cs.Games)
		line.Minutes, _ = strconv.Atoi(cs.Time)
		line.Goals, _ = strconv.Atoi(cs.Goals)
		line.Assists, _ = strconv.Atoi(cs.Assists)
		line.YellowCards, _ = strconv.Atoi(cs.YellowCards)
		line.RedCards, _ = strconv.Atoi(cs.RedCards)
		line.XG = optionalFloat(cs.XG)
		line.XA = optionalFloat(cs.XA)
		line.Shots = optionalInt(cs.Shots)
		line.KeyPasses = optionalInt(cs.KeyPasses)
		line.NonPenaltyGoals = optionalInt(cs.NPG)
		line.NPxG = optionalFloat(cs.NPxG)
		line.UpdatedAt = time.Now().UTC()
		out = append(out, line)
	}
	return out
}
