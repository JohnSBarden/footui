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

Status as of this writing: boundaries (1) source→backend are fully built
and exercised end-to-end (see `internal/data/sync`, tested against both a
fake source and live understat.com data). Boundary (2) backend→rendering
exists today only as `footy refresh`/`footy competitions` reading and
writing through `storage.Repositories`; the interactive TUI screens
described in the original design doc haven't been built yet, but they will
consume the exact same `storage.Repositories` interface — no new backend
work is implied by adding them.

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

## Boundary 3 — Rendering (CLI today, TUI in a later phase)

`internal/cli` commands (`footy refresh`, `footy competitions`) construct a
`sqlite.Store` and a provider, wire them into a `sync.Syncer`, and print
results — see `internal/cli/refresh.go`. The planned TUI
(`internal/tui`, per the original design doc's screen breakdown: Browse →
League → Team → Player) will be constructed the same way, taking a
`storage.Repositories` and issuing only its query methods
(`Standings`, `TeamHistory`, `RosterForTeamSeason`, etc.) — no screen will
import `internal/data/provider` or `internal/storage/sqlite` directly.

## Data flow for `footy refresh --competition eng-premier-league --season 2025`

1. `internal/cli/refresh.go` looks up the competition in
   `sync.Scope` (`internal/data/sync/scope.go` — the definitive list of
   what's currently syncable), opens/migrates the SQLite DB
   (`sqlite.Open`), and constructs an `understat.Provider` and a
   `sync.Syncer`.
2. `Syncer.SyncCompetitionSeason` upserts the `Competition`/`Season` rows,
   then calls the source through the `sync.Source` interface (the subset of
   the provider contract a full sync needs: `StandingsProvider` +
   `TeamsProvider` + `LeaguePlayersProvider`) to fetch teams, standings
   (with xG merged in already, since understat.com returns it in the same
   payload), and every player's season stats.
3. Each fetched `models.*` value is upserted through `storage.Repositories`
   — one HTTP round trip to understat.com (cached in-process per
   league/season so the standings, xG, and roster calls that hit the same
   endpoint don't re-fetch), then a batch of idempotent SQLite upserts.
