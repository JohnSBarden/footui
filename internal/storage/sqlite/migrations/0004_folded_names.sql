-- Search columns holding a lowercase, accent-stripped copy of each name.
--
-- SQLite's LIKE is case-insensitive only for ASCII and has no notion of
-- accent equivalence, so searching "martin" could not find "Martín
-- Zubimendi" and "jurrien" could not find "Jurriën Timber" at all. Folding
-- is done in Go (internal/textfold) on the way in; these columns are never
-- displayed.
ALTER TABLE team ADD COLUMN name_folded TEXT;
ALTER TABLE player ADD COLUMN name_folded TEXT;
ALTER TABLE competition ADD COLUMN name_folded TEXT;

CREATE INDEX idx_team_name_folded ON team(name_folded);
CREATE INDEX idx_player_name_folded ON player(name_folded);
