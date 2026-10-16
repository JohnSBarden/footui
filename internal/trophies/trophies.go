// Package trophies loads footui's hand-curated club honours and applies
// them to teams already in the cache.
//
// Like club colors, honours have no scrapeable structured source, so this
// is a manual dataset and a deliberately partial one — see the header of
// seed/trophies.yaml for exactly what it claims to cover. The Team screen
// labels the panel with that scope so a partial list is never read as an
// all-time honours count.
package trophies

import (
	"context"
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

//go:embed seed/trophies.yaml
var seedFile []byte

// SeedSource is the provenance recorded on trophies written from this
// file, so a future automated source can be told apart from the curated set.
const SeedSource = "seed:v1"

// LeagueTitlesFrom is the earliest season the curated league-title lists
// cover. It is surfaced in the UI: a club's count here is "titles since
// 2000-01", never an all-time total, and saying so is the difference
// between a useful partial dataset and a wrong one.
const LeagueTitlesFrom = "2000-01"

// Honour is one line of a club's honours list: a single title, won in the
// listed seasons.
type Honour struct {
	Title string `yaml:"title"`
	// Type is one of the models.Trophy trophy types: league_title,
	// domestic_cup, continental_cup, other.
	Type string `yaml:"type"`
	// Competition optionally links the honour to an in-scope competition
	// ID. Continental cups have none, since footui doesn't cover them as
	// competitions in their own right.
	Competition string   `yaml:"competition"`
	Seasons     []string `yaml:"seasons"`
}

// Entry is one club's curated honours.
type Entry struct {
	ID      string   `yaml:"id"`
	Aliases []string `yaml:"aliases"`
	Honours []Honour `yaml:"honours"`
}

// Load parses the embedded seed file.
func Load() ([]Entry, error) {
	var entries []Entry
	if err := yaml.Unmarshal(seedFile, &entries); err != nil {
		return nil, fmt.Errorf("trophies: parse seed file: %w", err)
	}
	return entries, nil
}

func byTeamID(entries []Entry) map[string]Entry {
	index := make(map[string]Entry, len(entries)*2)
	for _, e := range entries {
		index[e.ID] = e
		for _, alias := range e.Aliases {
			index[alias] = e
		}
	}
	return index
}

// Report describes what a Seed call did.
type Report struct {
	// Teams is how many cached clubs got honours.
	Teams int
	// Trophies is how many individual honour rows were written.
	Trophies int
	// Unused counts curated entries that matched no cached team.
	Unused int
}

// Seed applies the curated honours to every team currently in the cache.
// Like the colors seed it is driven by what is cached, so trophy rows can
// never reference a team that doesn't exist and refreshing a new
// competition picks up its clubs with no extra step.
func Seed(ctx context.Context, repos storage.Repositories) (Report, error) {
	var report Report

	entries, err := Load()
	if err != nil {
		return report, err
	}
	index := byTeamID(entries)

	teams, err := repos.ListTeams(ctx)
	if err != nil {
		return report, err
	}

	used := map[string]bool{}
	for _, team := range teams {
		entry, ok := index[team.ID]
		if !ok {
			continue
		}

		// Replace rather than append: re-running a seed whose data has
		// been corrected must not leave the old rows behind alongside the
		// new ones.
		if err := repos.DeleteTrophiesBySource(ctx, team.ID, SeedSource); err != nil {
			return report, fmt.Errorf("trophies: clear %s: %w", team.ID, err)
		}

		for _, honour := range entry.Honours {
			for _, season := range honour.Seasons {
				if err := repos.InsertTrophy(ctx, models.Trophy{
					TeamID:        team.ID,
					CompetitionID: honour.Competition,
					Title:         honour.Title,
					SeasonLabel:   season,
					TrophyType:    honour.Type,
					Source:        SeedSource,
				}); err != nil {
					return report, fmt.Errorf("trophies: insert %s %s: %w", team.ID, season, err)
				}
				report.Trophies++
			}
		}
		report.Teams++
		used[entry.ID] = true
	}

	for _, e := range entries {
		if !used[e.ID] {
			report.Unused++
		}
	}

	return report, nil
}
