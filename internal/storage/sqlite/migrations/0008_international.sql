-- National teams and international competitions.
--
-- Clubs are stored as one stat line per (team, competition, season). A
-- nation plays a dozen matches a year across half a dozen competitions,
-- and its record has to be filterable down to any of them, so it is
-- stored at the match grain instead and summed on read.

-- National team IDs are 'nat-{slug}' ('nat-{slug}-women'), which can't
-- collide with a club's country-prefixed ID.
ALTER TABLE team ADD COLUMN national INTEGER NOT NULL DEFAULT 0;

-- NULL for a club competition; 'tournament', 'qualifier',
-- 'nations-league' or 'friendly' for an international one.
ALTER TABLE competition ADD COLUMN category TEXT;
-- A compact label for tight columns ('WC', 'UNL').
ALTER TABLE competition ADD COLUMN short_name TEXT;

-- The group a tournament table row belongs to; position is then the rank
-- within it. NULL for one table, which is every club row.
ALTER TABLE team_season_stats ADD COLUMN group_label TEXT;

-- International matches are fetched one competition-year at a time (one
-- request covers every nation), and those fetches are logged too. A
-- year run has year set and season_id NULL.
ALTER TABLE sync_log ADD COLUMN year INTEGER;
CREATE INDEX idx_sync_log_year ON sync_log(competition_id, year, finished_at DESC);

-- Upserted by ID and never deleted: a rescheduled fixture updates in
-- place. kickoff is RFC3339 UTC text (see data-model.md's driver caveat).
CREATE TABLE match (
  id             TEXT PRIMARY KEY,
  competition_id TEXT NOT NULL REFERENCES competition(id),
  season_id      TEXT NOT NULL REFERENCES season(id),
  kickoff        TEXT NOT NULL,
  status         TEXT NOT NULL,   -- scheduled | in_play | ft | aet | pens | postponed | cancelled
  round          TEXT,            -- display label: 'Group B', 'Final'
  home_team_id   TEXT NOT NULL REFERENCES team(id),
  away_team_id   TEXT NOT NULL REFERENCES team(id),
  neutral        INTEGER NOT NULL DEFAULT 0,
  home_score     INTEGER,
  away_score     INTEGER,
  home_pens      INTEGER,
  away_pens      INTEGER,
  source         TEXT NOT NULL,
  updated_at     TEXT NOT NULL
);

CREATE INDEX idx_match_home ON match(home_team_id, kickoff);
CREATE INDEX idx_match_away ON match(away_team_id, kickoff);
CREATE INDEX idx_match_competition ON match(competition_id, kickoff);
CREATE INDEX idx_match_season ON match(season_id);

-- One side's numbers from one match. NULL when missing or implausible, so
-- an average skips it rather than counting a zero.
CREATE TABLE match_team_stats (
  match_id        TEXT NOT NULL REFERENCES match(id),
  team_id         TEXT NOT NULL REFERENCES team(id),
  possession      REAL,
  shots           INTEGER,
  shots_on_target INTEGER,
  corners         INTEGER,
  fouls           INTEGER,
  PRIMARY KEY (match_id, team_id)
);

-- The last competition filter used on each nation: a preset, or an
-- explicit JSON list of competition IDs.
CREATE TABLE team_match_filter (
  team_id      TEXT PRIMARY KEY,
  preset       TEXT,
  competitions TEXT,
  updated_at   TEXT NOT NULL
);
