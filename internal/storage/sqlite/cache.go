package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

// CacheSummary reports what's cached per competition.
//
// The counts come from correlated subqueries rather than joins because
// joining season, team_competition_season, and player_season_stats in one
// statement multiplies rows against each other — a competition with 3
// seasons and 20 teams would report 60 teams. Subqueries keep each count
// answering its own question.
func (s *Store) CacheSummary(ctx context.Context) ([]models.CompetitionCache, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.name, c.country, c.tier, c.gender, c.format, c.calendar_year,
			(SELECT COUNT(*) FROM season se WHERE se.competition_id = c.id),
			(SELECT MIN(label) FROM season se WHERE se.competition_id = c.id),
			(SELECT MAX(label) FROM season se WHERE se.competition_id = c.id),
			(SELECT COUNT(DISTINCT tcs.team_id) FROM team_competition_season tcs
				WHERE tcs.competition_id = c.id),
			(SELECT COUNT(*) FROM player_season_stats pss WHERE pss.competition_id = c.id)
		FROM competition c
		ORDER BY c.country, c.tier, c.name`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: cache summary: %w", err)
	}
	defer rows.Close()

	var out []models.CompetitionCache
	for rows.Next() {
		var e models.CompetitionCache
		var earliest, latest sql.NullString
		if err := rows.Scan(&e.Competition.ID, &e.Competition.Name, &e.Competition.Country,
			&e.Competition.Tier, &e.Competition.Gender, &e.Competition.Format, &e.Competition.CalendarYear,
			&e.Seasons, &earliest, &latest, &e.Teams, &e.PlayerSeasons); err != nil {
			return nil, fmt.Errorf("sqlite: scan cache summary: %w", err)
		}
		e.EarliestSeason = earliest.String
		e.LatestSeason = latest.String
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// The latest sync per competition is a separate lookup: folding it
	// into the query above would need a correlated ORDER BY ... LIMIT 1
	// per column, and there are only ever a handful of competitions.
	for i := range out {
		run, err := s.latestSyncForCompetition(ctx, out[i].Competition.ID)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return nil, err
		}
		out[i].LastSync = &run
	}

	return out, nil
}

func (s *Store) latestSyncForCompetition(ctx context.Context, competitionID string) (models.SyncRun, error) {
	var r models.SyncRun
	var startedAt, finishedAt string
	var errText sql.NullString
	var seasonID sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, provider, competition_id, season_id, started_at, finished_at,
			status, rows_upserted, error
		FROM sync_log WHERE competition_id = ?
		ORDER BY finished_at DESC, id DESC LIMIT 1`, competitionID).
		Scan(&r.ID, &r.Provider, &r.CompetitionID, &seasonID, &startedAt, &finishedAt,
			&r.Status, &r.RowsUpserted, &errText)
	if errors.Is(err, sql.ErrNoRows) {
		return r, fmt.Errorf("sqlite: sync_log %s: %w", competitionID, storage.ErrNotFound)
	}
	if err != nil {
		return r, fmt.Errorf("sqlite: latest sync for %s: %w", competitionID, err)
	}

	r.SeasonID = seasonID.String
	if t, err := parseTime(startedAt); err == nil {
		r.StartedAt = t
	}
	if t, err := parseTime(finishedAt); err == nil {
		r.FinishedAt = t
	}
	r.Error = errText.String
	return r, nil
}
