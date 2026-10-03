# Data model

Canonical Go types live in `internal/data/models` (package `models`). Every
provider maps its own source-specific shape into these types; the SQLite
schema (`internal/storage/sqlite/migrations/0001_init.sql`) is their
persisted form; nothing outside `internal/data/provider/*` ever sees a
provider-specific type. See [architecture.md](architecture.md) for how these
move between layers.

## Entity overview

```
Competition ─┬─< Season ─┬─< TeamCompetitionSeason >─┬─ Team
             │            │                            │
             │            └─< TeamSeasonStats ─────────┘
             │            └─< PlayerSeasonStats ──────── Player
             └─< Trophy ──────────────────────────────── Team
                                                          Team ─< TeamColors
```

The load-bearing design decision: **every stats row keys on
(team, competition, season) or (player, team, competition, season) — never
just (team, season) or (player, season)**. That's what keeps a team's
Premier League numbers separate from its Champions League numbers in the
same year. `TeamCompetitionSeason` is the
anchor/join table this correctness property is built on; `TeamSeasonStats`
and `PlayerSeasonStats` both carry the full `(team, competition, season)`
key directly rather than joining through a season-only row.

## Competition

| Field | Type | Notes |
|---|---|---|
| `ID` | string | Slug, e.g. `"eng-premier-league"`. Primary key everywhere else references this. |
| `Name` | string | Display name, e.g. `"Premier League"`. |
| `Country` | string | `"England"`, `"USA"`, etc. |
| `Tier` | int | `1` = top flight, `2` = second division. |
| `Gender` | string | `"men"` \| `"women"`. |
| `Format` | string | `"league"` (default) or `"cup"`. |

| `CalendarYear` | bool | Seasons fall within one calendar year (MLS, NWSL), so the 2026 season displays as `2026` rather than `2026-27`. |

`sync.Coverages` (`internal/data/sync/scope.go`) is the authoritative list of
what can be synced: each competition, the provider that serves it, and how
far back that provider goes. `footy competitions` prints it.

## Season

| Field | Type | Notes |
|---|---|---|
| `ID` | string | `"{competition_id}:{label}"`, e.g. `"eng-premier-league:2025"`. |
| `CompetitionID` | string | FK to `Competition.ID`. |
| `Label` | string | The bare start year (e.g. `"2025"` for 2025-26) for every provider — **a key, not a display string**. Formatting it as "2025-26" or "2025" is the rendering layer's job, using the competition's `CalendarYear`. |
| `StartDate` / `EndDate` | `*time.Time` | Not populated; neither provider's payload carries season boundaries. |

## Team

| Field | Type | Notes |
|---|---|---|
| `ID` | string | Canonical slug, e.g. `"eng-manchester-city"` — `{country-prefix}-{slugified name}`, `-women` appended for a women's side, `int-` for a European club of unknown country. Minted by `sync`'s identity resolver, never by a provider. |
| `Name` | string | |
| `ShortName` | string | From ESPN; understat.com has no short-name field. |
| `Country` | string | |
| `Gender` | string | Men's and women's sides of the same club get **separate `Team` rows** with different IDs — they play in different competitions and have independent squads/history. |
| `FoundedYear` | int | Not populated yet. |
| `Venue` | string | Not populated yet. |

**Identity:** providers return their own IDs (`understat:eng-arsenal`,
`espn:359`). Before anything is written, `internal/data/sync/identity.go`
maps each to a canonical team by normalized name within country and
gender, and records the mapping in `external_ref` so later refreshes never
re-guess. A club it can't confidently match becomes a new team rather
than being merged into the wrong one. understat's IDs were the first
scheme, so an understat club that matches nothing keeps its own slug.

## TeamSeasonStats

The per-`(team, competition, season)` stat line — the "not blended across
competitions" row.

