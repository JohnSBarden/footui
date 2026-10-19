-- Friendlies were abbreviated "FRI", which reads as Friday beside a match
-- date on a nation's card. New syncs write "FRND"; this renames the rows
-- already cached, which a nation refresh might not rewrite for days.
UPDATE competition SET short_name = 'FRND'
WHERE id IN ('int-friendly', 'int-friendly-women') AND short_name = 'FRI';
