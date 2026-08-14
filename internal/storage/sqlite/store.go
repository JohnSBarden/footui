package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

// Store implements storage.Repositories against a SQLite database. It is
// the only package that issues SQL — the sync layer and CLI/TUI reach it
// exclusively through the storage.Repositories interface.
type Store struct {
	db *sql.DB
}

// NewStore wraps an already-open, already-migrated *sql.DB.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

var _ storage.Repositories = (*Store)(nil)

func nullFloat(f *float64) sql.NullFloat64 {
	if f == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *f, Valid: true}
}

func nullInt(i *int) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*i), Valid: true}
}

func scanNullFloat(dst **float64, v sql.NullFloat64) {
	if v.Valid {
		f := v.Float64
		*dst = &f
	}
}

func scanNullInt(dst **int, v sql.NullInt64) {
	if v.Valid {
		i := int(v.Int64)
		*dst = &i
	}
}

func nullIntFromZero(i int) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(i), Valid: true}
}

func timePtrToNullString(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: t.Format("2006-01-02"), Valid: true}
}

// The sqlite driver stores time.Time as TEXT but doesn't scan TEXT back
// into *time.Time on read, so timestamps are formatted/parsed explicitly
// as RFC3339 on the way in and out rather than passed as time.Time values.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// --- CompetitionsRepo ---

