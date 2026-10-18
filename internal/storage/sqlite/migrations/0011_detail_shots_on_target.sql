-- Shots on target, for a forward's accuracy: understat has total shots
-- but not how many were on target.
ALTER TABLE player_season_detail ADD COLUMN shots_on_target INTEGER;
