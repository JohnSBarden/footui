-- Match detail: one match's events, full stat sheet, lineups and
-- commentary, fetched for a club's last game. A row in match_detail means
-- the detail was fetched; the other tables hang off it and are replaced
-- whole on each fetch. The stats are their own table rather than more
-- match_team_stats columns because the scoreboards rewrite those rows.
-- Sides are 'home' or 'away' rather than team IDs: the match row already
-- says who they are. Every stat is nullable: NULL is "not recorded".
CREATE TABLE match_detail (
  match_id       TEXT PRIMARY KEY REFERENCES match(id),
  final          INTEGER NOT NULL,  -- fetched after full time
  venue          TEXT,
  attendance     INTEGER,
  home_formation TEXT,
  away_formation TEXT,
  fetched_at     TEXT NOT NULL
);

CREATE TABLE match_detail_stats (
  match_id            TEXT NOT NULL REFERENCES match_detail(match_id),
  side                TEXT NOT NULL CHECK (side IN ('home', 'away')),
  possession          REAL,
  shots               INTEGER,
  shots_on_target     INTEGER,
  blocked_shots       INTEGER,
  passes              INTEGER,
  accurate_passes     INTEGER,
  crosses             INTEGER,
  accurate_crosses    INTEGER,
  long_balls          INTEGER,
  accurate_long_balls INTEGER,
  tackles             INTEGER,
  tackles_won         INTEGER,
  interceptions       INTEGER,
  clearances          INTEGER,
  saves               INTEGER,
  offsides            INTEGER,
  corners             INTEGER,
  fouls               INTEGER,
  yellow_cards        INTEGER,
  red_cards           INTEGER,
  PRIMARY KEY (match_id, side)
);

-- kind: goal | own_goal | penalty_goal | penalty_miss | yellow |
-- second_yellow | red | sub | half_time | full_time. other is the assist,
-- or the player going off. Player IDs are canonical when known.
CREATE TABLE match_event (
  match_id  TEXT NOT NULL REFERENCES match_detail(match_id),
  seq       INTEGER NOT NULL,
  kind      TEXT NOT NULL,
  side      TEXT NOT NULL,     -- 'home', 'away', or '' for a whistle
  period    INTEGER NOT NULL,
  minute    INTEGER NOT NULL,
  added     INTEGER NOT NULL,
  player    TEXT,
  player_id TEXT,
  other     TEXT,
  other_id  TEXT,
  text      TEXT,
  PRIMARY KEY (match_id, seq)
);

CREATE TABLE match_lineup (
  match_id  TEXT NOT NULL REFERENCES match_detail(match_id),
  side      TEXT NOT NULL CHECK (side IN ('home', 'away')),
  seq       INTEGER NOT NULL,  -- starters by formation place, then the bench
  name      TEXT NOT NULL,
  player_id TEXT,
  shirt     TEXT,
  position  TEXT,
  place     INTEGER NOT NULL,  -- formation place, 0 on the bench
  starter   INTEGER NOT NULL,
  on_min    INTEGER NOT NULL,  -- 0 when not subbed on
  off_min   INTEGER NOT NULL,  -- 0 when not subbed off
  goals     INTEGER NOT NULL,
  assists   INTEGER NOT NULL,
  own_goals INTEGER NOT NULL,
  yellow    INTEGER NOT NULL,
  red       INTEGER NOT NULL,
  saves     INTEGER NOT NULL,
  PRIMARY KEY (match_id, side, seq)
);

CREATE TABLE match_commentary (
  match_id TEXT NOT NULL REFERENCES match_detail(match_id),
  seq      INTEGER NOT NULL,
  clock    TEXT NOT NULL,      -- "45+2'", '' before kick-off
  kind     TEXT NOT NULL,      -- the play ('Goal', 'Foul'), '' for narration
  text     TEXT NOT NULL,
  PRIMARY KEY (match_id, seq)
);