func (s *Store) UpsertCompetition(ctx context.Context, c models.Competition) error {
	format := c.Format
	if format == "" {
		format = "league"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO competition (id, name, country, tier, gender, format)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, country = excluded.country,
			tier = excluded.tier, gender = excluded.gender, format = excluded.format`,
		c.ID, c.Name, c.Country, c.Tier, c.Gender, format)
	if err != nil {
		return fmt.Errorf("sqlite: upsert competition %s: %w", c.ID, err)
	}
	return nil
}

func (s *Store) ListCompetitions(ctx context.Context) ([]models.Competition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, country, tier, gender, format FROM competition ORDER BY country, tier`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list competitions: %w", err)
	}
	defer rows.Close()

	var out []models.Competition
	for rows.Next() {
		var c models.Competition
		if err := rows.Scan(&c.ID, &c.Name, &c.Country, &c.Tier, &c.Gender, &c.Format); err != nil {
			return nil, fmt.Errorf("sqlite: scan competition: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- SeasonsRepo ---

func (s *Store) UpsertSeason(ctx context.Context, se models.Season) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO season (id, competition_id, label, start_date, end_date)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET label = excluded.label,
			start_date = excluded.start_date, end_date = excluded.end_date`,
		se.ID, se.CompetitionID, se.Label, timePtrToNullString(se.StartDate), timePtrToNullString(se.EndDate))
	if err != nil {
		return fmt.Errorf("sqlite: upsert season %s: %w", se.ID, err)
	}
	return nil
}

// --- TeamsRepo ---

func (s *Store) UpsertTeam(ctx context.Context, t models.Team) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO team (id, name, short_name, country, gender, founded_year, venue)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, short_name = excluded.short_name,
			country = excluded.country, gender = excluded.gender,
			founded_year = excluded.founded_year, venue = excluded.venue`,
		t.ID, t.Name, t.ShortName, t.Country, t.Gender, nullIntFromZero(t.FoundedYear), t.Venue)
	if err != nil {
		return fmt.Errorf("sqlite: upsert team %s: %w", t.ID, err)
	}
	return nil
}

func (s *Store) FindTeamsByName(ctx context.Context, query string) ([]models.Team, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, short_name, country, gender, founded_year, venue
		FROM team WHERE name LIKE '%' || ? || '%' ORDER BY name`, query)
	if err != nil {
		return nil, fmt.Errorf("sqlite: find teams by name %q: %w", query, err)
	}
	defer rows.Close()

	var out []models.Team
	for rows.Next() {
		var t models.Team
		var founded sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Name, &t.ShortName, &t.Country, &t.Gender, &founded, &t.Venue); err != nil {
			return nil, fmt.Errorf("sqlite: scan team: %w", err)
		}
		if founded.Valid {
			t.FoundedYear = int(founded.Int64)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- TeamStatsRepo ---

func (s *Store) LinkTeamCompetitionSeason(ctx context.Context, teamID, competitionID, seasonID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO team_competition_season (team_id, competition_id, season_id)
		VALUES (?, ?, ?)
		ON CONFLICT(team_id, competition_id, season_id) DO NOTHING`,
		teamID, competitionID, seasonID)
	if err != nil {
		return fmt.Errorf("sqlite: link team_competition_season %s/%s/%s: %w", teamID, competitionID, seasonID, err)
	}
	return nil
}

func (s *Store) UpsertTeamSeasonStats(ctx context.Context, st models.TeamSeasonStats) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO team_season_stats (
			team_id, competition_id, season_id, position, matches_played, wins, draws, losses,
			goals_for, goals_against, goal_diff, points, possession_avg, fouls_committed,
			fouls_suffered, yellow_cards, red_cards, shots, shots_on_target, corners, xg, xga,
			source, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(team_id, competition_id, season_id) DO UPDATE SET
			position = excluded.position, matches_played = excluded.matches_played,
			wins = excluded.wins, draws = excluded.draws, losses = excluded.losses,
			goals_for = excluded.goals_for, goals_against = excluded.goals_against,
			goal_diff = excluded.goal_diff, points = excluded.points,
			possession_avg = COALESCE(excluded.possession_avg, team_season_stats.possession_avg),
			fouls_committed = COALESCE(excluded.fouls_committed, team_season_stats.fouls_committed),
			fouls_suffered = COALESCE(excluded.fouls_suffered, team_season_stats.fouls_suffered),
			yellow_cards = COALESCE(excluded.yellow_cards, team_season_stats.yellow_cards),
			red_cards = COALESCE(excluded.red_cards, team_season_stats.red_cards),
			shots = COALESCE(excluded.shots, team_season_stats.shots),
			shots_on_target = COALESCE(excluded.shots_on_target, team_season_stats.shots_on_target),
			corners = COALESCE(excluded.corners, team_season_stats.corners),
			xg = COALESCE(excluded.xg, team_season_stats.xg),
			xga = COALESCE(excluded.xga, team_season_stats.xga),
			source = excluded.source, updated_at = excluded.updated_at`,
		st.TeamID, st.CompetitionID, st.SeasonID, nullInt(st.Position), st.MatchesPlayed,
		st.Wins, st.Draws, st.Losses, st.GoalsFor, st.GoalsAgainst, st.GoalDiff, st.Points,
		nullFloat(st.PossessionAvg), nullInt(st.FoulsCommitted), nullInt(st.FoulsSuffered),
		nullInt(st.YellowCards), nullInt(st.RedCards), nullInt(st.Shots), nullInt(st.ShotsOnTarget),
		nullInt(st.Corners), nullFloat(st.XG), nullFloat(st.XGA), st.Source, formatTime(st.UpdatedAt))
	if err != nil {
		return fmt.Errorf("sqlite: upsert team_season_stats %s/%s/%s: %w", st.TeamID, st.CompetitionID, st.SeasonID, err)
	}
	return nil
}

func (s *Store) Standings(ctx context.Context, competitionID, seasonID string) ([]models.TeamSeasonStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT team_id, competition_id, season_id, position, matches_played, wins, draws, losses,
			goals_for, goals_against, goal_diff, points, possession_avg, fouls_committed,
			fouls_suffered, yellow_cards, red_cards, shots, shots_on_target, corners, xg, xga,
			source, updated_at
		FROM team_season_stats
		WHERE competition_id = ? AND season_id = ?
		ORDER BY position IS NULL, position`, competitionID, seasonID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: standings %s/%s: %w", competitionID, seasonID, err)
	}
	defer rows.Close()
	return scanTeamSeasonStats(rows)
}

