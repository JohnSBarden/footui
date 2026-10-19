# footui

[![CI](https://github.com/JohnSBarden/footui/actions/workflows/ci.yml/badge.svg)](https://github.com/JohnSBarden/footui/actions/workflows/ci.yml)

A terminal dashboard for drillable football statistics.

`footui` fetches league, team, player and match data into a
local SQLite database and lets you drill through it in an interactive TUI:
**Browse → League → Team → Player**, with expected goals, pressing metrics,
squads, honours, and every club's recent games across its league, cups and
Europe. National teams and international tournaments are covered too: every
match a nation plays, friendlies included, filterable down to the
competitions you care about.

```
 Arsenal   England · men
[Premier League (3)]  Champions League (3)    updated 34 minutes ago

Recent  W W L W W  m all games
Date       Comp       Opponent               Score
2026-10-13 UCL    H   Lille                  3–0
2026-10-10 PL     H   Leeds                  2–1             xG 2.3–1.2
2026-09-19 PL     A   Brighton               0–3             xG 1.2–2.1
2026-09-15 EFL    A   Ipswich                4–2
2026-09-12 PL     A   Sunderland             2–0             xG 2.0–2.1

╭───────────────╮╭───────────────╮╭───────────────╮╭───────────────╮╭───────────────╮
│ Position      ││ Points        ││ Record        ││ Goals         ││ xG            │
│ 2             ││ 15            ││ 5-0-1         ││ 10:5          ││ 11.7          │
│ 2026-27       ││ 6 played      ││ W-D-L         ││ +5            ││ -1.7 vs GF    │
╰───────────────╯╰───────────────╯╰───────────────╯╰───────────────╯╰───────────────╯
```

## Why footui exists

The numbers that explain football — expected goals, pressing, a club's form
across every competition, a nation's record with the friendlies taken out —
are scattered across websites built for one match or one season at a time.
footui puts them in one place you can move through with the keyboard, from a
league table to a club to a player and back, across as many seasons as you
care to download.

A few principles shape everything in it:

- **Your data, on your machine.** Everything footui shows comes from a local
  database. It works offline, opens instantly, and keeps history that the
  sources themselves page away.
- **Honest numbers.** A figure footui doesn't have is shown as missing, never
  as zero. Competitions are never blended: a club's Premier League and
  Champions League seasons are separate lines. An impossible figure from a
  source is dropped rather than displayed.
- **Polite to free sources.** Both data sources are free and undocumented.
  footui fetches in shared batches, never re-fetches what can't have
  changed, and stops at once when asked to.
- **One identity per club and player.** Arsenal reached through two sources
  is one club, and Saka has one career.
- **Keyboard-first and readable.** Every action is reachable from the TUI,
  every screen lists its keys, and a club's screens are drawn in its
  colours at accessible contrast.

## Install

Requires Go 1.26 or newer.

```sh
go install github.com/JohnSBarden/footui/cmd/footui@latest
```

Or build from a clone:

```sh
make static      # single static binary at bin/footui
make release     # cross-compiled binaries for linux and macOS in dist/
```

The SQLite driver is [`modernc.org/sqlite`](https://modernc.org/sqlite), a
pure-Go implementation, so the result is one static binary with nothing to
install alongside it.

## Getting started

`footui` ships with no data. The easiest start is the in-app league catalog:
open `footui`, press `ctrl+g` (or `d` on any screen without a search box),
pick a league, tick the seasons you want and press `enter`. Downloads run in
the background, one league at a time, with progress on the status line, so
you can keep browsing while they finish.

Each launch opens on footui's wordmark for a moment, listing the leagues it
is bringing up to date (and saying so if ESPN has paused footui for asking
too often); any key skips it.

From then on, opening footui keeps what you follow current. A league whose
current season you have is refreshed once it's more than six hours old, and
when a new season starts, the one you followed last season gets it.
Starred clubs and nations have their games kept current the same way.
`--stale-after 2h` changes the threshold and `--stale-after 0` turns this
off.

Or fetch from the shell:

```sh
footui refresh                        # current season of the Premier League
footui refresh --competition esp-la-liga --years 3   # La Liga, this season and two before
footui refresh --all                  # current season of the starter set (big five, MLS, NWSL, WSL, World Cup, UCL, UEL)
footui refresh --all --years 5 --dry-run   # show what that would fetch, and stop
footui refresh --team england         # a national team's matches, this year and last
footui refresh --team arsenal         # a cached club's recent games
footui refresh --competition world-cup --season 2022   # a tournament edition
```

For a scheduled refresh, `--max-age` skips whatever is already current:

```sh
footui refresh --all --years 10 --max-age 24h
```

`footui status` shows what you have and how fresh it is. Then explore:

```sh
footui                          # your favourites, or browse everything cached
footui league "premier"         # standings table
footui team "Arsenal"           # club dashboard
footui team england             # a national team ("england women" for the women's side)
footui league world-cup         # a tournament: groups, then fixtures by round
footui player "Saka"            # player career
```

Names are matched against what you've cached, ignoring case and accents, so
partial names work — `footui player martin` finds Martín Zubimendi. A name
matching several results opens the browse screen filtered to them rather
than guessing.

### Keys

Every screen shows its bindings along the bottom; `?` expands the full set.

| Key | Anywhere |
|---|---|
| `/` | Search, from any screen |
| `enter` | Open the highlighted league, club, nation, player, season or card |
| `esc` | Back (clears a search query first) |
| `f` | Star or un-star the league, club, nation or player on screen, or the highlighted result or card |
| `r` | Refresh what's on screen — a competition, a club's or a nation's games; progress shows on the status line |
| `d` | Open the league catalog |
| `t` | Switch theme, and remember the choice |
| `?` | Show every key |
| `q` | Quit |
| `ctrl+k` | A surprise, listed only under `?` |

The search box takes letters as text, so while you type there use `ctrl+f`,
`ctrl+r`, `ctrl+g`, `ctrl+t` and `ctrl+c` instead, or press `tab` to move
into the results, where `j`/`k` and the plain keys work again.

| Screen | Keys |
|---|---|
| Dashboard | arrows or `hjkl` move between cards |
| League | `←`/`→` change season; for a tournament, `[`/`]` step through groups and `m` lists every fixture by round |
| Team | `←`/`→` change competition, `tab` moves between seasons and squad, `m` opens the last 30 games |
| Games | `enter` opens a box score, `o` opens the opponent |
| Player | `←`/`→` change competition |
| Nation | `←`/`→` change year, `tab` moves into the competition filter (`space` applies), `enter` opens a box score |
| Catalog | `space` ticks a season, `a` every missing season of a league, `c` its current season, `C` the current season of every club league, `u` clears, `enter` downloads, `s` stops downloads |

### Favourites and the dashboard

Star what you follow with `f`. It stars whatever is highlighted: on a league
table, the league while the cursor is on its header (where the screen
opens) and the club once you move into the table. Starred names carry a ★.
Once anything is starred, `footui` opens on a dashboard of cards that flow
together into columns, leagues first, then tournaments, clubs, nations and
players. Each card's title carries a badge that says what it is:

- **League** 🥅 — twice the size of the others, two columns wide: the top
  fifteen of the current table with each club's full record, and your
  starred clubs picked out even further down.
- **Tournament** 🏆 — your nation's group during the group stage, the
  latest round after it.
- **Club** `◆` in its colour, with its colours in a scarf along the top —
  position, points and form, record, and xG against goals where understat
  covers the league.
- **Nation** — its flag: its last three results, its next fixture and its
  form, under its saved filter.
- **Player** `●` in their club's colour — the season's goals and assists,
  plus xG and xA where available.

Each card says at its top right how fresh its numbers are (`3h`), and when a
refresh of them is running. Favourites are keyed by footui's own IDs, so a club starred from its
Premier League page is the same favourite reached through the Champions
League.

### Recent games

A club's screen opens with its form and its last five games in every
competition — its league, its country's cups, and Europe — with the score
from the club's side, extra time and shoot-outs marked, and xG where
understat covers the league. `m` lists the last 30, each with a box score
(possession, shots, corners and fouls from ESPN; xG from understat).

They arrive on their own: opening a club fetches whatever its games need,
and nothing if they're current. The first English club costs about ten
requests — the FA Cup, the League Cup and the three European competitions,
this year and last — and every English club after it costs none, because
those lists are shared (see [How it works](#how-it-works)).

The cups are the FA Cup, League Cup, Copa del Rey, DFB-Pokal, Coppa Italia,
Coupe de France and U.S. Open Cup, men's only. A cup opponent from outside
the leagues footui covers is a club of its own: open it from a game and you
get its games, without a table.

### National teams and tournaments

Every nation is searchable from the first launch. Open one and star it: its
matches for this year and last arrive in the background — friendlies,
qualifiers, Nations League, tournaments — newest first, with kickoff times
for fixtures to come and each match's box score a keypress away. The first
nation costs about 25 requests; every nation after it costs none.

The competition filter above the matches is **All**, **Competitive** (no
friendlies), **Tournaments**, or a tick per competition. The record and
averages are worked out from exactly the matches the filter and year keep,
and the filter is remembered per nation. A shoot-out counts as a draw, as
FIFA records it. There are no player stats for national teams.

A tournament opens on the League screen: its groups one at a time, or `m`
for every fixture by round through to the final.

### The look

One night-match palette, called Floodlit: footui's own lime for the
wordmark, focus and the cursor, and a club's colours wherever the screen
belongs to a club — the band across the top of its page, the dot beside its
name in tables and search, the scarf along its dashboard card. Everything
else carries a number:

- Form is a row of letter chips, **W** green, **D** amber, **L** red. Each
  game in a list leads with ● won, ◐ drawn or ○ lost.
- Bars show points against the leader, xG against goals, and expected goal
  difference either side of zero.
- On a league table a coloured `▌` marks the Champions League places
  (cyan), the Europa League place (orange) and relegation (red), where
  footui knows that season's rule; without colour they're `C`, `E` and `↓`.
- A goal or assist count is green where it beat its expected value and
  orange where it fell short.

Nothing depends on colour alone: `--no-color` keeps every letter and glyph.

### Themes

A club's screens are drawn in its colours, in one of two modes:

- **weak** (default): your terminal's own background, with club colours for
  highlights, headings and borders.
- **strong**: the whole screen painted in the club's primary colour, cut
  into mowing stripes like a pitch. Text, accents and win/loss colours are
  chosen to meet WCAG AA contrast on both stripes. Screens without a club
  are the kickabout's grass.

`t` switches mode and saves the choice; `--theme` and `FOOTUI_THEME` override
it for a single run.

### Settings

Settings live in an optional YAML file at
`$XDG_CONFIG_HOME/footui/config.yaml` (usually `~/.config/footui/config.yaml`;
`$FOOTUI_CONFIG` points elsewhere):

```sh
footui config init    # write a commented file with every setting at its default
footui config         # show the settings in effect and where each came from
footui config path    # print the file's location
```

```yaml
theme: strong        # weak | strong
stale_after: 2h      # refresh-on-open threshold; 0 turns it off
db: ~/footui/footui.db # database location
no_color: false
```

A flag always wins, then an environment variable (`FOOTUI_THEME`,
`NO_COLOR`), then the file, then the default. A misspelt key is an error
naming the file, not a setting silently ignored.

## What's covered

**Read this before relying on it.** footui draws on two sources, and what you
get for a competition depends on which one serves it (`footui competitions`
lists each with its source and history):

| | understat.com | ESPN |
|---|---|---|
| Competitions | Premier League, La Liga, Bundesliga, Serie A, Ligue 1, Russian Premier League | MLS, USL Championship; Championship, La Liga 2, 2. Bundesliga, Serie B, Ligue 2; Women's Super League, Liga F, Première Ligue, NWSL; Champions League, Europa League, Conference League, Women's Champions League |
| History | 2014-15 on | 2010-11 on for most; 2016 for NWSL, 2018-19 for the WSL, 2021-22 or later for the newest |
| Team stats | Results, goals, points, xG, xGA, npxG, PPDA, deep completions, expected points | Results, goals, points, possession, shots, shots on target, corners, fouls, cards |
| Player stats | Appearances, minutes, goals, assists, shots, key passes, xG, xA, npxG | Appearances, goals, assists, shots, cards — no minutes |
| Per match | Score and xG | Score, possession, shots, shots on target, corners, fouls |
| Not available | Possession, fouls, corners, cards | Expected goals of any kind |

International football comes from ESPN: the World Cup, Euros, Copa América,
Gold Cup, Africa Cup of Nations and Asian Cup; World Cup qualifying in five
confederations; Euro qualifying; the UEFA and Concacaf Nations Leagues;
friendlies; and the Women's World Cup, Women's Euro, UEFA Women's Nations
League and women's friendlies, mostly from 2010-2012 on.

Domestic cups are there as clubs' games only, with no tables. Not covered:
the German and Italian women's leagues, third tiers and below as
competitions, women's cups, super cups, and player stats for national teams.
Tables with groups (MLS's conferences, old Champions League group stages)
are shown as one overall table.

Two datasets are hand-curated because no scrapeable source exists, and both
welcome contributions:

- **Club colours** (`internal/colors/seed/teamcolors.yaml`) override what
  ESPN supplies; `footui colors` reports cached clubs that still lack any.
- **Honours** (`internal/trophies/seed/trophies.yaml`) are deliberately
  partial — complete European Cup lists and domestic league titles from
  2000-01 — and the Team screen says so.

## Commands

| Command | What it does |
|---|---|
| `footui` | Open the dashboard of favourites, or browse everything cached |
| `footui league <name>` | Open a competition's standings |
| `footui team <name>` | Open a club's dashboard, or a national team's matches |
| `footui player <name>` | Open a player's career |
| `footui refresh` | Fetch data (`--competition ID`, `--all`, `--years N`, `--season YYYY`, `--team NATION\|CLUB`, `--max-age D`, `--dry-run`) |
| `footui status` | Show what's cached and how fresh it is |
| `footui competitions` | List what can be refreshed |
| `footui colors` | Apply curated club colours and report coverage |
| `footui config` | Show settings and their sources (`init`, `path`) |
| `footui completion <shell>` | Shell completions for bash, zsh, fish, powershell |

Global flags: `--db` for another database, `--no-color` (or `NO_COLOR`),
`--theme weak|strong`, and `--stale-after D` (default `6h`, `0` disables).

## Where data lives

`$XDG_DATA_HOME/footui/footui.db`, falling back to
`~/.local/share/footui/footui.db`. It is durable data, not a disposable cache:
re-fetching several seasons is slow and leans on someone else's server.

There is no daemon; a scheduled refresh is a cron entry or systemd timer
running `footui refresh --all --max-age 24h`. One competition failing doesn't
abort the rest, the summary names what failed, and the process exits
non-zero only if nothing at all was fetched.

## How it works

The code is organised around a few patterns. The details are in
[`docs/architecture.md`](docs/architecture.md) and
[`docs/data-model.md`](docs/data-model.md).

**Three boundaries.** Providers (`internal/data/provider`) turn a source's
payloads into canonical `models` types and know nothing of storage.
Storage (`internal/storage`) is one `Repositories` interface over SQLite;
nothing else writes SQL. Sync (`internal/data/sync`) is the only code that
moves data from one to the other, and it is tested with fakes on each side.
The TUI reads only from storage; the CLI builds the providers and hands them
to sync. Screens reach each other through
injected constructors, so no screen imports another.

**A registry routes competitions.** `sync.Coverages` lists every competition
footui can fetch, the provider that serves it, and how far back it goes.
Adding a competition is a row there and its source's slug; adding a source
is a package implementing the provider interfaces.

**Identity is resolved once and remembered.** Each source has its own IDs.
Before anything is written, sync maps them to footui's canonical ones —
matching clubs by normalised name within a country, exactly within Europe,
players within their club's squad — and records every mapping, so a later
refresh never re-guesses. A club it can't confidently match is kept separate
rather than merged into the wrong one; a nation is never matched to a club.

**Two grains.** Club seasons are stored as table lines per (team,
competition, season), which keeps competitions from ever being summed. Games
— a nation's, or a club's recent ones — are stored per match, and records
and averages are computed from the matches on read, so any filter gives a
correct record with nothing precomputed to go stale.

**The unit of fetching is shared.** ESPN's scoreboard returns a whole
competition's calendar year in one request — every nation's friendlies,
every club's FA Cup ties — so following a second nation or club of the same
country costs nothing. understat's league payload already lists a season's
matches, so a club's league games come free with the table.

**Politeness is a rule set, not a hope.** A year fetched after it ended is
complete and never fetched again. The current year is fetched again only
when it's stale and a stored match has kicked off since, or once a week for
new fixtures. Requests to both sources are paced and retried with backoff.
If ESPN keeps answering 429, footui stops asking it for an hour, the TUI
drops its queue, and the stale check on open stands down.

**Plan, then fetch.** Whether something needs fetching is decided from the
sync log without a request: opening a club asks whether its games are due
and queues a refresh only if they are. Every refresh — from the catalog, the
`r` key, the stale check on open, or the CLI — goes through one pipeline and
one queue, so nothing is fetched twice at once.

**Fetch everything, then write.** Each season or competition-year is fetched
in full before any of it is written, so a failure leaves no half-written
season behind, and every attempt is logged — failures included — so
freshness is never overstated.

## How it's built

**Tests at the boundaries.** Providers are tested against hand-written
payloads in each source's shape, with fictional clubs and nations — never
captured real responses. Sync is tested with fake providers and a real
in-memory database, including request budgets (following a second nation
must cost no requests). Screens are tested by driving them with messages and
reading their rendered output.

**Schema changes are frozen.** Every migration has a fixture database
written by the schema it introduced (`internal/storage/sqlite/testdata`).
Every later build must open, upgrade and read each one, and a new migration
without a fixture fails CI.

**One gate.** `make check` runs gofmt, vet, staticcheck, a `go mod tidy`
check and the tests under the race detector. CI runs the same, and a pushed
`v*` tag publishes release binaries.

**A canary for the sources.** Fixture tests can't notice an undocumented
source changing shape, so `make live` fetches one real competition from each
(understat's Premier League, ESPN's NWSL) and checks the payloads still
parse into teams, scores, xG and squads. CI runs it weekly; a red canary
means a provider changed, not the code.

**Checked in the real thing.** Features are verified by running footui in a
terminal against a copy of a real database, not only by tests, with request
counts checked against what the design promised.

Working on it:

```sh
make check     # the full gate
make cover     # test coverage summary
make live      # the provider canary (FOOTUI_LIVE=1)
```

Set `FOOTUI_HTTP_CACHE=/some/dir` while developing to keep raw responses on
disk and avoid hitting the sources repeatedly. It's off by default, because
a refresh should mean the current numbers.

## Data sources

Data comes from [understat.com](https://understat.com) and ESPN's public
endpoints. footui is not affiliated with either, and the endpoints are
undocumented, so they can change without notice. Requests are throttled and
retried conservatively; please don't reconfigure footui to hammer them.

## License

[MIT](LICENSE). The data footui fetches belongs to its providers and is not
covered by this license.
