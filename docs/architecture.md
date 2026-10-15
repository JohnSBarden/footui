# Architecture: the three contract boundaries

footui is built around three explicit interface boundaries so that the data
source, the storage/processing backend, and the TUI can each change without
the others knowing:

```
 ┌────────────────────┐        ┌──────────────────────┐        ┌───────────────────┐
 │   DATA SOURCE       │        │   BACKEND             │        │   RENDERING         │
 │   (a provider)      │  (1)   │   (SQLite store)       │  (2)   │   (CLI + TUI)        │
 │                      │ ────▶ │                        │ ────▶ │                      │
 │ internal/data/       │        │ internal/storage       │        │ internal/cli,        │
 │ provider/*           │        │ internal/storage/sqlite │        │ internal/tui         │
 └────────────────────┘        └──────────────────────┘        └───────────────────┘
        implements                    implements                    consumes only
   internal/data/provider          internal/storage.               internal/storage.
   interfaces, returns              Repositories,                   Repositories'
   internal/data/models             persists                        query methods
   values only                      internal/data/models
```

Nothing reaches across a boundary directly: a provider never touches SQL, the
sync layer never touches a provider's raw JSON/HTML shapes, and the CLI/TUI
never open the database or call a provider directly. Everything that crosses
a boundary is a value from `internal/data/models` (see
[data-model.md](data-model.md)) or a small `*Ref` struct
(`provider.CompetitionRef`, `provider.SeasonRef`, `provider.TeamRef`,
`provider.PlayerRef`).

Status: both boundaries are built and exercised end to end. The source
contract has two implementations: `internal/data/provider/understat` (the
six leagues with xG) and `internal/data/provider/espn` (MLS, the women's
leagues, the second divisions, the European competitions, and
international football). Each
competition is routed to one of them by `sync.Coverages`. Both providers
return their own IDs (`understat:eng-arsenal`, `espn:359`). Before anything
is written, the sync layer resolves those to canonical team and player IDs
(`internal/data/sync/identity.go`) and records each mapping in
`external_ref`, so a club reached through both providers is one entity.
understat's IDs were the original canonical scheme, so a club only
understat knows keeps exactly the ID it always had. The rendering side
is the TUI — the favourites dashboard, Browse, League, Team, Nation,
Player, and the league catalog — and it
reaches data only through `storage.Repositories`, never through a provider
or the database directly.

## Boundary 1 — Source contract (`internal/data/provider`)

Small, domain-scoped interfaces. A provider implements only the ones its
source can actually supply:

```go
type StandingsProvider interface {
    Standings(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.TeamSeasonStats, error)
}

type TeamStatsProvider interface {
    TeamSeasonStats(ctx context.Context, comp CompetitionRef, season SeasonRef, team TeamRef) (models.TeamSeasonStats, error)
}

type XGProvider interface {
    TeamXG(ctx context.Context, comp CompetitionRef, season SeasonRef, team TeamRef) (xg, xga float64, err error)
    PlayerXG(ctx context.Context, comp CompetitionRef, season SeasonRef, player PlayerRef) (xg, xa float64, err error)
}

type PlayerStatsProvider interface {
    RosterStats(ctx context.Context, comp CompetitionRef, season SeasonRef, team TeamRef) ([]models.PlayerSeasonStats, error)
}

type TrophyProvider interface {
    TeamTrophies(ctx context.Context, team TeamRef) ([]models.Trophy, error)
}

type TeamResolver interface {
    ResolveTeam(ctx context.Context, name string) ([]TeamRef, error)
}

// Bulk-fetch counterparts: a full sync needs "every team in this league"
// and "every player in this league" as single calls, not per-team lookups.

type TeamsProvider interface {
    Teams(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.Team, error)
}

type LeaguePlayersProvider interface {
    Players(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.Player, error)
    AllRosterStats(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.PlayerSeasonStats, error)
}

// International football is fetched by the match, not the season.

type NationalTeamsProvider interface {
    NationalTeams(ctx context.Context, gender string) ([]models.Team, error)
}

// CompetitionMatches returns every match of comp in one calendar year,
// every nation's, with per-team stats where the source has them, and the
// teams that played them.
type MatchesProvider interface {
    CompetitionMatches(ctx context.Context, comp CompetitionRef, year int) ([]models.Match, []models.Team, error)
}
```

Three sentinel errors cross the boundary: `provider.ErrRateLimited` (the
source refused footy and kept refusing — stop), `provider.ErrIncomplete`
(data returned alongside it is known to be partial — store it, don't call
it complete), and `provider.ErrNoTable` (no table for that season).