func (s *Store) TeamHistory(ctx context.Context, teamID string) ([]models.TeamSeasonStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT team_id, competition_id, season_id, position, matches_played, wins, draws, losses,
			goals_for, goals_against, goal_diff, points, possession_avg, fouls_committed,
			fouls_suffered, yellow_cards, red_cards, shots, shots_on_target, corners, xg, xga,
			source, updated_at
		FROM team_season_stats
		WHERE team_id = ?
		ORDER BY season_id DESC, competition_id`, teamID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: team history %s: %w", teamID, err)
	}
	defer rows.Close()
	return scanTeamSeasonStats(rows)
}

func scanTeamSeasonStats(rows *sql.Rows) ([]models.TeamSeasonStats, error) {
	var out []models.TeamSeasonStats
	for rows.Next() {
		var st models.TeamSeasonStats
		var position sql.NullInt64
		var possession sql.NullFloat64
		var fc, fs, yc, rc, shots, sot, corners sql.NullInt64
		var xg, xga sql.NullFloat64
		var updatedAt string
		if err := rows.Scan(&st.TeamID, &st.CompetitionID, &st.SeasonID, &position, &st.MatchesPlayed,
			&st.Wins, &st.Draws, &st.Losses, &st.GoalsFor, &st.GoalsAgainst, &st.GoalDiff, &st.Points,
			&possession, &fc, &fs, &yc, &rc, &shots, &sot, &corners, &xg, &xga,
			&st.Source, &updatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan team_season_stats: %w", err)
		}
		parsed, err := parseTime(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite: parse team_season_stats.updated_at %q: %w", updatedAt, err)
		}
		st.UpdatedAt = parsed
		scanNullInt(&st.Position, position)
		scanNullFloat(&st.PossessionAvg, possession)
		scanNullInt(&st.FoulsCommitted, fc)
		scanNullInt(&st.FoulsSuffered, fs)
		scanNullInt(&st.YellowCards, yc)
		scanNullInt(&st.RedCards, rc)
		scanNullInt(&st.Shots, shots)
		scanNullInt(&st.ShotsOnTarget, sot)
		scanNullInt(&st.Corners, corners)
		scanNullFloat(&st.XG, xg)
		scanNullFloat(&st.XGA, xga)
		out = append(out, st)
	}
	return out, rows.Err()
}

// --- PlayersRepo ---

func (s *Store) UpsertPlayer(ctx context.Context, p models.Player) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO player (id, name, date_of_birth, nationality, position, gender)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, date_of_birth = excluded.date_of_birth,
			nationality = excluded.nationality, position = excluded.position, gender = excluded.gender`,
		p.ID, p.Name, timePtrToNullString(p.DateOfBirth), p.Nationality, p.Position, p.Gender)
	if err != nil {
		return fmt.Errorf("sqlite: upsert player %s: %w", p.ID, err)
	}
	return nil
}

// --- PlayerStatsRepo ---

func (s *Store) UpsertPlayerSeasonStats(ctx context.Context, st models.PlayerSeasonStats) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO player_season_stats (
			player_id, team_id, competition_id, season_id, appearances, minutes, goals, assists,
			xg, xa, yellow_cards, red_cards, source, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(player_id, team_id, competition_id, season_id) DO UPDATE SET
			appearances = excluded.appearances, minutes = excluded.minutes, goals = excluded.goals,
			assists = excluded.assists, xg = excluded.xg, xa = excluded.xa,
			yellow_cards = excluded.yellow_cards, red_cards = excluded.red_cards,
			source = excluded.source, updated_at = excluded.updated_at`,
		st.PlayerID, st.TeamID, st.CompetitionID, st.SeasonID, st.Appearances, st.Minutes,
		st.Goals, st.Assists, nullFloat(st.XG), nullFloat(st.XA), st.YellowCards, st.RedCards,
		st.Source, formatTime(st.UpdatedAt))
	if err != nil {
		return fmt.Errorf("sqlite: upsert player_season_stats %s/%s: %w", st.PlayerID, st.TeamID, err)
	}
	return nil
}

func (s *Store) RosterForTeamSeason(ctx context.Context, teamID, competitionID, seasonID string) ([]models.PlayerSeasonStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT player_id, team_id, competition_id, season_id, appearances, minutes, goals, assists,
			xg, xa, yellow_cards, red_cards, source, updated_at
		FROM player_season_stats
		WHERE team_id = ? AND competition_id = ? AND season_id = ?
		ORDER BY minutes DESC`, teamID, competitionID, seasonID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: roster %s/%s/%s: %w", teamID, competitionID, seasonID, err)
	}
	defer rows.Close()

	var out []models.PlayerSeasonStats
	for rows.Next() {
		var st models.PlayerSeasonStats
		var xg, xa sql.NullFloat64
		var updatedAt string
		if err := rows.Scan(&st.PlayerID, &st.TeamID, &st.CompetitionID, &st.SeasonID, &st.Appearances,
			&st.Minutes, &st.Goals, &st.Assists, &xg, &xa, &st.YellowCards, &st.RedCards,
			&st.Source, &updatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan player_season_stats: %w", err)
		}
		parsed, err := parseTime(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite: parse player_season_stats.updated_at %q: %w", updatedAt, err)
		}
		st.UpdatedAt = parsed
		scanNullFloat(&st.XG, xg)
		scanNullFloat(&st.XA, xa)
		out = append(out, st)
	}
	return out, rows.Err()
}