| Field | Type | understat.com | ESPN |
|---|---|---|---|
| `Position` | `*int` | **Computed by footui**: teams sorted by points → goal difference → goals for. Head-to-head tie-breaks aren't modeled, so treat it as an approximation. | Same rule; a table split into groups (MLS conferences) is ranked as one. |
| `MatchesPlayed`, `Wins`, `Draws`, `Losses`, `GoalsFor`, `GoalsAgainst`, `GoalDiff`, `Points` | int | Summed from the per-match `history` array. | From the standings payload. |
| `PossessionAvg` | `*float64` | No. | Yes, when plausible. |
| `FoulsCommitted`, `FoulsSuffered`, `YellowCards`, `RedCards`, `Shots`, `ShotsOnTarget`, `Corners` | `*int` | No. | Yes, each checked against matches played; an impossible figure is left `nil`. |
| `XG`, `XGA` | `*float64` | Summed from per-match `xG`/`xGA`. | No. |
| `Extras` | `map[string]float64` | The long-tail stats below. | None. |
| `Source` | string | `"understat"` | `"espn"` |
| `UpdatedAt` | `time.Time` | Set at sync time; stored as RFC3339 text (see the SQLite-driver caveat below). |

**Merge behavior:** `Store.UpsertTeamSeasonStats` uses `COALESCE(excluded.x,
existing.x)` for every nullable stat column — a provider that only
supplies possession/fouls can upsert into an existing row without
clobbering the xG a prior sync already wrote, and vice versa. This is the
concrete mechanism behind the "provider output is merged, not overwritten"
rule from the architecture doc. `Extras` merges the same way at key
granularity: writing a map with two keys updates those two and leaves any
other keys on that team-season untouched.

### Long-tail stats (`Extras` → `team_season_stat_extra`)

Stats that don't warrant a column of their own live in the key/value
`team_season_stat_extra` table. Key names are declared as `models.Stat*`
constants rather than in a provider package, because storage and the TUI
both read them: a provider chooses which of these it can supply, but not
what they are called.

| Key | Constant | Meaning | Aggregation |
|---|---|---|---|
| `npxg` / `npxga` | `StatNPxG` / `StatNPxGA` | Expected goals (for/against) with penalties excluded — a better read on open-play quality than raw xG. | Summed over matches. |
| `ppda` | `StatPPDA` | Opponent passes allowed per defensive action. **Lower means a more intense press.** | **Ratio of sums**, not a mean of per-match ratios — `sum(att) / sum(def)`. Averaging the ratios would weight a quiet match as heavily as a frantic one and give a different, wrong number. Regression-tested in `advanced_test.go`. |
| `ppda_allowed` | `StatPPDAAllowed` | The same measure applied to the opponent's press. | Same ratio-of-sums rule. |
| `deep` / `deep_allowed` | `StatDeep` / `StatDeepAllowed` | Completed passes within roughly 20 yards of goal, excluding crosses. | Summed. |
| `xpts` | `StatExpectedPoints` | Points the team would be expected to have taken given each match's chances. | Summed. Displayed against actual points, which is the clearest single measure of whether a season flattered a team or robbed it. |

A team with no matches played gets **no** extras rather than a set of
zeroes: "played nothing yet" and "pressed nobody" are different claims. For
the same reason `ppda` is omitted entirely when a team recorded no
defensive actions, rather than emitting a division by zero.

## PlayerSeasonStats

Same per-competition-per-season grain as `TeamSeasonStats`, keyed by
`(player, team, competition, season)`.

| Field | Type | Notes |
|---|---|---|
| `PlayerID` | string | Canonical; see Player. |
| `TeamID` | string | **Caveat:** understat.com reports a comma-joined `team_title` for a player who changed clubs mid-season (e.g. `"Bournemouth,Manchester City"`). footui attributes the row to the **last-listed (most recent) club only** (`understat.primaryTeamTitle`) — the stat line itself is still the player's full-season aggregate across every club, not split per club. A transferred player's numbers will look inflated for whichever club they're attributed to. Regression-tested in `internal/data/provider/understat/mapper_test.go`. |
| `Appearances`, `Goals`, `Assists`, `YellowCards`, `RedCards` | int | Both providers. |
| `Minutes` | int | understat.com only; ESPN reports none. |
| `XG`, `XA` | `*float64` | understat.com only. |
| `Shots`, `KeyPasses`, `NonPenaltyGoals` | `*int` | Shots from both; the rest understat.com only. Nullable, and nil means "the provider didn't report this" — distinct from a genuine zero, which the TUI renders differently (`—` vs `0`). |
| `NPxG` | `*float64` | Non-penalty expected goals. Same nullable semantics. |

