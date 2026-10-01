-- Competitions whose seasons run within one calendar year (MLS, NWSL)
-- rather than across two (the European "2025-26"). Season labels stay the
-- bare start year either way; this flag is what tells display code to
-- show "2026" instead of "2026-27".
ALTER TABLE competition ADD COLUMN calendar_year INTEGER NOT NULL DEFAULT 0;
