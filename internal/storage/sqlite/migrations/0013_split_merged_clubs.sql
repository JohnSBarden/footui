-- A club's name could match another club's loosely enough to be taken for
-- it: Miami FC as Inter Miami ("miami" is in "inter miami"), Atlanta
-- United 2 as Atlanta United (the digit is dropped). ESPN, which uses one
-- ID per club everywhere, had already said they were different clubs, and
-- resolution now listens. A nation is left alone: ESPN lists some twice
-- under one name.

-- Their games came in with yearly match lists, which are only fetched
-- again when due. Forgetting when each list holding one of them was
-- fetched makes it due, so the next refresh files the games under the
-- right clubs.
DELETE FROM sync_log
WHERE year IS NOT NULL
  AND (competition_id, year) IN (
    SELECT m.competition_id, CAST(strftime('%Y', m.kickoff) AS INTEGER)
    FROM match m
    JOIN (
      SELECT canonical_id FROM external_ref
      WHERE entity_type = 'team'
        AND canonical_id IN (SELECT id FROM team WHERE national = 0)
      GROUP BY canonical_id, provider
      HAVING COUNT(*) > 1
    ) merged ON merged.canonical_id IN (m.home_team_id, m.away_team_id)
  );

-- Then each such club's second mapping from the same provider goes,
-- keeping the first: the one the club was created for, as the second was
-- only ever matched onto it. The next refresh of the seasons the other
-- club played resolves it to a club of its own, and replaces the rows
-- filed under the wrong one.
DELETE FROM external_ref
WHERE entity_type = 'team'
  AND canonical_id IN (SELECT id FROM team WHERE national = 0)
  AND rowid NOT IN (
    SELECT MIN(rowid) FROM external_ref
    WHERE entity_type = 'team'
    GROUP BY canonical_id, provider
  );
