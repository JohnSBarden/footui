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

`footy` ships with no data. Press `ctrl+r` on the first screen to fetch the
Premier League, or fetch from the shell:

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
the outcome show on the status line.

## What's covered

**This is the part to read before relying on it.** footy's coverage is
determined by its data source, and the gaps are real:

| | Covered |
|---|---|
| Competitions | Premier League, La Liga, Bundesliga, Serie A, Ligue 1, Russian Premier League |
| Tiers | Top flight only — no second divisions |
| Women's football | Not covered |
| Continental competitions | Not covered as competitions in their own right |
| Team stats | Results, goals, points, xG, xGA, npxG, PPDA, deep completions, expected points |
| Player stats | Appearances, minutes, goals, assists, shots, key passes, xG, xA, npxG |
| Missing team stats | Possession, fouls, corners, cards — the source has none |

The original design targeted the top two divisions of six countries plus
women's leagues. The v1 source (understat.com) doesn't reach that far. The
gap is tracked in `internal/data/sync/scope.go` and
[`docs/architecture.md`](docs/architecture.md), not papered over — and the
provider boundary is designed so closing it means writing one new package.

Two datasets are hand-curated because no scrapeable source exists:

- **Club colors** (`internal/colors/seed/teamcolors.yaml`) — used to theme a
  club's dashboard. `footy colors` reports which cached clubs still lack them.
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
| `footy completion <shell>` | Shell completions for bash, zsh, fish, powershell |

Global flags: `--db` to point at another database, `--no-color` to disable
color (`NO_COLOR` is honoured too).

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
