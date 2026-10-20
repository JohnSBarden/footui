package sync

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/textfold"
)

// Identity resolution: a provider that doesn't speak footui's canonical
// IDs returns its own ("espn:359"), and before anything is written each is
// mapped to the canonical team or player it denotes. Without this, Arsenal
// in the Champions League (from ESPN) and Arsenal in the Premier League
// (from understat) would be two clubs, and Saka would have two careers.
//
// For a team, in order:
//  1. A mapping recorded on an earlier refresh.
//  2. A cached club of the same gender whose normalized name matches
//     exactly ("Paris Saint-Germain" = "Paris Saint Germain", "AS Roma" =
//     "Roma"), or matches through a short alias table for names that no
//     rule can equate ("Internazionale" = "Inter"). For a domestic
//     competition the club must be from the same country, and a unique
//     word-subset match also counts ("Coventry City" = "Coventry"), which
//     is what follows a club between divisions. A European competition's
//     clubs come from everywhere, so there it's exact or alias only:
//     "Paris Saint-Germain" must never become "Paris FC". A domestic
//     cup's word-subset match only counts when the words that differ are
//     club suffixes ("Leeds United" = "Leeds", but "FC United of
//     Manchester" is not Manchester United). A club the provider already
//     knows by a different one of its IDs is never a match: the
//     provider says it's another club ("Miami FC" is not "Inter Miami
//     CF", "Columbus Crew 2" not "Columbus Crew").
//  3. Otherwise a new canonical club.
//
// A player is matched within the squad of the club they were resolved
// to — exact normalized name, then a unique word-subset ("Gabriel" =
// "Gabriel Magalhães" only if no other Gabriel played there) — and is
// otherwise new. Every resolution is recorded, so later refreshes are
// stable and never re-guess.

// canonicalLocalIDs are providers whose own IDs are already footui's
// canonical form. understat was footui's first provider, and its scheme
// became the canonical one: a club it reports that matches nothing cached
// keeps that ID, so every existing database and curated seed still lines
// up.
var canonicalLocalIDs = map[string]bool{"understat": true}

// nativeID reports whether id is a provider's own, and splits it.
func nativeID(id string) (provider, local string, ok bool) {
	provider, local, ok = strings.Cut(id, ":")
	return provider, local, ok && provider != "" && local != ""
}

// clubAliases equates normalized names that differ by more than
// punctuation and noise words. Keyed and valued by normalizeClub output.
var clubAliases = map[string]string{
	"internazionale":           "inter",
	"inter milan":              "inter",
	"tottenham hotspur":        "tottenham",
	"brighton hove albion":     "brighton",
	"rb leipzig":               "rasenballsport leipzig",
	"west ham united":          "west ham",
	"leicester city":           "leicester",
	"wolverhampton":            "wolverhampton wanderers",
	"olympique marseille":      "marseille",
	"olympique lyonnais":       "lyon",
	"borussia monchengladbach": "borussia m gladbach",
	"zenit st petersburg":      "zenit saint petersburg",
}

