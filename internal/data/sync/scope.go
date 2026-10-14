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
	// usable table for. For an international competition it is also the
	// first calendar year of its matches the provider has.
	FirstSeason int
	// Editions lists a tournament's editions, by the year each is named
	// for; nil means one season a year. Tournaments are every two or four
	// years, and irregularly — Copa América 2015 and 2016 — so the list is
	// explicit rather than a rule.
	Editions []int
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
	{men("eng-premier-league", "Premier League", "England", 1), ProviderUnderstat, 2014, nil},
	{men("esp-la-liga", "La Liga", "Spain", 1), ProviderUnderstat, 2014, nil},
	{men("ger-bundesliga", "Bundesliga", "Germany", 1), ProviderUnderstat, 2014, nil},
	{men("ita-serie-a", "Serie A", "Italy", 1), ProviderUnderstat, 2014, nil},
	{men("fra-ligue-1", "Ligue 1", "France", 1), ProviderUnderstat, 2014, nil},
	{men("rus-premier-league", "Russian Premier League", "Russia", 1), ProviderUnderstat, 2014, nil},

	// Men's second divisions — ESPN.
	{men("eng-championship", "Championship", "England", 2), ProviderESPN, 2010, nil},
	{men("esp-la-liga-2", "La Liga 2", "Spain", 2), ProviderESPN, 2010, nil},
	{men("ger-2-bundesliga", "2. Bundesliga", "Germany", 2), ProviderESPN, 2010, nil},
	{men("ita-serie-b", "Serie B", "Italy", 2), ProviderESPN, 2010, nil},
	{men("fra-ligue-2", "Ligue 2", "France", 2), ProviderESPN, 2010, nil},

	// USA — ESPN, calendar-year seasons.
	{calendar(men("usa-mls", "MLS", "USA", 1)), ProviderESPN, 2010, nil},
	{calendar(men("usa-usl-championship", "USL Championship", "USA", 2)), ProviderESPN, 2017, nil},

	// Women's leagues — ESPN.
	{women("eng-wsl", "Women's Super League", "England", 1), ProviderESPN, 2018, nil},
	{women("esp-liga-f", "Liga F", "Spain", 1), ProviderESPN, 2022, nil},
	{women("fra-premiere-ligue", "Première Ligue", "France", 1), ProviderESPN, 2022, nil},
	{calendar(women("usa-nwsl", "NWSL", "USA", 1)), ProviderESPN, 2016, nil},

	// European competitions — ESPN. Country "Europe" keeps them together
	// in lists; their tables are the league phase (or the group stage in
	// older seasons).
	{men("eur-champions-league", "Champions League", "Europe", 1), ProviderESPN, 2010, nil},
	{men("eur-europa-league", "Europa League", "Europe", 1), ProviderESPN, 2010, nil},
	{men("eur-conference-league", "Conference League", "Europe", 1), ProviderESPN, 2021, nil},
	{women("eur-womens-champions-league", "Women's Champions League", "Europe", 1), ProviderESPN, 2021, nil},

	// International football — ESPN. A nation's matches come from every
	// one of these of its gender; each first year was checked against
	// ESPN, and anything that didn't answer was left out. An edition is
	// keyed by the year it is named for (Euro 2020, played in 2021, is
	// 2020), so tournaments and friendlies show as one year, and
	// qualifiers and nations leagues, which span two or three, as a split
	// season.
	{international("world-cup", "FIFA World Cup", "WC", "men", models.CategoryTournament), ProviderESPN, 2010,
		[]int{2010, 2014, 2018, 2022, 2026}},
	{international("euro", "European Championship", "EURO", "men", models.CategoryTournament), ProviderESPN, 2012,
		[]int{2012, 2016, 2020, 2024}},
	{international("copa-america", "Copa América", "COPA", "men", models.CategoryTournament), ProviderESPN, 2011,
		[]int{2011, 2015, 2016, 2019, 2020, 2024}},
	{international("gold-cup", "Gold Cup", "GOLD", "men", models.CategoryTournament), ProviderESPN, 2011,
		[]int{2011, 2013, 2015, 2017, 2019, 2021, 2023, 2025}},
	{international("afcon", "Africa Cup of Nations", "AFCON", "men", models.CategoryTournament), ProviderESPN, 2012,
		[]int{2012, 2013, 2015, 2017, 2019, 2021, 2023, 2025}},
	{international("asian-cup", "AFC Asian Cup", "ASIA", "men", models.CategoryTournament), ProviderESPN, 2011,
		[]int{2011, 2015, 2019, 2023}},
	{international("wcq-uefa", "WC Qualifying – UEFA", "WCQ", "men", models.CategoryQualifier), ProviderESPN, 2012, nil},
	{international("wcq-conmebol", "WC Qualifying – CONMEBOL", "WCQ", "men", models.CategoryQualifier), ProviderESPN, 2011, nil},
	{international("wcq-concacaf", "WC Qualifying – Concacaf", "WCQ", "men", models.CategoryQualifier), ProviderESPN, 2011, nil},
	{international("wcq-afc", "WC Qualifying – AFC", "WCQ", "men", models.CategoryQualifier), ProviderESPN, 2011, nil},
	{international("wcq-caf", "WC Qualifying – CAF", "WCQ", "men", models.CategoryQualifier), ProviderESPN, 2011, nil},
	{international("euro-qualifying", "Euro Qualifying", "EQ", "men", models.CategoryQualifier), ProviderESPN, 2011, nil},
	{international("nations-league", "UEFA Nations League", "UNL", "men", models.CategoryNationsLeague), ProviderESPN, 2018, nil},
	{international("concacaf-nations-league", "Concacaf Nations League", "CNL", "men", models.CategoryNationsLeague), ProviderESPN, 2019, nil},
	{international("int-friendly", "International Friendlies", "FRI", "men", models.CategoryFriendly), ProviderESPN, 2010, nil},

	{international("world-cup-women", "FIFA Women's World Cup", "WWC", "women", models.CategoryTournament), ProviderESPN, 2011,
		[]int{2011, 2015, 2019, 2023}},
	{international("euro-women", "Women's Euro", "WEURO", "women", models.CategoryTournament), ProviderESPN, 2017,
		[]int{2017, 2022, 2025}},
	{international("nations-league-women", "Women's Nations League", "WNL", "women", models.CategoryNationsLeague), ProviderESPN, 2025, nil},
	{international("int-friendly-women", "Women's International Friendlies", "FRI", "women", models.CategoryFriendly), ProviderESPN, 2015, nil},
}

