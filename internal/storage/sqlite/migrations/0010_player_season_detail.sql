-- Player season detail: the stats a position is judged on beyond goals and
-- assists (clean sheets, saves, tackles, interceptions, clearances,
-- passing), fetched per player when one is opened or starred. Its own
-- table rather than more player_season_stats columns because it is
-- fetched on a different schedule and for only a few players; fetched_at
-- is what that schedule runs on. Every stat is nullable: NULL is "not
-- recorded", distinct from a real zero.
CREATE TABLE player_season_detail (
  player_id      TEXT NOT NULL,
  team_id        TEXT NOT NULL,
  competition_id TEXT NOT NULL,
  season_id      TEXT NOT NULL,
  position       TEXT,
  appearances    INTEGER,
  minutes        INTEGER,
  clean_sheets   INTEGER,
  goals_conceded INTEGER,
  saves          INTEGER,
  tackles        INTEGER,
  interceptions  INTEGER,
  clearances     INTEGER,
  pass_pct       REAL,
  fetched_at     TEXT NOT NULL,
  PRIMARY KEY (player_id, team_id, competition_id, season_id)
);