**Implementations:**

- `internal/data/provider/understat` serves the six leagues with expected
  goals. It reads understat.com's JSON endpoint
  (`GET /getLeagueData/{league}/{season}`, the one its own frontend uses —
  see `client.go`), and implements `StandingsProvider`, `XGProvider`,
  `PlayerStatsProvider`, `TeamsProvider`, `LeaguePlayersProvider`, and
  `SeasonMatchesProvider` — the season's matches with each side's xG, from
  the same payload, so they cost no request of their own.
- `internal/data/provider/espn` serves everything else: MLS and USL, the
  second divisions, the women's leagues, the European competitions, and
  international football. It has possession, shots, corners, fouls and
  cards, but no expected goals. It implements `MatchesProvider` from the
  scoreboard endpoint — for internationals, and for clubs' games in its
  leagues, the European competitions and seven domestic cups — `NationalTeamsProvider`
  from the friendlies' (and women's tournaments') team lists, and
  `StandingsProvider` with group labels kept — one request, no rosters.

FBref would cover more, but it sits behind a Cloudflare JS challenge that
plain HTTP requests can't clear. Adding a source means writing a package
that implements these interfaces and adding its competitions to
`sync.Coverages`; nothing in `internal/data/sync` or above changes. Because
the interfaces are small, a provider that supplies only some stats for
teams another already covers is additive: its rows are merged in, not
substituted.

## Boundary 2 — Backend contract (`internal/storage`)

```go
type Repositories interface {
    CompetitionsRepo
    SeasonsRepo
    TeamsRepo
    TeamStatsRepo
    PlayersRepo
    PlayerStatsRepo
}

type CompetitionsRepo interface {
    UpsertCompetition(ctx context.Context, c models.Competition) error
    ListCompetitions(ctx context.Context) ([]models.Competition, error)
}

type TeamStatsRepo interface {
    LinkTeamCompetitionSeason(ctx context.Context, teamID, competitionID, seasonID string) error
    UpsertTeamSeasonStats(ctx context.Context, s models.TeamSeasonStats) error
    Standings(ctx context.Context, competitionID, seasonID string) ([]models.TeamSeasonStats, error)
    TeamHistory(ctx context.Context, teamID string) ([]models.TeamSeasonStats, error)
}

type MatchesRepo interface {
    UpsertMatches(ctx context.Context, matches []models.Match) error
    MatchesForTeam(ctx context.Context, teamID string, filter models.MatchFilter, years []int) ([]models.Match, error)
    MatchesForCompetitionSeason(ctx context.Context, competitionID, seasonID string) ([]models.Match, error)
    TeamMatchSummary(ctx context.Context, teamID string, filter models.MatchFilter, years []int) (models.MatchSummary, error)
    GetTeamMatchFilter(ctx context.Context, teamID string) (models.MatchFilter, error)
    SetTeamMatchFilter(ctx context.Context, teamID string, f models.MatchFilter) error
    // ...plus TeamMatchYears and MatchesKickedOff
}

// ...SeasonsRepo, TeamsRepo, PlayersRepo, PlayerStatsRepo follow the same
// Upsert{Entity}/query shape — see internal/storage/repository.go.
```

`internal/storage/sqlite.Store` is the only implementation: it holds the
`*sql.DB`, and every SQL statement in the codebase lives in
`internal/storage/sqlite/store.go`. The sync layer
(`internal/data/sync.Syncer`) is constructed with a `storage.Repositories`
value, not a `*sqlite.Store`, so `Syncer` can be tested against a fake
store with no real database (see the pattern used the other direction in
`internal/data/sync/syncer_test.go`, which fakes the *source* side against
a real in-memory SQLite store).

## Boundary 3 — Rendering (`internal/cli`, `internal/tui`)

`internal/cli` commands construct a `sqlite.Store` and, for `refresh`, a
provider and a `sync.Syncer`. The TUI is constructed the same way: it
receives a `storage.Repositories` and issues only its query methods
(`Standings`, `TeamHistory`, `RosterForTeamSeason`, `TeamTrophies`, …). No
screen imports `internal/data/provider` or `internal/storage/sqlite`.

The TUI's refresh key is the one place it triggers a write, and it keeps
the boundary by injection: `internal/cli` builds a `core.Refresher` (a
function) from the same pipeline `footy refresh` uses and hands it to the
root app in `core.Deps`. Screens only report which competition a refresh
from them should fetch (`core.Refreshable`) and re-query when told it
landed (`core.RefreshedMsg`).

