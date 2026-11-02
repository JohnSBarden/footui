# Data model

Canonical Go types live in `internal/data/models` (package `models`). Every
provider maps its own source-specific shape into these types; the SQLite
schema (`internal/storage/sqlite/migrations/`) is their
persisted form; nothing outside `internal/data/provider/*` ever sees a
provider-specific type. See [architecture.md](architecture.md) for how these
move between layers.

## Entity overview

```
Competition ─┬─< Season ─┬─< TeamCompetitionSeason >─┬─ Team
             │            │                            │
             │            └─< TeamSeasonStats ─────────┘
             │            └─< PlayerSeasonStats ──────── Player
             │            └─< Match >── home, away ───── Team (national)
             │                  └─< MatchTeamStats
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

National teams take the same idea one step further, to **the match**. A
nation's year is a dozen matches across half a dozen competitions, and
which of them count (friendlies? qualifiers?) is the user's choice, so
every match is stored and a nation's record is computed from the matches a
filter keeps, on read. Clubs keep their season lines, and their games go
in the same `match` table too (schema 0009): a club's recent games are its
last matches in every competition — league, cups, Europe — read straight
from it.

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
| `Category` | string | Empty for a club league. International ones are `tournament` (World Cup, Euros, Copa América, AFCON, …), `qualifier`, `nations-league` or `friendly` — what a nation's filter presets select on (schema 0008). A domestic cup is `cup`: a source of clubs' games, never browsed or given a table, and not international (`Competition.Cup()`). |
| `ShortName` | string | A compact label for tight columns: `WC`, `WCQ`, `UNL`, `FRI`, and for clubs `PL`, `UCL`, `FAC`. Stored with the competition; a row cached before club leagues had one gets it from the coverage registry when shown. |

An international competition's season is an **edition**, keyed by the year
it is named for — Euro 2024 is `2024`, the 2024-25 Nations League is
`2024`, and Euro 2020, played in 2021, is `2020`. Tournaments and
friendlies display as one year, qualifiers and nations leagues as a split
season. Tournaments list their editions explicitly in `sync.Coverages`,
since they come every two or four years, and irregularly.

`sync.Coverages` (`internal/data/sync/scope.go`) is the authoritative list of
what can be synced: each competition, the provider that serves it, and how
far back that provider goes. `footui competitions` prints it.

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
| `National` | bool | A national team. Its ID is `nat-{slug}`, or `nat-{slug}-women` for a women's side (whose name is just the country's), so it can never collide with a club's country-prefixed ID. Schema 0008. |

**Identity:** providers return their own IDs (`understat:eng-arsenal`,
`espn:359`). Before anything is written, `internal/data/sync/identity.go`
maps each to a canonical team by normalized name within country and
gender, and records the mapping in `external_ref` so later refreshes never
re-guess. A club it can't confidently match becomes a new team rather
than being merged into the wrong one. understat's IDs were the first
scheme, so an understat club that matches nothing keeps its own slug.

A national team is resolved the same way but only ever against other
national teams of its gender, by exact name — never against a club, so no
club called "Arcadia" can collect a nation's matches. ESPN's directory of
nations is stored on first use, so search finds every nation before any of
its matches are fetched.

## TeamSeasonStats

The per-`(team, competition, season)` stat line — the "not blended across
competitions" row.

| Field | Type | understat.com | ESPN |
|---|---|---|---|
| `Position` | `*int` | **Computed by footui**: teams sorted by points → goal difference → goals for. Head-to-head tie-breaks aren't modeled, so treat it as an approximation. | Same rule; a club table split into groups (MLS conferences) is ranked as one. A tournament's groups keep ESPN's rank within each group. |
| `GroupLabel` | string | Not populated. | A tournament table row's group ("Group B"), empty for one table. Schema 0008. |
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
| `TeamID` | string | understat.com reports a comma-joined `team_title` for a player who changed clubs mid-season within a league (e.g. `"Barcelona,Atletico Madrid"`), with one aggregate line. footui fetches that player's `getPlayerData/{id}` (one request per mover, cached for every season) and stores one line per club from its `groups.season`. If the page can't be fetched or doesn't split cleanly, the aggregate is kept under the last-listed club (`understat.primaryTeamTitle`). Tested in `internal/data/provider/understat/movers_test.go`. |
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

## PlayerSeasonDetail

The rest of a player's season for one club in one competition — what a
position is judged on beyond goals and assists. Same
`(player, team, competition, season)` key as `PlayerSeasonStats`
(schema 0010, shots on target in 0011), but fetched from ESPN per player,
on demand: when someone opens the player, and on launch for starred ones.
It exists only for those players, so it never feeds squad or league tables.

| Field | Type | Notes |
|---|---|---|
| `Position` | string | `"G"`, `"D"`, `"M"`, `"F"`, or `""`. Picks the Player screen's tiles. |
| `Appearances`, `Minutes` | `*int` | ESPN's detail minutes, where the season stats have none. |
| `CleanSheets`, `GoalsConceded`, `Saves` | `*int` | Keepers and defenders. |
| `Tackles`, `Interceptions`, `Clearances` | `*int` | |
| `ShotsOnTarget` | `*int` | |
| `PassPct` | `*float64` | Share of passes completed, 0 to 1. |
| `FetchedAt` | time | A past season is fetched once; the current one again once stale. |

Every stat is nullable: nil is "not recorded", rendered `—`, because a
source's zero for a season it barely covered is not a real zero.

## Player

| Field | Type | Notes |
|---|---|---|
| `ID` | string | `"{provider}-{provider's id}"`, e.g. `"understat-8260"`, from whichever provider first reported the player. A player later reached through the other provider is matched within their club's squad and keeps this ID. |
| `Name` | string | |
| `DateOfBirth` | `*time.Time` | ESPN only. A player is never matched to one born on another day. |
| `Nationality` | string | ESPN only; shown on the Player screen. |
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

`external_ref` maps each canonical team, player and match to every
provider ID it has been reported under (see Identity above). `sync_log` has
one row per refresh attempt — provider, timing, row counts, and the error
if it failed — and drives the freshness shown on the League, Team and
catalog screens, the stale check on open, and `footui refresh --max-age`.

A row is either a competition-season (`entity_type = 'competition_season'`,
`season_id` set) or, for international matches, one calendar year of a
competition's matches (`'competition_year'`, the `year` column set, schema
0008). The two never stand in for each other. Year rows are what the
politeness rules read: a year fetched after it ended is complete and never
fetched again. A refusal for asking too often is logged with status
`rate_limited`, which suspends automatic refreshes for an hour.

## Match

One game, the grain national teams are tracked at (schema 0008) and clubs'
recent games come from (schema 0009). ESPN's matches are fetched a
competition-year at a time — every nation's matches in, say, the 2025
friendlies, or every club's in the 2025 FA Cup, in one request; understat's
league matches come with the league season's payload. All are upserted by
ID, never deleted: a rescheduled fixture moves, a played one gets its
score.

| Field | Type | Notes |
|---|---|---|
| `ID` | string | Canonical, `espn-{event id}` or `understat-{match id}`, recorded in `external_ref` like a team. |
| `CompetitionID`, `SeasonID` | string | The season is the **edition** ESPN files the match under, not the year it was fetched for: AFCON 2025's January matches are in `afcon:2025`. |
| `Kickoff` | `time.Time` | UTC, stored as RFC3339 text. A nation's years are calendar years of kickoff. |
| `Status` | string | `scheduled`, `in_play`, `ft`, `aet`, `pens`, `postponed`, `cancelled`. Only `ft`/`aet`/`pens` count in a record. |
| `Round` | string | A display label: the group ("Group B") or the stage ("Quarterfinal"); empty for a friendly. |
| `HomeTeamID`, `AwayTeamID` | string | Canonical team IDs. |
| `Neutral` | bool | ESPN's neutral-site flag where it sends one; otherwise, for internationals, played in a country neither side is from (from the venue). A club match without the flag is not neutral. |
| `HomeScore`, `AwayScore` | `*int` | Nil until it starts. After a shoot-out, the score after extra time. |
| `HomePens`, `AwayPens` | `*int` | The shoot-out, for a `pens` match. |
| `HomeStats`, `AwayStats` | `*MatchTeamStats` | Nil when the source had none for that side. |

`MatchTeamStats` (`match_team_stats`, keyed by match and team) holds
possession, shots, shots on target, corners and fouls from ESPN, and xG
from understat (schema 0009), each nullable. As
with club seasons, impossible figures are dropped rather than stored
(possession outside 0–100, more shots on target than shots), a side
reporting nothing but zeroes has no stats, and a later fetch that lacks a
figure keeps the earlier one.

`SummarizeMatches` (in `models`) is the record over a set of matches: P W D
L, goals, form (the last five, oldest first), and averages that count only
the matches that have the stat — "61% possession" from the two matches that
reported it, not diluted by the one that didn't. A shoot-out counts as a
draw, as FIFA records it; an aet result counts by its score.

## MatchDetail

A club's **last game** in full (schema 0015): its events, every team stat,
both lineups and the commentary, from ESPN's per-match summary. One large
response per match, so it is fetched only for the last finished game of a
club being refreshed, never for a whole season. A detail fetched after full
time (`final`) is kept for good; one that isn't is fetched again. understat
league games have no ESPN event ID, so the first fetch finds it from that
day's ESPN scoreboard (matching the two teams) and records it in
`external_ref`.

`match_detail` (one row per match; its presence means "fetched") holds the
venue, attendance, both formations and `fetched_at`. The rest hang off it
and are replaced whole on each fetch; sides are `home`/`away` rather than
team IDs, since the `match` row already says who they are.

| Table | Rows | Notes |
|---|---|---|
| `match_detail_stats` | one per side | Possession and 19 counts: shots, on target, blocked, passes and accurate passes, crosses, long balls, tackles and tackles won, interceptions, clearances, saves, offsides, corners, fouls, cards. Each nullable. Its own table because the scoreboards rewrite `match_team_stats`. |
| `match_event` | in match order | `goal`, `own_goal`, `penalty_goal`, `penalty_miss`, `yellow`, `second_yellow`, `red`, `sub`, `half_time`, `full_time`, with period, minute and stoppage minute. `side` is the side it counts for, so an own goal's scorer plays for the other side. `other` is the assist, or the player going off. Kick-offs and injury delays are dropped. |
| `match_lineup` | per side, starters by formation place then the bench | Shirt, position, minutes on and off, goals, assists, own goals, cards, saves. |
| `match_commentary` | in match order | The clock as footui writes it (`45+2'`), the play (`Goal`, `Foul`) or empty for narration, and the text. |

Player IDs in events and lineups are canonical when footui already knows the
player through `external_ref`, and empty otherwise.

## team_match_filter

The competition filter last used on each nation: a preset (`all`,
`competitive` — tournaments, qualifiers and nations leagues — or
`tournaments`), or an explicit JSON list of competition IDs. No row means
every match. The nation screen and its dashboard card both read it, so they
always agree.

## favorite

The leagues, clubs and players the user has starred (schema 0007), keyed
by `(entity_type, entity_id)` with `entity_type` one of `competition`,
`team` or `player`. IDs are canonical, so a favourite follows a club
across providers. There are deliberately no foreign keys: a favourite
whose entity isn't cached any more stays, and the dashboard shows it as
not cached so it can still be un-starred. `added_at` orders cards within
each kind on the dashboard.

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
