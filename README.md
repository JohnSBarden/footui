# footui

A terminal dashboard for drillable football statistics.

`footui` (binary: `footy`) fetches league, team, and player data into a local
SQLite cache and lets you drill through it in an interactive TUI:
**Browse → League → Team → Player**, with expected goals, pressing metrics,
squad breakdowns, and club honours along the way.

```
 Arsenal   England · men
[Premier League (1)]

╭───────────────╮╭───────────────╮╭───────────────╮╭───────────────╮
│ Position      ││ Points        ││ xG            ││ xPts          │
│ 1             ││ 85            ││ 77.5          ││ 79.9          │
│ 2025-26       ││ 38 played     ││ -6.5 vs GF    ││ +5.1 vs Pts   │
╰───────────────╯╰───────────────╯╰───────────────╯╰───────────────╯
```

## Install

Requires Go 1.26 or newer.

```sh
go install github.com/JohnSBarden/footui/cmd/footy@latest
```

Or build from a clone:

```sh
make static      # single static binary at bin/footy
make release     # cross-compiled binaries for linux and macOS in dist/
```

The SQLite driver is [`modernc.org/sqlite`](https://modernc.org/sqlite), a
pure-Go implementation, so the result is one static binary with nothing to
install alongside it.

## Getting started

`footy` ships with no data. The easiest start is the in-app league
catalog: open `footy`, press `ctrl+g` (or `d` on any screen without a
search box), pick a league, tick the seasons you want (back to 2014-15) and
press `enter`. Downloads run in the background, one league at a time, with
progress on the status line, so you can keep browsing while they finish.

Opening the interface also brings followed leagues up to date. If a league
you have the current season of was last refreshed more than six hours ago,
it is refreshed in the background. Once a new season starts, a league you
followed last season gets the new one fetched. `--stale-after 2h` changes
the threshold and `--stale-after 0` turns this off.

Or fetch from the shell:

```sh
footy refresh                        # current season of the Premier League
footy refresh --all                  # current season of every covered competition
footy refresh --all --years 5        # ...and the four seasons before it
footy refresh --all --years 5 --dry-run   # show what that would fetch, and stop
```

For a scheduled refresh, `--max-age` skips what's already current. A
finished season doesn't change, so re-fetching it nightly is slow and
unkind to someone else's server:

```sh
footy refresh --all --years 10 --max-age 24h
```

Check what you have at any point:

```sh
footy status
```

Then explore:

```sh
footy                          # browse everything cached
footy league "premier"         # standings table
footy team "Arsenal"           # club dashboard
footy player "Saka"            # player career
```

Names are matched against what you've cached, so partial names work, and
matching ignores case and accents — `footy player martin` finds Martín
Zubimendi. A name matching several results opens the browse screen filtered
to them rather than guessing.

### Keys

Every screen shows its bindings along the bottom; `?` expands the full set.
`esc` goes back, `q` quits. `r` (or `ctrl+r` while the search box has focus)
refreshes the competition on screen without leaving the app; progress and
the outcome show on the status line. `d` (or `ctrl+g`) opens the league
catalog. In it, `space` ticks a season, `a` ticks every season you don't
have yet, `c` ticks the current one, and `enter` downloads what's ticked.

### Themes

A club's dashboard is drawn in its colors, in one of two modes:

- **weak** (default): your terminal's own background, transparent if it is,
  with club colors for highlights, headings and borders.
- **strong**: the whole screen painted in the club's primary color. Text,
  accents, and win/loss colors are chosen to meet WCAG AA contrast against
  it, and the selected row is drawn inverse.

Press `t` (or `ctrl+t`, which also works while typing in search) to switch
while running, or pick one with `--theme strong`. Strong mode paints every
screen: a club's pages in its colors, everything else in footy's blue. To
make it the default, set `theme: strong` in the settings file (below) or
`FOOTY_THEME=strong` in your shell profile.

### Settings

Settings live in an optional YAML file at
`$XDG_CONFIG_HOME/footui/config.yaml` (usually `~/.config/footui/config.yaml`;
set `$FOOTY_CONFIG` to use another path):

```sh
footy config init    # write a commented file with every setting at its default
footy config         # show the settings in effect and where each came from
footy config path    # print the file's location
```

```yaml
theme: strong        # weak | strong
stale_after: 2h      # refresh-on-open threshold; 0 turns it off
db: ~/footy/footy.db # database location
no_color: false
```

A flag on the command line always wins, then an environment variable
(`FOOTY_THEME`, `NO_COLOR`), then the file, then the built-in default. A
misspelt key is an error naming the file, not a setting silently ignored.

## What's covered

**This is the part to read before relying on it.** footy draws on two
sources, and what you get for a competition depends on which one serves it
(`footy competitions` lists every competition with its source and how far
back it goes):

| | understat.com | ESPN |
|---|---|---|
| Competitions | Premier League, La Liga, Bundesliga, Serie A, Ligue 1, Russian Premier League | MLS, USL Championship; Championship, La Liga 2, 2. Bundesliga, Serie B, Ligue 2; Women's Super League, Liga F, Première Ligue, NWSL; Champions League, Europa League, Conference League, Women's Champions League |
| History | 2014-15 on | 2010-11 on for most; 2016 for NWSL, 2018-19 for the WSL, 2021-22 or later for the newest competitions |
| Team stats | Results, goals, points, xG, xGA, npxG, PPDA, deep completions, expected points | Results, goals, points, possession, shots, shots on target, corners, fouls, cards |
| Player stats | Appearances, minutes, goals, assists, shots, key passes, xG, xA, npxG | Appearances, goals, assists, shots, cards — no minutes |
| Not available | Possession, fouls, corners, cards | Expected goals of any kind |

Not covered by either: the German and Italian women's leagues, third tiers
and below, and domestic cups.

A club or player reached through both sources is one entity: Arsenal's
Champions League seasons sit on the same dashboard as its Premier League
ones, and Saka has one career. footy matches clubs by name (exactly, for
European competitions; a club following its way between divisions is also
matched) and players within their club's squad, and remembers each match,
so it never re-guesses. A club it can't confidently match is kept separate
rather than merged into the wrong one. Women's sides are always separate
from the men's club of the same name.

ESPN's team figures are uneven across seasons — a season can report 0%
possession or thousands of corners. footy checks each against the matches
played and leaves an impossible figure blank rather than showing it.
Tables with groups (MLS's conferences, old Champions League group stages)
are shown as one overall table ranked by points, goal difference, then
goals scored.

Two datasets are hand-curated because no scrapeable source exists:

- **Club colors** (`internal/colors/seed/teamcolors.yaml`) — used to theme a
  club's dashboard. ESPN supplies colors for the clubs it serves, and the
  curated file overrides them where it has an entry. `footy colors` reports
  which cached clubs still lack any.
- **Honours** (`internal/trophies/seed/trophies.yaml`) — deliberately partial:
  complete European Cup lists, domestic league titles from 2000-01 onward. The
  Team screen labels the panel with that limit so a count is never mistaken for
  an all-time total.

Both are applied only to clubs actually in your cache, so adding an entry for
a club footy has never seen is harmless. Contributions to either file are the
easiest way to improve coverage.

## Commands

| Command | What it does |
|---|---|
| `footy` | Browse everything cached |
| `footy league <name>` | Open a competition's standings |
| `footy team <name>` | Open a club's dashboard |
| `footy player <name>` | Open a player's career dashboard |
| `footy refresh` | Fetch data (`--all`, `--years N`, `--season YYYY`, `--max-age D`, `--dry-run`) |
| `footy status` | Show what's cached and how fresh it is |
| `footy competitions` | List what can be refreshed |
| `footy colors` | Apply curated club colors and report coverage |
| `footy config` | Show settings and their sources (`init`, `path`) |
| `footy completion <shell>` | Shell completions for bash, zsh, fish, powershell |

Global flags: `--db` to point at another database, `--no-color` to disable
color (`NO_COLOR` is honoured too), `--theme weak|strong` for club screens
(default `$FOOTY_THEME`), and `--stale-after D` for the refresh-on-open
threshold (default `6h`, `0` disables).

## Where data lives

`$XDG_DATA_HOME/footui/footy.db`, falling back to
`~/.local/share/footui/footy.db`. This is durable data, not a disposable
cache — re-fetching several seasons of several leagues is slow and leans on
someone else's server.

There is no background daemon. A scheduled refresh is a cron entry or systemd
timer running `footy refresh --all --max-age 24h`. A refresh isolates failures: one
competition failing doesn't abort the rest, the summary names exactly what
failed, and the process only exits non-zero if nothing at all was fetched. If
the provider looks unreachable, the run gives up rather than working through
every remaining competition at full timeout.

## Development

```sh
make check     # vet, tests, and a gofmt check — what CI should run
make cover     # test coverage summary
```

Set `FOOTY_HTTP_CACHE=/some/dir` to cache raw provider responses on disk. It
is off by default, because `footy refresh` should mean "fetch the current
numbers" — but during development it keeps you from repeatedly hitting an
undocumented third-party endpoint.

Architecture is three explicit interface boundaries — source, storage,
rendering — described in [`docs/architecture.md`](docs/architecture.md), with
the data model in [`docs/data-model.md`](docs/data-model.md). The short
version: providers return canonical `models` types and never touch SQL; the
TUI queries `storage.Repositories` and never calls a provider; screens reach
each other through injected constructors so no screen imports another.

## Data source

Data comes from [understat.com](https://understat.com)'s public endpoint.
footy is not affiliated with it. Requests are throttled and retried
conservatively; please don't reconfigure it to hammer them.
