// Package colors applies hand-curated club colours to cached teams. An
// entry only takes effect when its ID or an alias matches a cached team.
package colors

import (
	"context"
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

//go:embed seed/teamcolors.yaml
var seedFile []byte

// SeedSource is the provenance recorded on colours from the seed file.
const SeedSource = "seed:v1"

// Entry is one club's curated colours.
type Entry struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	// Aliases cover other IDs a provider's spelling of the name may yield.
	Aliases   []string `yaml:"aliases"`
	Primary   string   `yaml:"primary"`
	Secondary string   `yaml:"secondary"`
	Tertiary  string   `yaml:"tertiary"`
}

// Load parses the embedded seed file.
func Load() ([]Entry, error) {
	var entries []Entry
	if err := yaml.Unmarshal(seedFile, &entries); err != nil {
		return nil, fmt.Errorf("colors: parse seed file: %w", err)
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
	Applied int
	// Uncolored lists cached teams with no matching seed entry.
	Uncolored []models.Team
	// Unused counts seed entries that matched no cached team; one for a
	// refreshed league means its ID is wrong.
	Unused int
}

// Seed applies the curated colours to every cached team; it is driven by
// the cache so team_colors never references a missing team.
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
			report.Uncolored = append(report.Uncolored, team)
			continue
		}

		if err := repos.UpsertTeamColors(ctx, models.TeamColors{
			TeamID:       team.ID,
			PrimaryHex:   entry.Primary,
			SecondaryHex: entry.Secondary,
			TertiaryHex:  entry.Tertiary,
			Source:       SeedSource,
		}); err != nil {
			return report, fmt.Errorf("colors: apply to %s: %w", team.ID, err)
		}
		report.Applied++
		used[entry.ID] = true
	}

	for _, e := range entries {
		if !used[e.ID] {
			report.Unused++
		}
	}

	return report, nil
}
