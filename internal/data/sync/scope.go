package sync

import (
	"strconv"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// Provider names, as recorded in the sync log and used to route a
// competition to the package that fetches it.
const (
	ProviderUnderstat = "understat"
	ProviderESPN      = "espn"
)

// Coverage is one competition footy can sync: which provider serves it,
// how its seasons are counted, and how far back its history goes.
type Coverage struct {
	Competition models.Competition
	Provider    string
	// FirstSeason is the earliest season start year the provider has a
	// usable table for.
	FirstSeason int
}

// Coverages is every competition footy can sync. A provider-side table
// (understat's leagueKeys, espn's leagueSlugs) must have a row for each
// ID routed to it.
//
// understat.com serves the six leagues it has expected goals for — it is
// kept as their source because nothing else free has xG. ESPN's public
// API covers everything else: MLS, the women's leagues, the second
// divisions, and the European competitions, with no xG but with
// possession, corners, and fouls, which understat never had.
var Coverages = []Coverage{
	// Men's top flights — understat (with xG).
	{men("eng-premier-league", "Premier League", "England", 1), ProviderUnderstat, 2014},
	{men("esp-la-liga", "La Liga", "Spain", 1), ProviderUnderstat, 2014},
	{men("ger-bundesliga", "Bundesliga", "Germany", 1), ProviderUnderstat, 2014},
	{men("ita-serie-a", "Serie A", "Italy", 1), ProviderUnderstat, 2014},
	{men("fra-ligue-1", "Ligue 1", "France", 1), ProviderUnderstat, 2014},
	{men("rus-premier-league", "Russian Premier League", "Russia", 1), ProviderUnderstat, 2014},

	// Men's second divisions — ESPN.
	{men("eng-championship", "Championship", "England", 2), ProviderESPN, 2010},
	{men("esp-la-liga-2", "La Liga 2", "Spain", 2), ProviderESPN, 2010},
	{men("ger-2-bundesliga", "2. Bundesliga", "Germany", 2), ProviderESPN, 2010},
	{men("ita-serie-b", "Serie B", "Italy", 2), ProviderESPN, 2010},
	{men("fra-ligue-2", "Ligue 2", "France", 2), ProviderESPN, 2010},

	// USA — ESPN, calendar-year seasons.
	{calendar(men("usa-mls", "MLS", "USA", 1)), ProviderESPN, 2010},
	{calendar(men("usa-usl-championship", "USL Championship", "USA", 2)), ProviderESPN, 2017},

	// Women's leagues — ESPN.
	{women("eng-wsl", "Women's Super League", "England", 1), ProviderESPN, 2018},
	{women("esp-liga-f", "Liga F", "Spain", 1), ProviderESPN, 2022},
	{women("fra-premiere-ligue", "Première Ligue", "France", 1), ProviderESPN, 2022},
	{calendar(women("usa-nwsl", "NWSL", "USA", 1)), ProviderESPN, 2016},

	// European competitions — ESPN. Country "Europe" keeps them together
	// in lists; their tables are the league phase (or the group stage in
	// older seasons).
	{men("eur-champions-league", "Champions League", "Europe", 1), ProviderESPN, 2010},
	{men("eur-europa-league", "Europa League", "Europe", 1), ProviderESPN, 2010},
	{men("eur-conference-league", "Conference League", "Europe", 1), ProviderESPN, 2021},
	{women("eur-womens-champions-league", "Women's Champions League", "Europe", 1), ProviderESPN, 2021},
}

func men(id, name, country string, tier int) models.Competition {
	return models.Competition{ID: id, Name: name, Country: country, Tier: tier, Gender: "men"}
}

func women(id, name, country string, tier int) models.Competition {
	c := men(id, name, country, tier)
	c.Gender = "women"
	return c
}

func calendar(c models.Competition) models.Competition {
	c.CalendarYear = true
	return c
}

// Scope is every syncable competition, in Coverages order.
var Scope = func() []models.Competition {
	out := make([]models.Competition, len(Coverages))
	for i, c := range Coverages {
		out[i] = c.Competition
	}
	return out
}()

// FindCompetition returns the in-Scope competition matching id, or false.
func FindCompetition(id string) (models.Competition, bool) {
	c, ok := FindCoverage(id)
	return c.Competition, ok
}

// FindCoverage returns the coverage entry for a competition ID.
func FindCoverage(id string) (Coverage, bool) {
	for _, c := range Coverages {
		if c.Competition.ID == id {
			return c, true
		}
	}
	return Coverage{}, false
}

// calendarRolloverMonth is when a calendar-year league's new season is
// treated as current. MLS and NWSL kick off in late February or March;
// before that the season just finished is the one with data.
const calendarRolloverMonth = time.March

// CurrentSeason is the start year of this competition's season in
// progress at now.
func (c Coverage) CurrentSeason(now time.Time) string {
	if !c.Competition.CalendarYear {
		return CurrentSeasonStartYear(now)
	}
	year := now.Year()
	if now.Month() < calendarRolloverMonth {
		year--
	}
	return strconv.Itoa(year)
}

// RecentSeasons returns the n most recent seasons, most recent first,
// never reaching back before FirstSeason. n < 1 is treated as 1.
func (c Coverage) RecentSeasons(now time.Time, n int) []string {
	if n < 1 {
		n = 1
	}
	start, err := strconv.Atoi(c.CurrentSeason(now))
	if err != nil {
		return nil
	}
	var out []string
	for y := start; y > start-n && y >= c.FirstSeason; y-- {
		out = append(out, strconv.Itoa(y))
	}
	return out
}

// AvailableSeasons returns every season the provider offers for this
// competition, current first.
func (c Coverage) AvailableSeasons(now time.Time) []string {
	start, err := strconv.Atoi(c.CurrentSeason(now))
	if err != nil || start < c.FirstSeason {
		return nil
	}
	return c.RecentSeasons(now, start-c.FirstSeason+1)
}
