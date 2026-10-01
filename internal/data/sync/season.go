package sync

import (
	"strconv"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

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

// seasonRolloverMonth is the month a new European season is considered to
// have begun. August rather than July: in July the new season has no
// matches played yet, so treating it as current would point a refresh at
// an empty payload while last season's completed data is what a user
// actually wants to see.
const seasonRolloverMonth = time.August

// CurrentSeasonStartYear returns the start year of the season in progress
// at the given time, as the bare-year string used for season labels.
func CurrentSeasonStartYear(now time.Time) string {
	year := now.Year()
	if now.Month() < seasonRolloverMonth {
		year--
	}
	return strconv.Itoa(year)
}

// EarliestSeasonYear is the first season the provider has data for.
// understat.com's coverage of all six leagues starts with 2014-15.
const EarliestSeasonYear = 2014

// AvailableSeasonYears returns every season the provider offers, current
// first, back to EarliestSeasonYear.
func AvailableSeasonYears(now time.Time) []string {
	start, err := strconv.Atoi(CurrentSeasonStartYear(now))
	if err != nil || start < EarliestSeasonYear {
		return nil
	}
	return RecentSeasonYears(now, start-EarliestSeasonYear+1)
}

// RecentSeasonYears returns the start years of the n most recent seasons,
// most recent first. n < 1 is treated as 1, so a caller passing a
// zero-valued flag still gets the current season rather than nothing.
func RecentSeasonYears(now time.Time, n int) []string {
	if n < 1 {
		n = 1
	}
	start, err := strconv.Atoi(CurrentSeasonStartYear(now))
	if err != nil {
		return nil
	}

	years := make([]string, 0, n)
	for i := 0; i < n; i++ {
		years = append(years, strconv.Itoa(start-i))
	}
	return years
}
