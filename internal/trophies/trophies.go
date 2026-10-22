// Package trophies applies hand-curated club honours to cached teams. The
// dataset is deliberately partial; seed/trophies.yaml's header says what
// it covers.
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

// SeedSource is the provenance recorded on trophies from the seed file.
const SeedSource = "seed:v1"

// LeagueTitlesFrom is the earliest season the league-title lists cover;
// the UI must show counts as "since" this, never as all-time totals.
const LeagueTitlesFrom = "2000-01"

// Honour is one title, won in the listed seasons.
type Honour struct {
	Title string `yaml:"title"`
	// Type is league_title, domestic_cup, continental_cup or other.
	Type string `yaml:"type"`
	// Competition is an optional in-scope competition ID.
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
	Teams    int
	Trophies int
	// Unused counts curated entries that matched no cached team.
	Unused int
}

// Seed applies the curated honours to every cached team; it is driven by
// the cache so rows never reference a missing team.
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

		// Replace, so corrected seed data doesn't leave old rows behind.
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
