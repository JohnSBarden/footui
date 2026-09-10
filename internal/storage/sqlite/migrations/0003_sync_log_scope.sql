-- sync_log originally recorded only that *a* sync ran. Staleness is a
-- per-competition-per-season question ("how old is this table?"), so the
-- scope of each run is recorded alongside it.
ALTER TABLE sync_log ADD COLUMN competition_id TEXT;
ALTER TABLE sync_log ADD COLUMN season_id TEXT;

-- The staleness lookup is "the most recent run for this competition and
-- season", which needs both columns and the ordering key.
CREATE INDEX idx_sync_log_scope ON sync_log(competition_id, season_id, finished_at DESC);
