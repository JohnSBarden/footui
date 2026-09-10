-- Advanced per-player stats that understat.com already returns in the same
-- payload as the basics. They live as real columns rather than in a
-- key/value table because every plausible provider has them: they are core
-- advanced stats, not a source-specific long tail.
ALTER TABLE player_season_stats ADD COLUMN shots INTEGER;
ALTER TABLE player_season_stats ADD COLUMN key_passes INTEGER;
ALTER TABLE player_season_stats ADD COLUMN non_penalty_goals INTEGER;
ALTER TABLE player_season_stats ADD COLUMN npxg REAL;

-- The roster query filters by (team, competition, season), which the
-- table's primary key (player_id first) can't serve.
CREATE INDEX idx_player_stats_team_season
  ON player_season_stats(team_id, competition_id, season_id);