These four are real columns rather than key/value extras (added in
`0002_advanced_stats.sql`) because every plausible provider has them: they
are core advanced stats, not a source-specific long tail. The same
migration adds an index on `(team_id, competition_id, season_id)`, which
the roster query needs and the table's `player_id`-first primary key can't
serve.

## Player

| Field | Type | Notes |
|---|---|---|
| `ID` | string | `"{provider}-{provider's id}"`, e.g. `"understat-8260"`, from whichever provider first reported the player. A player later reached through the other provider is matched within their club's squad and keeps this ID. |
| `Name` | string | |
| `DateOfBirth` | `*time.Time` | Not populated. |
| `Nationality` | string | Not populated. |
| `Position` | string | The provider's position string, e.g. `"F M"`. |
| `Gender` | string | The competition's gender. |

## Name search

`team`, `player`, and `competition` each carry a `name_folded` column: a
lowercase, accent-stripped copy of the name, written by
`internal/textfold.Fold` and never displayed.

It exists because SQLite's `LIKE` is case-insensitive only for ASCII and
has no notion of accent equivalence. Football names are full of diacritics
and people search from keyboards that don't have them, so without it
`martin` would not find "Martín Zubimendi" and `jurrien` would find nothing
for "Jurriën Timber". Folding both sides of the comparison fixes both
directions without asking the user to type characters they may not have
keys for.

Folding is Unicode canonical decomposition (NFD) followed by dropping
combining marks, which covers the whole Latin range without a
hand-maintained character table, and lowercases everything else — so
non-Latin scripts stay searchable rather than being mangled.

`0004_folded_names.sql` adds the columns, but cannot populate them: SQLite
has no normalization function. `sqlite.Open` backfills any NULL rows
instead, so a database created before the columns existed becomes
searchable on the next open rather than only after the user happens to
refresh. A silently unsearchable name would be hard to notice — the query
succeeds and simply returns nothing.

## Trophy, TeamColors

Both come from hand-curated seed files (`internal/trophies/seed/`,
`internal/colors/seed/`), applied after every refresh to the clubs actually
cached. ESPN also supplies a primary color for the clubs it serves; a
curated entry overrides it.

## external_ref, sync_log

`external_ref` maps each canonical team and player to every provider ID it
has been reported under (see Identity above). `sync_log` has one row per
competition-season refresh attempt — provider, timing, row counts, and the
error if it failed — and drives the freshness shown on the League, Team and
catalog screens, the stale check on open, and `footy refresh --max-age`.

## favorite

The leagues, clubs and players the user has starred (schema 0007), keyed
by `(entity_type, entity_id)` with `entity_type` one of `competition`,
`team` or `player`. IDs are canonical, so a favourite follows a club
across providers. There are deliberately no foreign keys: a favourite
whose entity isn't cached any more stays, and the dashboard shows it as
not cached so it can still be un-starred. `added_at` orders cards within
each dashboard group.

## A SQLite driver caveat worth knowing

`modernc.org/sqlite` stores `time.Time` values as TEXT but does not scan
TEXT columns back into `*time.Time` — `rows.Scan(&someTimeField)` fails
with `unsupported Scan ... storing driver.Value type string into type
*time.Time`. `internal/storage/sqlite/store.go` works around this by
formatting timestamps as RFC3339 strings explicitly on write
(`formatTime`) and parsing them explicitly on read (`parseTime`), rather
than passing `time.Time` values straight through `database/sql`. Tests that
round-trip through real SQLite (`internal/storage/sqlite`,
`internal/data/sync`) guard this; keep them when touching upsert or scan
paths.
