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
leagues, the second divisions, and the European competitions). Each
competition is routed to one of them by `sync.Coverages`. Both providers
return their own IDs (`understat:eng-arsenal`, `espn:359`). Before anything
is written, the sync layer resolves those to canonical team and player IDs
(`internal/data/sync/identity.go`) and records each mapping in
`external_ref`, so a club reached through both providers is one entity.
understat's IDs were the original canonical scheme, so a club only
understat knows keeps exactly the ID it always had. The rendering side
is the TUI — Browse, League, Team, Player, and the league catalog — and it
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
```

**Implementations:**

- `internal/data/provider/understat` serves the six leagues with expected
  goals. It reads understat.com's JSON endpoint
  (`GET /getLeagueData/{league}/{season}`, the one its own frontend uses —
  see `client.go`), and implements `StandingsProvider`, `XGProvider`,
  `PlayerStatsProvider`, `TeamsProvider`, and `LeaguePlayersProvider`.
- `internal/data/provider/espn` serves everything else: MLS and USL, the
  second divisions, the women's leagues, and the European competitions.
  It has possession, shots, corners, fouls and cards, but no expected
  goals.

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
└── screens/{browse,catalog,league,team,player}/
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

## Curated datasets

Two things have no scrapeable source and are hand-maintained:
`internal/colors/seed/teamcolors.yaml` and
`internal/trophies/seed/trophies.yaml`. Both follow the same rule: seeding
is driven by **what is cached**, not by the file. An entry applies only
when its ID — or one of its aliases, since team IDs are slugs of however
the provider spells a club's name — matches a team that actually exists in
the database. A wrong guess is therefore inert rather than harmful, and
`footy colors` reports which cached clubs are still uncovered.
