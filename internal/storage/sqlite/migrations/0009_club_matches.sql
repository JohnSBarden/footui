-- Club matches: a club's recent games come from the match table too —
-- understat's league matches with each side's expected goals, and ESPN's
-- leagues, European competitions and domestic cups. Only xG is new; a
-- domestic cup is a competition with category 'cup'.
ALTER TABLE match_team_stats ADD COLUMN xg REAL;
