package sync

import (
	"strconv"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// NewSeason builds the canonical Season for a competition and start year
// ("2025" for 2025-26). Label stays the bare year: it is understat's season
// key and goes back out as provider.SeasonRef.Label.
func NewSeason(competitionID, startYear string) models.Season {
	return models.Season{
		ID:            competitionID + ":" + startYear,
		CompetitionID: competitionID,
		Label:         startYear,
	}
}

// seasonRolloverMonth is when a new European season counts as begun.
// August, not July: in July the new season has no matches yet.
const seasonRolloverMonth = time.August

// CurrentSeasonStartYear returns the start year of the season in progress.
func CurrentSeasonStartYear(now time.Time) string {
	year := now.Year()
	if now.Month() < seasonRolloverMonth {
		year--
	}
	return strconv.Itoa(year)
}