A refresh request is usually a competition and some seasons, but it can
also name a national team (`TeamID`), every nation of a gender
(`Nations`), or the directory of nations (`Directory`). A screen whose
refresh isn't one competition — the nation screen, a nation's dashboard
card — implements `core.RefreshRequester` and hands the app the whole
request. When the refresher returns `core.ErrRateLimited`, the app drops
everything queued and says so, instead of letting each request be refused
in turn.

Every refresh the TUI runs goes through one queue in the root app
(`internal/tui/refresh.go`), whatever triggered it: the refresh key, the
league catalog (which sends a `core.RefreshRequestMsg` naming leagues and
seasons), or the stale check on open. `internal/cli` works out which
followed leagues have a stale current season, using only storage queries
and the sync package's season arithmetic, and hands those to `tui.Run` as
startup requests. The queue runs one request at a time, broadcasts
`core.RefreshQueueMsg` so screens can mark what's in flight, and reports a
batch once it drains. That way an early failure isn't hidden by a later
success.

Messages broadcast to the whole stack can make a buried screen issue a
command, such as Browse re-running its search after a refresh. That
command's reply is wrapped in a `routedMsg` addressed to the screen's
stack id (`internal/tui/route.go`), so it reaches that screen rather than
whichever one is on top.

Keyboard handling is layered so a text box never loses a keystroke and
every action is reachable from every screen. The root app handles global
keys first (`r`, `d`, `t`, `/`, `?`, `q`), each with a `ctrl+` twin that
works while a screen reports `core.TextEntry`. `esc` goes to a screen that
implements `core.Escaper` first, so clearing a search query happens before
navigating back. `/` returns to the screen implementing `core.Searcher`
rather than stacking a new one.

Club themes come in two modes (`styles.Mode`). Weak uses club colors as
foreground accents and leaves the terminal's background alone. Strong
(`styles.StrongTeamTheme`) solves a palette against the club's primary as
the background, and the root app paints the whole frame in it with
`styles.Paint`. A themed screen exposes its theme through
`core.ThemedScreen` so the chrome matches. The mode lives in a shared
`*styles.Appearance` so the `t` toggle reaches every screen, and the
toggle is saved through `core.Deps.SaveTheme` so it survives a restart.

### How screens are structured

```
internal/tui/
├── app.go            # the navigation stack, global keys, chrome
├── core/             # the rendering contract: Screen, Deps, Navigator, nav messages
├── keys/             # every key binding, paired with its help text
├── styles/           # the theme, and team-brand theming with contrast adjustment
├── components/       # table, stat cards, tabs, freshness, formatting
└── screens/{dashboard,browse,catalog,league,team,nation,player}/
```

Three rules make this hold together:

1. **Screens never import each other.** Drill-down targets arrive as
   constructors in `core.Navigator`, injected via `core.Deps`. This is what
   lets League open Team *and* Team open League without a cycle, and lets
   a screen's tests wire only the routes they exercise.

2. **`internal/tui` is the only package that imports the screen packages.**
   It owns the stack, so "back" is always just popping; no screen has to
   remember where it came from or rebuild its parent's state.

3. **Screens declare key bindings, not help text.** The root app renders
   the footer from `Screen.Keys()`, so what the footer promises is what the
   screen actually handles.

Screens are directly testable: `Update` and `View` are ordinary functions.
`internal/tui/tuitest` steps a screen through its load sequence, and
`internal/storage/storagetest` seeds a real in-memory SQLite store — so a
screen test exercises the same SQL the binary runs.

## Data flow for `footy refresh --all --years 3`

