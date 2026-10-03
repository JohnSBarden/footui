-- Favourites: the competitions, teams and players the user has starred.
-- Keyed by canonical ID, which identity resolution keeps stable across
-- providers, so a favourite follows a club wherever it is fetched from.
-- No foreign keys: a favourite outlives its rows being re-fetched, and one
-- whose entity isn't cached any more is shown as such rather than being
-- silently deleted.
CREATE TABLE favorite (
  entity_type TEXT NOT NULL CHECK (entity_type IN ('competition', 'team', 'player')),
  entity_id   TEXT NOT NULL,
  added_at    TEXT NOT NULL,
  PRIMARY KEY (entity_type, entity_id)
);
