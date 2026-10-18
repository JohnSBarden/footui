package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/JohnSBarden/footui/internal/data/models"
)

func (s *Store) UpsertPlayerSeasonDetail(ctx context.Context, d models.PlayerSeasonDetail) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO player_season_detail (
			player_id, team_id, competition_id, season_id, position, appearances, minutes,
			clean_sheets, goals_conceded, saves, tackles, interceptions, clearances, pass_pct, fetched_at,
			shots_on_target
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(player_id, team_id, competition_id, season_id) DO UPDATE SET
			position = excluded.position, appearances = excluded.appearances, minutes = excluded.minutes,
			clean_sheets = excluded.clean_sheets, goals_conceded = excluded.goals_conceded,
			saves = excluded.saves, tackles = excluded.tackles, interceptions = excluded.interceptions,
			clearances = excluded.clearances, pass_pct = excluded.pass_pct, fetched_at = excluded.fetched_at,
			shots_on_target = excluded.shots_on_target`,
		d.PlayerID, d.TeamID, d.CompetitionID, d.SeasonID, nullString(d.Position),
		nullInt(d.Appearances), nullInt(d.Minutes), nullInt(d.CleanSheets), nullInt(d.GoalsConceded),
		nullInt(d.Saves), nullInt(d.Tackles), nullInt(d.Interceptions), nullInt(d.Clearances),
		nullFloat(d.PassPct), formatTime(d.FetchedAt), nullInt(d.ShotsOnTarget))
	if err != nil {
		return fmt.Errorf("sqlite: upsert player_season_detail %s/%s/%s: %w", d.PlayerID, d.CompetitionID, d.SeasonID, err)
	}
	return nil
}

func (s *Store) PlayerSeasonDetails(ctx context.Context, playerID string) ([]models.PlayerSeasonDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT player_id, team_id, competition_id, season_id, position, appearances, minutes,
			clean_sheets, goals_conceded, saves, tackles, interceptions, clearances, pass_pct, fetched_at,
			shots_on_target
		FROM player_season_detail
		WHERE player_id = ?
		ORDER BY season_id DESC, competition_id`, playerID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: player season detail %s: %w", playerID, err)
	}
	defer rows.Close()

	var out []models.PlayerSeasonDetail
	for rows.Next() {
		var d models.PlayerSeasonDetail
		var position sql.NullString
		var apps, mins, cs, ga, saves, tkl, ints, clr, sot sql.NullInt64
		var pass sql.NullFloat64
		var fetched string
		if err := rows.Scan(&d.PlayerID, &d.TeamID, &d.CompetitionID, &d.SeasonID, &position,
			&apps, &mins, &cs, &ga, &saves, &tkl, &ints, &clr, &pass, &fetched, &sot); err != nil {
			return nil, fmt.Errorf("sqlite: scan player_season_detail: %w", err)
		}
		d.Position = position.String
		for _, f := range []struct {
			dst **int
			v   sql.NullInt64
		}{
			{&d.Appearances, apps}, {&d.Minutes, mins}, {&d.CleanSheets, cs}, {&d.GoalsConceded, ga},
			{&d.Saves, saves}, {&d.Tackles, tkl}, {&d.Interceptions, ints}, {&d.Clearances, clr},
			{&d.ShotsOnTarget, sot},
		} {
			scanNullInt(f.dst, f.v)
		}
		scanNullFloat(&d.PassPct, pass)
		if d.FetchedAt, err = parseTime(fetched); err != nil {
			return nil, fmt.Errorf("sqlite: parse player_season_detail.fetched_at %q: %w", fetched, err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) SeasonPlayerLines(ctx context.Context, competitionID, seasonID string) ([]models.PlayerSeasonStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+playerStatsColumns+`
		FROM player_season_stats
		WHERE competition_id = ? AND season_id = ?`, competitionID, seasonID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: season player lines %s/%s: %w", competitionID, seasonID, err)
	}
	defer rows.Close()
	return scanPlayerSeasonStats(rows)
}
