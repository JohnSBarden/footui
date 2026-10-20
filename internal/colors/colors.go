// Package colors loads footui's hand-curated club brand colours and applies
// them to teams already in the cache.
//
// There is no scrapeable structured source for club colours, so this seed
// file is a manual dataset — a known chore from the original design, not an
// oversight. It is applied rather than trusted: an entry only takes effect
// when its ID (or one of its aliases) matches a team the provider actually
// returned, so an entry for a club footui has never cached, or one whose
// ID guesses the provider's spelling wrongly, is simply inert.
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

// SeedSource is the provenance recorded on colours written from this file,
// so a future automated source can be told apart from the hand-curated set.
const SeedSource = "seed:v1"

// Entry is one club's curated colours.
type Entry struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	// Aliases are alternative canonical IDs the same club might appear
	// under, because a team ID is derived from however the data provider
	// spells the club's name.
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

// byTeamID indexes entries by every ID they answer to.
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
	// Applied is the number of cached teams that got colours.
	Applied int
	// Uncolored lists cached teams with no matching seed entry — the
	// actionable output, since each is a club someone can add to the file.
	Uncolored []models.Team
	// Unused counts seed entries that matched no cached team. Usually
	// benign (that competition hasn't been refreshed), but a persistently
	// unused entry for a league you have refreshed means its ID is wrong.
	Unused int
}

// Seed applies the curated colours to every team currently in the cache.
// It is driven by what is cached rather than by the file so that
// team_colors never references a team row that doesn't exist, and so
// re-running it after refreshing a new competition picks that competition
// up with no extra step.
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
