package sync

import "github.com/JohnSBarden/footui/internal/data/models"

// NewSeason builds the canonical Season for a competition and a starting
// year (e.g. "2025" for the 2025-26 season). Label is kept as the bare
// start-year string understat.com itself uses as a season key (it's also
// what gets passed back out as provider.SeasonRef.Label) — display
// formatting like "2025-26" is a TUI-layer concern, not stored here.
func NewSeason(competitionID, startYear string) models.Season {
	return models.Season{
		ID:            competitionID + ":" + startYear,
		CompetitionID: competitionID,
		Label:         startYear,
	}
}