// clubNoise are words that distinguish nothing: "AS Roma" is "Roma".
var clubNoise = map[string]bool{
	"fc": true, "cf": true, "afc": true, "sc": true, "as": true, "ac": true,
	"club": true, "sv": true, "fk": true, "ss": true, "ssc": true, "cd": true,
	"ud": true, "rc": true, "vfl": true, "vfb": true, "tsg": true, "the": true,
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// normalizeClub reduces a club name to the words that identify it.
func normalizeClub(name string) string {
	var words []string
	for _, w := range nonAlnum.Split(textfold.Fold(name), -1) {
		if w == "" || clubNoise[w] || isDigits(w) {
			continue
		}
		if w == "st" {
			w = "saint"
		}
		words = append(words, w)
	}
	n := strings.Join(words, " ")
	if a, ok := clubAliases[n]; ok {
		return a
	}
	return n
}

// normalizePerson reduces a player's name to lowercase, accent-free words.
func normalizePerson(name string) string {
	return strings.Join(strings.Fields(nonAlnum.ReplaceAllString(textfold.Fold(name), " ")), " ")
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// subset reports whether every word of a is in b, or every word of b in a.
func subset(a, b string) bool {
	in := func(x, y string) bool {
		ys := map[string]bool{}
		for _, w := range strings.Fields(y) {
			ys[w] = true
		}
		for _, w := range strings.Fields(x) {
			if !ys[w] {
				return false
			}
		}
		return x != ""
	}
	return in(a, b) || in(b, a)
}

// clubSuffixes are the words a club's name can gain or lose between
// sources and still be the same club: "Leeds United" is "Leeds".
var clubSuffixes = map[string]bool{
	"united": true, "city": true, "town": true, "county": true, "rovers": true, "wanderers": true,
	"athletic": true, "albion": true, "hotspur": true, "villa": true, "rangers": true, "argyle": true,
	"alexandra": true, "orient": true, "calcio": true, "balompie": true, "1899": true, "1846": true,
}

// suffixesOnly reports whether every word one name has and the other
// lacks is a club suffix. A cup brings in clubs no league table had, and
// a word-subset alone would make "FC United of Manchester" Manchester
// United; this keeps a loose match to the differences that don't matter.
func suffixesOnly(a, b string) bool {
	count := func(s string) map[string]int {
		m := map[string]int{}
		for _, w := range strings.Fields(s) {
			m[w]++
		}
		return m
	}
	wa, wb := count(a), count(b)
	for w, n := range wa {
		if wb[w] < n && !clubSuffixes[w] {
			return false
		}
	}
	for w, n := range wb {
		if wa[w] < n && !clubSuffixes[w] {
			return false
		}
	}
	return true
}

var countryPrefixes = map[string]string{
	"England": "eng", "Spain": "esp", "Germany": "ger", "Italy": "ita",
	"France": "fra", "Russia": "rus", "USA": "usa",
}

func slug(s string) string {
	return strings.Trim(nonAlnum.ReplaceAllString(textfold.Fold(s), "-"), "-")
}

// resolver maps one competition-season's provider IDs to canonical ones.
type resolver struct {
	repos storage.Repositories
	comp  models.Competition

	cached  []models.Team // every cached club, loaded once
	teamOf  map[string]string
	squads  map[string][]models.Player
	claimed map[string]bool // canonical teams already taken in this table
}

func newResolver(ctx context.Context, repos storage.Repositories, comp models.Competition) (*resolver, error) {
	cached, err := repos.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	return &resolver{repos: repos, comp: comp, cached: cached,
		teamOf: map[string]string{}, squads: map[string][]models.Player{}, claimed: map[string]bool{}}, nil
}

// teams resolves every native team, returning the rows to write: the
// cached row for a club footui already has (so one provider's spelling
// never overwrites another's), the provider's row under a new canonical
// ID otherwise.
func (r *resolver) teams(ctx context.Context, teams []models.Team) ([]models.Team, error) {
	out := make([]models.Team, 0, len(teams))
	for _, t := range teams {
		provider, local, ok := nativeID(t.ID)
		if !ok {
			out = append(out, t)
			continue
		}
		// A domestic competition's clubs are from its country, whatever
		// the provider said; a European one's can't be assumed.
		if t.Country == "" && r.comp.Country != "Europe" {
			t.Country = r.comp.Country
		}
		if t.Gender == "" {
			t.Gender = r.comp.Gender
		}
		row, err := r.team(ctx, provider, local, t)
		if err != nil {
			return nil, err
		}
		r.teamOf[t.ID] = row.ID
		r.claimed[row.ID] = true
		out = append(out, row)
	}
	return out, nil
}

func (r *resolver) team(ctx context.Context, provider, local string, t models.Team) (models.Team, error) {
	if id, err := r.repos.ExternalRef(ctx, "team", provider, local); err == nil {
		if existing, ok := r.cachedTeam(id); ok {
			return existing, nil
		}
		t.ID = id
		return t, nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return models.Team{}, err
	}

	row, ok, err := r.match(ctx, provider, local, t)
	if err != nil {
		return models.Team{}, err
	}
	if !ok {
		row = t
		row.ID = r.newTeamID(t)
		if canonicalLocalIDs[provider] {
			row.ID = local
		}
	}
	if err := r.repos.PutExternalRef(ctx, "team", row.ID, provider, local); err != nil {
		return models.Team{}, err
	}
	return row, nil
}

func (r *resolver) cachedTeam(id string) (models.Team, bool) {
	for _, c := range r.cached {
		if c.ID == id {
			return c, true
		}
	}
	return models.Team{}, false
}

// match finds the cached club a fetched one denotes, if exactly one fits.
// A club the provider already knows by another of its IDs is, on the
// provider's own word, another club, however alike the names: Miami FC
// is not Inter Miami, nor Atlanta United 2 Atlanta United.
func (r *resolver) match(ctx context.Context, provider, local string, t models.Team) (models.Team, bool, error) {
	europe := r.comp.Country == "Europe"
	name := normalizeClub(t.Name)
	var exact, loose []models.Team
	for _, c := range r.cached {
		// A national team is never a club, whatever it's called.
		if c.Gender != t.Gender || r.claimed[c.ID] || c.National {
			continue
		}
		if !europe && c.Country != r.comp.Country {
			continue
		}
		cn := normalizeClub(c.Name)
		switch {
		case cn == name:
			exact = append(exact, c)
		case !europe && subset(cn, name) && (!r.comp.Cup() || suffixesOnly(cn, name)):
			loose = append(loose, c)
		}
	}
	var err error
	if exact, err = r.unclaimedBy(ctx, provider, local, t, exact); err != nil {
		return models.Team{}, false, err
	}
	if loose, err = r.unclaimedBy(ctx, provider, local, t, loose); err != nil {
		return models.Team{}, false, err
	}
	if len(exact) == 1 {
		return exact[0], true, nil
	}
	if len(exact) == 0 && len(loose) == 1 {
		return loose[0], true, nil
	}
	return models.Team{}, false, nil
}

// unclaimedBy drops the clubs provider already maps a different one of
// its IDs to, unless t has the very same name: that's the provider
// listing one club twice, as it does some nations.
func (r *resolver) unclaimedBy(ctx context.Context, provider, local string, t models.Team, clubs []models.Team) ([]models.Team, error) {
	out := clubs[:0]
	for _, c := range clubs {
		id, err := r.repos.ProviderID(ctx, "team", c.ID, provider)
		switch {
		case errors.Is(err, storage.ErrNotFound):
		case err != nil:
			return nil, err
		case id != local && normalizePerson(c.Name) != normalizePerson(t.Name):
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// newTeamID mints a canonical ID for a club footui hasn't seen: the
// country prefix and a slug of the name, "-women" for a women's side so
// it can never collide with the men's club of the same name, and "int"
// for a European club whose country isn't known.
func (r *resolver) newTeamID(t models.Team) string {
	prefix := countryPrefixes[t.Country]
	if prefix == "" {
		prefix = "int"
	}
	id := prefix + "-" + slug(t.Name)
	if t.Gender == "women" && !strings.Contains(id, "women") {
		id += "-women"
	}
	base := id
	for n := 2; ; n++ {
		existing, taken := r.cachedTeam(id)
		if (!taken || existing.Gender == t.Gender && normalizeClub(existing.Name) == normalizeClub(t.Name)) && !r.claimed[id] {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

// nations maps national teams reported under a provider's IDs to
// canonical ones, returning the rows to write and the mapping.
//
// A nation is simpler than a club: it is "nat-{slug}" (with "-women" for a
// women's side), its name is the same in every competition, and it is
// only ever matched against other national teams — never a club, so no
// "Arsenal" can become "England". As with clubs, a recorded mapping wins,
// then a cached nation of the same gender and name, then a new one.
func nations(ctx context.Context, repos storage.Repositories, teams []models.Team) ([]models.Team, map[string]string, error) {
	mapping := map[string]string{}
	cached := map[string][]models.Team{}
	for _, g := range []string{"men", "women"} {
		ts, err := repos.NationalTeams(ctx, g)
		if err != nil {
			return nil, nil, err
		}
		cached[g] = ts
	}
	byID := func(id string) (models.Team, bool) {
		for _, ts := range cached {
			for _, t := range ts {
				if t.ID == id {
					return t, true
				}
			}
		}
		return models.Team{}, false
	}

	out := make([]models.Team, 0, len(teams))
	for _, t := range teams {
		t.National = true
		provider, local, ok := nativeID(t.ID)
		if !ok {
			out = append(out, t)
			continue
		}
		if _, done := mapping[t.ID]; done {
			continue
		}
		row := t
		if id, err := repos.ExternalRef(ctx, "team", provider, local); err == nil {
			row.ID = id
			if existing, ok := byID(id); ok {
				row = existing
			}
		} else if !errors.Is(err, storage.ErrNotFound) {
			return nil, nil, err
		} else {
			row.ID = ""
			name := normalizePerson(t.Name)
			for _, c := range cached[t.Gender] {
				if normalizePerson(c.Name) == name {
					row = c
					break
				}
			}
			if row.ID == "" {
				row.ID = newNationID(t, byID)
				cached[t.Gender] = append(cached[t.Gender], row)
			}
			if err := repos.PutExternalRef(ctx, "team", row.ID, provider, local); err != nil {
				return nil, nil, err
			}
		}
		mapping[t.ID] = row.ID
		out = append(out, row)
	}
	return out, mapping, nil
}

// newNationID mints "nat-{slug}", or "nat-{slug}-women", taking a numbered
// suffix in the unlikely case another nation already has it.
func newNationID(t models.Team, taken func(string) (models.Team, bool)) string {
	base := "nat-" + slug(t.Name)
	if t.Gender == "women" {
		base += "-women"
	}
	id := base
	for n := 2; ; n++ {
		if _, used := taken(id); !used {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

// canonicalTeam maps a native team ID seen in stats rows.
func (r *resolver) canonicalTeam(id string) string {
	if c, ok := r.teamOf[id]; ok {
		return c
	}
	return id
}

// players resolves every native player, given the stat lines that say
// which club each played for, and returns the rows to write plus the
// native-to-canonical mapping.
func (r *resolver) players(ctx context.Context, players []models.Player, lines []models.PlayerSeasonStats) ([]models.Player, map[string]string, error) {
	clubOf := map[string]string{}
	for _, l := range lines {
		clubOf[l.PlayerID] = r.canonicalTeam(l.TeamID)
	}
	mapping := map[string]string{}
	taken := map[string]bool{}
	out := make([]models.Player, 0, len(players))
	for _, p := range players {
		provider, local, ok := nativeID(p.ID)
		if !ok {
			out = append(out, p)
			continue
		}
		row, err := r.player(ctx, provider, local, p, clubOf[p.ID], taken)
		if err != nil {
			return nil, nil, err
		}
		mapping[p.ID] = row.ID
		taken[row.ID] = true
		out = append(out, row)
	}
	return out, mapping, nil
}

func (r *resolver) player(ctx context.Context, provider, local string, p models.Player, club string, taken map[string]bool) (models.Player, error) {
	if id, err := r.repos.ExternalRef(ctx, "player", provider, local); err == nil {
		if existing, err := r.repos.GetPlayer(ctx, id); err == nil {
			return enrich(existing, p), nil
		}
		p.ID = id
		return p, nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return models.Player{}, err
	}

	row := p
	row.ID = provider + "-" + local
	if club != "" {
		squad, err := r.squad(ctx, club)
		if err != nil {
			return models.Player{}, err
		}
		if existing, ok := matchPlayer(p, squad, taken); ok {
			row = enrich(existing, p)
		}
	}
	if err := r.repos.PutExternalRef(ctx, "player", row.ID, provider, local); err != nil {
		return models.Player{}, err
	}
	return row, nil
}

func (r *resolver) squad(ctx context.Context, club string) ([]models.Player, error) {
	if s, ok := r.squads[club]; ok {
		return s, nil
	}
	s, err := r.repos.TeamPlayers(ctx, club)
	if err != nil {
		return nil, err
	}
	r.squads[club] = s
	return s, nil
}

// matchPlayer finds a squad member with the same name, or failing that
// the one squad member whose name is a word-subset of it.
func matchPlayer(p models.Player, squad []models.Player, taken map[string]bool) (models.Player, bool) {
	name := normalizePerson(p.Name)
	var exact, loose []models.Player
	for _, s := range squad {
		if taken[s.ID] {
			continue
		}
		sn := normalizePerson(s.Name)
		switch {
		case sn == name:
			exact = append(exact, s)
		case subset(sn, name):
			loose = append(loose, s)
		}
	}
	if len(exact) == 1 {
		return exact[0], true
	}
	if len(exact) == 0 && len(loose) == 1 {
		return loose[0], true
	}
	return models.Player{}, false
}

// enrich keeps a cached player's identity and fills in only what it was
// missing — a birth date or nationality another provider didn't supply.
func enrich(existing, fetched models.Player) models.Player {
	if existing.DateOfBirth == nil {
		existing.DateOfBirth = fetched.DateOfBirth
	}
	if existing.Nationality == "" {
		existing.Nationality = fetched.Nationality
	}
	if existing.Position == "" {
		existing.Position = fetched.Position
	}
	return existing
}
