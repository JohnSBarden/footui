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
 │ provider/*           │        │ internal/storage/sqlite │        │ internal/tui (WIP)   │
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
contract has one implementation (`internal/data/provider/understat`),
tested against a fixture, a fake source, and live data. The rendering side
is the full TUI — Browse, League, Team, Player — and it reaches data only
through `storage.Repositories`, never through a provider or the database
directly.

The boundaries earned their keep: adding four screens required no changes
to the provider or sync layers, only additional query methods on the
storage contract.

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

// Bulk-fetch counterparts, added during implementation once it became
// clear a full sync needs "every team in this league" and "every player
// in this league" as first-class calls, not just per-team lookups:

type TeamsProvider interface {
    Teams(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.Team, error)
}

type LeaguePlayersProvider interface {
    Players(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.Player, error)
    AllRosterStats(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.PlayerSeasonStats, error)
}
```

**Current implementation:** `internal/data/provider/understat` (package
`understat`), satisfying `StandingsProvider`, `XGProvider`,
`PlayerStatsProvider`, `TeamsProvider`, and `LeaguePlayersProvider` — not
`TeamStatsProvider` (understat.com has no possession/fouls/cards/shots
data) or `TrophyProvider`/`TeamResolver` (not implemented yet).

**Why understat.com and not FBref, per the original design doc:** FBref
sits behind a Cloudflare JS challenge that blocks plain HTTP requests
(confirmed: even `curl` against the FBref homepage returns a 403 challenge
page, not real content), and the environment this was built in has no
headless-browser access to clear it. understat.com serves the same class
of match/season data as a fetchable JSON endpoint
(`GET /getLeagueData/{league}/{season}`, reverse-engineered from its own
frontend JS — see `internal/data/provider/understat/client.go`). Swapping
in an FBref provider (or a paid API) later means writing a new package that
implements the same interfaces; nothing in `internal/data/sync` or above
needs to change.

**Coverage gap this creates**, tracked in `internal/data/sync/scope.go`:
understat.com covers 6 men's top-flight leagues only (England, Spain,
Germany, Italy, France, Russia) — no second divisions, no MLS/USL, no
women's leagues, no continental competitions, and no fouls/possession/
corners at all. This is a real, known gap versus the full v1 target in the
design doc, not a hidden one. `provider.TeamsProvider`/
`LeaguePlayersProvider`/etc. being separate interfaces from
`TeamStatsProvider` is exactly what makes filling that gap additive: a
future FBref-backed `TeamStatsProvider` implementation can supply
possession/fouls for the same teams understat.com already covers, merged
in by `internal/data/sync` rather than replacing anything.

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

Club themes come in two modes (`styles.Mode`). Weak uses club colors as
foreground accents and leaves the terminal's background alone. Strong
(`styles.StrongTeamTheme`) solves a palette against the club's primary as
the background, and the root app paints the whole frame in it with
`styles.Paint`. A themed screen exposes its theme through
`core.ThemedScreen` so the chrome matches. The mode lives in a shared
`*styles.Appearance` so the `t` toggle reaches every screen.

### How screens are structured

```
internal/tui/
├── app.go            # the navigation stack, global keys, chrome
├── core/             # the rendering contract: Screen, Deps, Navigator, nav messages
├── keys/             # every key binding, paired with its help text
├── styles/           # the theme, and team-brand theming with contrast adjustment
├── components/       # table, stat cards, tabs, freshness, formatting
└── screens/{browse,catalog,league,team,player}/
```

Three rules make this hold together:

1. **Screens never import each other.** Drill-down targets arrive as
   constructors in `core.Navigator`, injected via `core.Deps`. This is what
   lets League open Team *and* Team open League without a cycle. A route
   that isn't wired is a nil field, and screens show a status message
   rather than crashing — so a screen can ship before its drill-down
   target exists.

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
   (from `sync.Scope` — the definitive list of what's syncable) and season
   start years, opens/migrates the SQLite DB, and constructs an
   `understat.Provider` and a `sync.Syncer`.
2. `Syncer.SyncMany` walks every competition-season pair. Each unit is
   independent: a failure is recorded and the run continues, because the
   realistic failure mode across dozens of requests to a third-party
   endpoint is partial, not total. After three *consecutive* failures the
   run gives up and marks the rest skipped rather than paying a full
   network timeout for each.
3. For each unit, `SyncCompetitionSeason` upserts the `Competition` and
   `Season`, then calls the source through the `sync.Source` interface (the
   subset of the provider contract a full sync needs: `StandingsProvider` +
   `TeamsProvider` + `LeaguePlayersProvider`) for teams, standings — with
   xG and the long-tail stats already merged in, since understat.com
   returns them in the same payload — and every player's season stats.
   Each `models.*` value is upserted through `storage.Repositories`.
4. Every attempt, successful or not, is written to `sync_log`. Recording
   failures is the point: "we tried an hour ago and it broke" is a
   different fact from "we haven't tried since Tuesday", and the League
   screen's freshness indicator distinguishes them.
5. Finally the curated club colors and honours are re-applied to whatever
   is now cached. Neither can fail the refresh — the stats are the point.

One HTTP round trip per league/season, cached in-process so the standings,
team, and roster calls that hit the same endpoint don't re-fetch, and
optionally cached on disk via `FOOTY_HTTP_CACHE` during development.

## Curated datasets

Two things have no scrapeable source and are hand-maintained:
`internal/colors/seed/teamcolors.yaml` and
`internal/trophies/seed/trophies.yaml`. Both follow the same rule: seeding
is driven by **what is cached**, not by the file. An entry applies only
when its ID — or one of its aliases, since team IDs are slugs of however
the provider spells a club's name — matches a team that actually exists in
the database. A wrong guess is therefore inert rather than harmful, and
`footy colors` reports which cached clubs are still uncovered.
