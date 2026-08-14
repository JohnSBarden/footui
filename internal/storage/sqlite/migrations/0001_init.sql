-- Canonical entities use slug-style TEXT primary keys (stable, human-debuggable,
-- and independent of any single provider's numeric IDs).

CREATE TABLE competition (
  id      TEXT PRIMARY KEY,        -- e.g. 'eng-premier-league'
  name    TEXT NOT NULL,
  country TEXT NOT NULL,           -- 'England','USA','France',...
  tier    INTEGER NOT NULL,        -- 1 = top flight, 2 = second division
  gender  TEXT NOT NULL,           -- 'men' | 'women'
  format  TEXT NOT NULL DEFAULT 'league'
);

CREATE TABLE season (
  id             TEXT PRIMARY KEY, -- 'eng-premier-league:2024-25'
  competition_id TEXT NOT NULL REFERENCES competition(id),
  label          TEXT NOT NULL,    -- '2024-25' or '2024' (MLS)
  start_date     TEXT,
  end_date       TEXT
);

CREATE TABLE team (
  id           TEXT PRIMARY KEY,   -- 'eng-manchester-united'
  name         TEXT NOT NULL,
  short_name   TEXT,
  country      TEXT,
  gender       TEXT NOT NULL,      -- distinct rows for men's/women's sides
  founded_year INTEGER,
  venue        TEXT
);

-- The anchor for per-competition-per-season aggregation: every stats query
-- joins through this table so a team's numbers stay sliceable by
-- competition instead of blending across them.
CREATE TABLE team_competition_season (
  team_id        TEXT NOT NULL REFERENCES team(id),
  competition_id TEXT NOT NULL REFERENCES competition(id),
  season_id      TEXT NOT NULL REFERENCES season(id),
  PRIMARY KEY (team_id, competition_id, season_id)
);

CREATE TABLE team_season_stats (
  team_id         TEXT NOT NULL,
  competition_id  TEXT NOT NULL,
  season_id       TEXT NOT NULL,
  position        INTEGER,         -- league standing rank (NULL for cup comps)
  matches_played  INTEGER,
  wins            INTEGER,
  draws           INTEGER,
  losses          INTEGER,
  goals_for       INTEGER,
  goals_against   INTEGER,
  goal_diff       INTEGER,
  points          INTEGER,
  possession_avg  REAL,
  fouls_committed INTEGER,
  fouls_suffered  INTEGER,
  yellow_cards    INTEGER,
  red_cards       INTEGER,
  shots           INTEGER,
  shots_on_target INTEGER,
  corners         INTEGER,
  xg              REAL,            -- nullable: filled by the xG provider pass
  xga             REAL,            -- nullable: filled by the xG provider pass
  source          TEXT NOT NULL,   -- provenance, e.g. 'fbref'
  updated_at      TEXT NOT NULL,
  PRIMARY KEY (team_id, competition_id, season_id),
  FOREIGN KEY (team_id, competition_id, season_id)
    REFERENCES team_competition_season(team_id, competition_id, season_id)
);

-- Long-tail stats without constant schema migrations.
CREATE TABLE team_season_stat_extra (
  team_id        TEXT NOT NULL,
  competition_id TEXT NOT NULL,
  season_id      TEXT NOT NULL,
  stat_key       TEXT NOT NULL,    -- e.g. 'progressive_passes', 'aerials_won'
  stat_value     REAL,
  PRIMARY KEY (team_id, competition_id, season_id, stat_key)
);

CREATE TABLE player (
  id            TEXT PRIMARY KEY,
  name          TEXT NOT NULL,
  date_of_birth TEXT,
  nationality   TEXT,
  position      TEXT,
  gender        TEXT NOT NULL
);

CREATE TABLE player_season_stats (
  player_id      TEXT NOT NULL REFERENCES player(id),
  team_id        TEXT NOT NULL,
  competition_id TEXT NOT NULL,
  season_id      TEXT NOT NULL,
  appearances    INTEGER,
  minutes        INTEGER,
  goals          INTEGER,
  assists        INTEGER,
  xg             REAL,
  xa             REAL,
  yellow_cards   INTEGER,
  red_cards      INTEGER,
  source         TEXT NOT NULL,
  updated_at     TEXT NOT NULL,
  PRIMARY KEY (player_id, team_id, competition_id, season_id)
);

CREATE TABLE trophy (
  id             INTEGER PRIMARY KEY,
  team_id        TEXT NOT NULL REFERENCES team(id),
  competition_id TEXT,             -- nullable: e.g. a cup not tracked as an in-scope competition
  title          TEXT NOT NULL,    -- 'Premier League Champions'
  season_label   TEXT,             -- '2012-13' or a bare year
  trophy_type    TEXT NOT NULL,    -- league_title | domestic_cup | continental_cup | other
  source         TEXT NOT NULL DEFAULT 'seed'
);

CREATE TABLE team_colors (
  team_id       TEXT PRIMARY KEY REFERENCES team(id),
  primary_hex   TEXT NOT NULL,
  secondary_hex TEXT,
  tertiary_hex  TEXT,
  source        TEXT NOT NULL DEFAULT 'seed:v1'
);

-- Maps a canonical entity to a specific provider's identifier for that
-- entity, so a future second provider can be merged/cross-checked without
-- renegotiating IDs.
CREATE TABLE external_ref (
  entity_type  TEXT NOT NULL,      -- 'team' | 'competition' | 'player'
  canonical_id TEXT NOT NULL,
  provider     TEXT NOT NULL,      -- 'fbref', etc.
  provider_id  TEXT NOT NULL,
  PRIMARY KEY (entity_type, provider, provider_id)
);

CREATE TABLE sync_log (
  id            INTEGER PRIMARY KEY,
  provider      TEXT,
  entity_type   TEXT,
  started_at    TEXT,
  finished_at   TEXT,
  status        TEXT,
  rows_upserted INTEGER,
  error         TEXT
);

CREATE INDEX idx_season_competition ON season(competition_id);
CREATE INDEX idx_tcs_competition_season ON team_competition_season(competition_id, season_id);
CREATE INDEX idx_player_stats_player ON player_season_stats(player_id);
CREATE INDEX idx_trophy_team ON trophy(team_id);
