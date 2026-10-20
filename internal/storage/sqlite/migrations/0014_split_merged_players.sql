-- A player was matched by name within everyone who ever played for the
-- club, so a 2014 "Xavi" became 2026's Xavi Espart, and three Daniels who
-- kept goal for FC Dallas and San Jose became one. Resolution now refuses
-- a player the provider knows by another ID (unless the name is the same
-- words) or with another birth date.
--
-- A player's canonical ID is the provider ID it was created for
-- ("espn-243832"), so its other mappings from that same provider are the
-- ones matched onto it, and they go. A refresh of the seasons they played
-- resolves each again: to a player of their own, or back to this one when
-- the provider merely listed them twice. Mappings from another provider
-- are left alone.

-- What was fetched for those players by an ID that may have been another
-- player's goes too, and is fetched again when they are next opened.
DELETE FROM player_season_detail
WHERE player_id IN (
  SELECT canonical_id FROM external_ref
  WHERE entity_type = 'player'
    AND canonical_id LIKE provider || '-%'
    AND canonical_id != provider || '-' || provider_id
);

DELETE FROM external_ref
WHERE entity_type = 'player'
  AND canonical_id LIKE provider || '-%'
  AND canonical_id != provider || '-' || provider_id;