1. `internal/cli/refresh.go` resolves the flags into a list of competitions
   (from `sync.Coverages` — the definitive list of what's syncable) and
   season start years, and opens/migrates the SQLite DB.
   `internal/cli/refreshrun.go` groups the work by provider and constructs
   one source and `sync.Syncer` per provider.
2. `Syncer.SyncMany` walks every competition-season pair. Each unit is
   independent: a failure is recorded and the run continues, because the
   realistic failure mode across dozens of requests to a third-party
   endpoint is partial, not total. After three *consecutive* failures the
   run gives up and marks the rest skipped rather than paying a full
   network timeout for each.
3. For each unit, `SyncCompetitionSeason` upserts the `Competition` and
   `Season`, then calls the source through the `sync.Source` interface (the
   subset of the provider contract a full sync needs: `StandingsProvider` +
   `TeamsProvider` + `LeaguePlayersProvider`) for teams, standings, and
   every player's season stats. Provider IDs are resolved to canonical
   ones (`identity.go`), and each `models.*` value is upserted through
   `storage.Repositories`.
4. Every attempt, successful or not, is written to `sync_log`. Recording
   failures is the point: "we tried an hour ago and it broke" is a
   different fact from "we haven't tried since Tuesday", and the freshness
   shown in the TUI distinguishes them.
5. Finally the curated club colors and honours are re-applied to whatever
   is now cached. Neither can fail the refresh — the stats are the point.

One HTTP round trip per league/season, cached in-process so the standings,
team, and roster calls that hit the same endpoint don't re-fetch, and
optionally cached on disk via `FOOTY_HTTP_CACHE` during development.

## International football: the unit, and politeness

A nation's matches can't be fetched per nation without hundreds of
requests, and per-match summaries are 440 KB each. Instead the unit is
**one calendar year of one competition** — ESPN's scoreboard returns every
match of, say, the 2025 friendlies, every nation's, in one request. A
nation's matches are the units of every international competition of its
gender for the years wanted (`sync.NationUnits`), so the first nation
followed costs a request per competition per year — about 25 for this year
and last — and every nation after it costs nothing.

`sync.MatchSyncer` decides per unit whether to fetch at all (`UnitDue`),
from the year rows in `sync_log`:

- never fetched, or a failure more than ten minutes old: fetch
- fetched after its year ended (UTC, plus a day's grace): complete, never
  again
- fetched while its year was still going, and the year has since ended:
  once more
- the current year: only when older than `stale_after` **and** a stored
  match has kicked off since the last fetch (or is in play), or the last
  fetch is a week old — so fixtures announced since aren't missed.
  Outside international windows, that is nothing.

A tournament edition (the catalog, the League screen's `r`) is its named
year's unit, the next year's only if the first held no final (Euro 2020,
AFCON 2025), and one standings request for its groups.

Each match is filed under the edition ESPN gives it, not the year fetched.
A scoreboard response exactly at its `limit` is taken as truncated (ESPN
doesn't say) and refetched as twelve month windows; if a month is full
too, the matches are stored but the unit is logged as failed.

The ESPN client paces scoreboards a second apart (other calls 150 ms). A
429 honours `Retry-After`, else waits 5 s, 20 s, 60 s; one that outlasts
that trips a breaker shared by every client in the process, so for an hour
every request fails at once with `provider.ErrRateLimited`. Sync stops the
run there, the unit is logged `rate_limited`, the TUI drops its queue, and
the stale check on open stands down for the hour.

## A club's recent games

The Team screen's strip and the Games screen read a club's last finished
matches in every competition (`RecentMatches`). They come from two places:

- **Its understat league**, with the season: `Syncer` stores whatever a
  `SeasonMatchesProvider` returns alongside the table. A season cached
  before this has standings but no matches, which is how it is known to be
  due once more (`CountSeasonMatches`).
- **ESPN units**, the same competition-year unit and `UnitDue` rules as
  internationals: the club's ESPN league, Europe's competitions (for a
  club from England, Spain, Germany, Italy or France), and its country's
  cups (men only). `MatchSyncer` resolves a club unit's teams as clubs,
  never as nations, and never links a club to a cup or European season —
  its season lines come from tables. In a cup, a name that merely contains
  a cached club's ("FC United of Manchester") is only matched when the
  words that differ are club suffixes ("Leeds United" = "Leeds").

`sync.PlanClub` works out both for this calendar year and last.
`cli.clubDue` asks it without fetching anything: the Team screen calls it
on open (through `core.Deps.ClubDue`) and queues the club's refresh only
when something is due, and the stale check on open does the same for
starred clubs. So the first English club opened costs about ten requests
and the next none. A season row stored only because a club's matches came
from it doesn't make its competition "followed": the stale check counts
seasons refreshed for their table, and skips cups.

## Curated datasets

Two things have no scrapeable source and are hand-maintained:
`internal/colors/seed/teamcolors.yaml` and
`internal/trophies/seed/trophies.yaml`. Both follow the same rule: seeding
is driven by **what is cached**, not by the file. An entry applies only
when its ID — or one of its aliases, since team IDs are slugs of however
the provider spells a club's name — matches a team that actually exists in
the database. A wrong guess is therefore inert rather than harmful, and
`footy colors` reports which cached clubs are still uncovered.
