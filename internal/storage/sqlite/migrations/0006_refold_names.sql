-- The search fold now spells out letters NFD can't decompose (Ø, ß, Ł,
-- Đ, ı …), so "odegaard" finds Martin Ødegaard. Folding is done in Go,
-- not SQL, so this clears the stored copies; Open's backfill refills every
-- NULL with the new fold the first time the database is opened.
UPDATE team SET name_folded = NULL;
UPDATE player SET name_folded = NULL;
UPDATE competition SET name_folded = NULL;