// international builds an international competition. Its country is
// "International", which keeps them together in lists; tier means nothing
// outside a domestic pyramid, so it is 0.
func international(id, name, short, gender, category string) models.Competition {
	return models.Competition{
		ID: id, Name: name, ShortName: short, Country: "International", Gender: gender, Format: "cup",
		Category:     category,
		CalendarYear: category == models.CategoryTournament || category == models.CategoryFriendly,
	}
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

// Scope is every syncable club competition, in Coverages order: what
// `refresh --all` covers. International competitions are left out — a
// nation's matches are fetched for the nations someone follows, not for
// every competition on a schedule.
var Scope = func() []models.Competition {
	var out []models.Competition
	for _, c := range Coverages {
		if !c.Competition.International() {
			out = append(out, c.Competition)
		}
	}
	return out
}()

// InternationalCoverages is every international competition of one gender
// ("men" or "women"), in Coverages order: the competitions a nation's
// matches are fetched from.
func InternationalCoverages(gender string) []Coverage {
	var out []Coverage
	for _, c := range Coverages {
		if c.Competition.International() && c.Competition.Gender == gender {
			out = append(out, c)
		}
	}
	return out
}

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
	switch {
	case c.Editions != nil:
		// The latest edition that has begun its year: in October 2026 the
		// World Cup's current edition is 2026.
		if eds := c.AvailableSeasons(now); len(eds) > 0 {
			return eds[0]
		}
		return strconv.Itoa(c.Editions[0])
	case c.Competition.International():
		// A qualifier or nations league runs all year round, and a match
		// list is fetched by calendar year.
		return strconv.Itoa(now.Year())
	}
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
	all := c.AvailableSeasons(now)
	return all[:min(n, len(all))]
}

// AvailableSeasons returns every season the provider offers for this
// competition, current first: a tournament's editions so far, otherwise
// every year back to FirstSeason.
func (c Coverage) AvailableSeasons(now time.Time) []string {
	if c.Editions != nil {
		var out []string
		for i := len(c.Editions) - 1; i >= 0; i-- {
			if c.Editions[i] <= now.Year() {
				out = append(out, strconv.Itoa(c.Editions[i]))
			}
		}
		return out
	}
	start, err := strconv.Atoi(c.CurrentSeason(now))
	if err != nil || start < c.FirstSeason {
		return nil
	}
	var out []string
	for y := start; y >= c.FirstSeason; y-- {
		out = append(out, strconv.Itoa(y))
	}
	return out
}
