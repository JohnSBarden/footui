# footui

[![CI](https://github.com/JohnSBarden/footui/actions/workflows/ci.yml/badge.svg)](https://github.com/JohnSBarden/footui/actions/workflows/ci.yml)

A terminal dashboard for drillable football statistics.

`footui` (binary: `footy`) fetches league, team, and player data into a local
SQLite cache and lets you drill through it in an interactive TUI:
**Browse → League → Team → Player**, with expected goals, pressing metrics,
squad breakdowns, club honours, and every club's recent games — league,
cups and Europe — along the way. National teams and
international tournaments are covered too: every match a nation plays,
friendlies included, filterable down to the competitions you care about.

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
footy refresh --team england         # a national team's matches, this year and last
footy refresh --competition world-cup --season 2022   # a tournament edition
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
footy                          # your favourites, or browse everything cached
footy league "premier"         # standings table
footy team "Arsenal"           # club dashboard
footy team england             # a national team ("england women" for the women's side)
footy league world-cup         # a tournament: groups, then fixtures by round
footy player "Saka"            # player career
```

Names are matched against what you've cached, so partial names work, and
matching ignores case and accents — `footy player martin` finds Martín
Zubimendi. A name matching several results opens the browse screen filtered
to them rather than guessing.

### Keys

Every screen shows its bindings along the bottom; `?` expands the full set.

| Key | Anywhere |
|---|---|
| `/` | Search, from any screen |
| `enter` | Open the highlighted league, club, nation, player, season or card |
| `esc` | Back (clears a search query first) |
| `f` | Star or un-star the league, club, nation or player on screen, or the highlighted result or card |
| `r` | Refresh the competition on screen; progress shows on the status line |
| `d` | Open the league catalog to download leagues and seasons |
| `t` | Switch theme, and remember the choice |
| `?` | Show every key |
| `q` | Quit |

The search box takes letters as text, so while you type there use `ctrl+f`,
`ctrl+r`, `ctrl+g`, `ctrl+t` and `ctrl+c` instead, or press `tab` to move
into the results, where `j`/`k` and the plain keys work again.

On the dashboard the arrow keys (or `hjkl`) move between cards. On the
League screen `←`/`→` change season; for a tournament, `[`/`]` step through
its groups and `m` switches to every fixture by round. On Team and Player
`←`/`→` change competition, and on Team `tab` moves between the seasons and
the squad, and `m` opens the club's last 30 games (`enter` for a box score,
`o` to open the opponent). On a nation, `←`/`→` change year, `tab` moves into the
competition filter (`←`/`→` along it, `space` to apply), and `enter` opens a
match's box score.

In the league catalog, `space` ticks a season, `a` ticks every season of the
league you don't have, `c` its current season, `C` the current season of
every club league, and `u` clears the ticks. `enter` downloads what's
ticked, and `s` stops downloads in progress. Its International section lists
tournaments by edition, qualifiers and nations leagues by year, and a
"National teams" line per gender for older years of every nation at once.

### Favourites and the dashboard

Star the leagues, clubs and players you follow with `f`. It stars whatever
is highlighted: on a league table, that's the league while the cursor is on
its header (where the screen opens) and the club once you move down into
the table. In the league catalog it's the highlighted league, and in search
results the highlighted result. Starred names carry a ★. Once anything is
starred, `footy` opens on a dashboard of them instead of the search box:

- **League cards** show the top of the current table, with any club you've
  starred picked out, even if it's further down.
- **Club cards** show league position, points, the season's record, xG
  against goals where understat covers the league, and form over the last
  five games in every competition.
- **Player cards** show the season's goals and assists, plus xG and xA
  where available, for the club and competition they've played most in.

Cards are grouped leagues, clubs, then players, in the order you starred
them, and stack into one column on a narrow terminal. Arrow keys (or
`hjkl`) move between cards, `enter` opens one, `/` searches, and `f`
un-stars the selected card. Each card says how fresh its numbers are, and
when a refresh of them is running or queued. A starred league is always
kept up to date when footy opens, even if you'd only downloaded old
seasons of it before.

Favourites are kept in the database and survive refreshes. They're keyed by
footy's own IDs, so a club starred from its Premier League page is the same
favourite when you reach it through the Champions League.

### Recent games

A club's screen opens with its form and its last five games in every
competition — its league, its country's cups, and Europe — each with the
score from the club's side, extra time and shoot-outs marked, and xG where
understat covers the league. `m` lists the last 30, with a box score for
each (possession, shots, corners and fouls from ESPN; xG from understat).

They arrive on their own: opening a club fetches what its games need, if
anything, and a starred club is kept current when footy opens. A league's
games come with its table — understat lists them in the payload footy
already downloads — and everything else comes from ESPN's yearly match
lists, shared by every club. So the first English club costs about ten
requests (the FA Cup, the League Cup and the three European competitions,
this year and last), and every English club after it costs none. The same
rules as national teams keep it polite: a year that has ended is never
fetched again, and a rate limit stops footy at once.

Cup opponents from outside the leagues footy covers are clubs of their own:
open one from a game and you get its games, without a table. The cups are
the FA Cup, League Cup, Copa del Rey, DFB-Pokal, Coppa Italia, Coupe de
France and U.S. Open Cup, men's only; super cups and CONCACAF's club
competitions aren't included.

### National teams and tournaments

Every nation is searchable from the first launch (footy fetches ESPN's
list of national teams once). Open one and star it with `f`: its matches
for this year and last arrive in the background. Its screen shows every
match it played, newest first — friendlies, qualifiers, Nations League,
tournaments — with aet and penalty shoot-outs, kickoff times for fixtures
to come, and each match's possession, shots, corners and fouls a keypress
away.

Above the matches sits the competition filter: **All**, **Competitive**
(no friendlies), **Tournaments**, or a tick per competition. The record and
averages (played, won, drawn, lost, goals, possession, shots, shots on
target, form) are worked out from exactly the matches the filter and year
keep, and the filter is remembered for each nation. A shoot-out counts as a
draw, as FIFA records it. There are no player stats for national teams.

A tournament opens on the League screen: its groups one at a time (`[`/`]`),
or `m` for every fixture by round through to the final. Starred, it gets a
dashboard card showing your nation's group during the group stage and the
latest round after it. A starred nation's card shows its last three
results, its next fixture and its form, under its saved filter.

**What it costs ESPN.** Matches are fetched a competition-year at a time:
one request returns every nation's 2025 friendlies. So the first nation
you star costs about 25 requests (each international competition, this
year and last), and every nation after it costs none. A year that has
ended is never fetched again once it's complete; the current year is
re-fetched only when it's stale and one of its matches has been played
since (or once a week, for newly announced fixtures). If ESPN ever
rate-limits footy, it stops at once, says so, and leaves ESPN alone for an
hour.

### Themes

A club's dashboard is drawn in its colors, in one of two modes:

- **weak** (default): your terminal's own background, transparent if it is,
  with club colors for highlights, headings and borders.
- **strong**: the whole screen painted in the club's primary color. Text,
  accents, and win/loss colors are chosen to meet WCAG AA contrast against
  it, and the selected row is drawn inverse.

Press `t` (or `ctrl+t` while typing in search) to switch while running.
The choice is saved to the settings file (below), so it sticks. Strong
mode paints every screen: a club's pages in its colors, everything else in
footy's blue. `--theme` and `FOOTY_THEME` still override the saved choice
for a single run.

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

International football comes from ESPN too: the World Cup, Euros, Copa
América, Gold Cup, Africa Cup of Nations and Asian Cup; World Cup
qualifying in Europe, South America, North America, Asia and Africa; Euro
qualifying; the UEFA and Concacaf Nations Leagues; friendlies; and the
Women's World Cup, Women's Euro, UEFA Women's Nations League and women's
friendlies. History goes back to 2010-2012 for most, 2018 for the Nations
League and 2025 for the women's one. Each match has the result and, where
ESPN has them, both sides' possession, shots, shots on target, corners and
fouls; cards and saves aren't in the data footy fetches.

Domestic cups (the seven listed under [Recent games](#recent-games)) are
there as clubs' games only, with no tables of their own. Not covered: the
German and Italian women's leagues, third tiers and below as competitions,
women's cups, and player stats for national teams.

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
| `footy` | Open the dashboard of favourites, or browse everything cached when there are none |
| `footy league <name>` | Open a competition's standings |
| `footy team <name>` | Open a club's dashboard, or a national team's matches |
| `footy player <name>` | Open a player's career dashboard |
| `footy refresh` | Fetch data (`--competition ID`, `--all`, `--years N`, `--season YYYY`, `--team NATION|CLUB`, `--max-age D`, `--dry-run`) |
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
make live      # provider canary: fetch one live competition from each source
```

Both sources are undocumented, so the fixture tests can't notice when one
changes shape. `make live` (the tests run only with `FOOTY_LIVE=1`) refreshes
the current Premier League season from understat and the current NWSL season
from ESPN and checks the payloads still parse into teams, scores, xG, team
stats and squads. A scheduled CI job runs it weekly; a red canary means a
provider changed, not that the code did.

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

## Data sources

Data comes from [understat.com](https://understat.com) and ESPN's public
endpoints. footy is not affiliated with either, and the endpoints are
undocumented, so they can change without notice. Requests are throttled and
retried conservatively; please don't reconfigure footy to hammer them.

## License

[MIT](LICENSE). The data footy fetches belongs to its providers and is not
covered by this license.
