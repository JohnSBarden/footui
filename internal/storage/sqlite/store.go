package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/textfold"
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

// nullString maps "" to SQL NULL, keeping "no secondary color recorded"
// distinct from an empty string in the column.
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
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

// inClause builds the placeholder list and argument slice for an
// `IN (...)` filter. Callers must guard against an empty id list before
// calling: `IN ()` is not valid SQL.
func inClause(ids []string) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(ids)), ","), args
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

// parseDatePtr converts a nullable DATE column back to *time.Time,
// mirroring timePtrToNullString on the write side. An unparseable value is
// treated as absent: season start/end dates are cosmetic, and a malformed
// one shouldn't fail an otherwise good query.
func parseDatePtr(v sql.NullString) *time.Time {
	if !v.Valid {
		return nil
	}
	t, err := time.Parse("2006-01-02", v.String)
	if err != nil {
		return nil
	}
	return &t
}

// --- CompetitionsRepo ---

func (s *Store) UpsertCompetition(ctx context.Context, c models.Competition) error {
	format := c.Format
	if format == "" {
		format = "league"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO competition (id, name, name_folded, country, tier, gender, format, calendar_year,
			category, short_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, name_folded = excluded.name_folded,
			country = excluded.country, tier = excluded.tier, gender = excluded.gender,
			format = excluded.format, calendar_year = excluded.calendar_year,
			category = excluded.category, short_name = excluded.short_name`,
		c.ID, c.Name, textfold.Fold(c.Name), c.Country, c.Tier, c.Gender, format, c.CalendarYear,
		nullString(c.Category), nullString(c.ShortName))
	if err != nil {
		return fmt.Errorf("sqlite: upsert competition %s: %w", c.ID, err)
	}
	return nil
}

func (s *Store) ListCompetitions(ctx context.Context) ([]models.Competition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+competitionColumns+` FROM competition ORDER BY country, tier, name`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list competitions: %w", err)
	}
	defer rows.Close()
	return scanCompetitions(rows)
}

const competitionColumns = `id, name, country, tier, gender, format, calendar_year,
	COALESCE(category, ''), COALESCE(short_name, '')`

func scanCompetition(sc interface{ Scan(...any) error }) (models.Competition, error) {
	var c models.Competition
	err := sc.Scan(&c.ID, &c.Name, &c.Country, &c.Tier, &c.Gender, &c.Format, &c.CalendarYear,
		&c.Category, &c.ShortName)
	return c, err
}

func scanCompetitions(rows *sql.Rows) ([]models.Competition, error) {
	var out []models.Competition
	for rows.Next() {
		c, err := scanCompetition(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan competition: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetCompetition(ctx context.Context, id string) (models.Competition, error) {
	c, err := scanCompetition(s.db.QueryRowContext(ctx, `
		SELECT `+competitionColumns+` FROM competition WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, fmt.Errorf("sqlite: competition %s: %w", id, storage.ErrNotFound)
	}
	if err != nil {
		return c, fmt.Errorf("sqlite: get competition %s: %w", id, err)
	}
	return c, nil
}

func (s *Store) FindCompetitionsByName(ctx context.Context, query string) ([]models.Competition, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+competitionColumns+` FROM competition
		WHERE name_folded LIKE '%' || ? || '%' OR id LIKE '%' || ? || '%'
		ORDER BY country, tier, name`, textfold.Fold(query), textfold.Fold(query))
	if err != nil {
		return nil, fmt.Errorf("sqlite: find competitions by name %q: %w", query, err)
	}
	defer rows.Close()
	return scanCompetitions(rows)
}

func (s *Store) GetCompetitions(ctx context.Context, ids []string) (map[string]models.Competition, error) {
	out := make(map[string]models.Competition, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	placeholders, args := inClause(ids)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+competitionColumns+`
		FROM competition WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: get competitions: %w", err)
	}
	defer rows.Close()

	comps, err := scanCompetitions(rows)
	for _, c := range comps {
		out[c.ID] = c
	}
	return out, err
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

// ListSeasons returns a competition's seasons most recent first. Season
// labels are start years ("2025"), which sort correctly as text, so
// ordering by label descending is the same as ordering by recency.
func (s *Store) ListSeasons(ctx context.Context, competitionID string) ([]models.Season, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT se.id, se.competition_id, se.label, se.start_date, se.end_date,
			COALESCE(c.calendar_year, 0)
		FROM season se LEFT JOIN competition c ON c.id = se.competition_id
		WHERE se.competition_id = ? ORDER BY se.label DESC`, competitionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list seasons %s: %w", competitionID, err)
	}
	defer rows.Close()

	var out []models.Season
	for rows.Next() {
		var se models.Season
		var start, end sql.NullString
		if err := rows.Scan(&se.ID, &se.CompetitionID, &se.Label, &start, &end, &se.CalendarYear); err != nil {
			return nil, fmt.Errorf("sqlite: scan season: %w", err)
		}
		se.StartDate = parseDatePtr(start)
		se.EndDate = parseDatePtr(end)
		out = append(out, se)
	}
	return out, rows.Err()
}

func (s *Store) GetSeasons(ctx context.Context, ids []string) (map[string]models.Season, error) {
	out := make(map[string]models.Season, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	placeholders, args := inClause(ids)
	rows, err := s.db.QueryContext(ctx, `
		SELECT se.id, se.competition_id, se.label, se.start_date, se.end_date,
			COALESCE(c.calendar_year, 0)
		FROM season se LEFT JOIN competition c ON c.id = se.competition_id
		WHERE se.id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: get seasons: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var se models.Season
		var start, end sql.NullString
		if err := rows.Scan(&se.ID, &se.CompetitionID, &se.Label, &start, &end, &se.CalendarYear); err != nil {
			return nil, fmt.Errorf("sqlite: scan season: %w", err)
		}
		se.StartDate = parseDatePtr(start)
		se.EndDate = parseDatePtr(end)
		out[se.ID] = se
	}
	return out, rows.Err()
}

// --- TeamsRepo ---

func (s *Store) UpsertTeam(ctx context.Context, t models.Team) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO team (id, name, name_folded, short_name, country, gender, founded_year, venue, national)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, name_folded = excluded.name_folded,
			short_name = excluded.short_name, country = excluded.country,
			gender = excluded.gender, founded_year = excluded.founded_year, venue = excluded.venue,
			national = excluded.national`,
		t.ID, t.Name, textfold.Fold(t.Name), t.ShortName, t.Country, t.Gender,
		nullIntFromZero(t.FoundedYear), t.Venue, t.National)
	if err != nil {
		return fmt.Errorf("sqlite: upsert team %s: %w", t.ID, err)
	}
	return nil
}

func (s *Store) FindTeamsByName(ctx context.Context, query string) ([]models.Team, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+teamColumns+`
		FROM team WHERE name_folded LIKE '%' || ? || '%' ORDER BY name`, textfold.Fold(query))
	if err != nil {
		return nil, fmt.Errorf("sqlite: find teams by name %q: %w", query, err)
	}
	defer rows.Close()

	var out []models.Team
	for rows.Next() {
		t, err := scanTeam(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan team: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetTeam(ctx context.Context, id string) (models.Team, error) {
	t, err := scanTeam(s.db.QueryRowContext(ctx, `
		SELECT `+teamColumns+`
		FROM team WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return t, fmt.Errorf("sqlite: team %s: %w", id, storage.ErrNotFound)
	}
	if err != nil {
		return t, fmt.Errorf("sqlite: get team %s: %w", id, err)
	}
	return t, nil
}

const teamColumns = `id, name, COALESCE(short_name, ''), COALESCE(country, ''), gender, founded_year,
	COALESCE(venue, ''), national`

func scanTeam(sc interface{ Scan(...any) error }) (models.Team, error) {
	var t models.Team
	var founded sql.NullInt64
	err := sc.Scan(&t.ID, &t.Name, &t.ShortName, &t.Country, &t.Gender, &founded, &t.Venue, &t.National)
	if founded.Valid {
		t.FoundedYear = int(founded.Int64)
	}
	return t, err
}

// GetTeams resolves a batch of team IDs in one query. IDs with no cached
// team row are absent from the returned map rather than an error —
// a stats row can outlive the team row it references only if the database
// was hand-edited, and a screen should render "unknown team" instead of
// failing the whole view.
func (s *Store) GetTeams(ctx context.Context, ids []string) (map[string]models.Team, error) {
	out := make(map[string]models.Team, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	placeholders, args := inClause(ids)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+teamColumns+`
		FROM team WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: get teams: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		t, err := scanTeam(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan team: %w", err)
		}
		out[t.ID] = t
	}
	return out, rows.Err()
}

func (s *Store) ListTeams(ctx context.Context) ([]models.Team, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+teamColumns+`
		FROM team ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list teams: %w", err)
	}
	defer rows.Close()

	var out []models.Team
	for rows.Next() {
		t, err := scanTeam(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan team: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- SyncLogRepo ---

func (s *Store) RecordSyncRun(ctx context.Context, r models.SyncRun) error {
	// A run is either one season of a competition or one calendar year of
	// its matches; the two are kept apart so a year's fetch never passes
	// for a season's freshness.
	entity := "competition_season"
	if r.Year != 0 {
		entity = "competition_year"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sync_log (provider, entity_type, competition_id, season_id, year,
			started_at, finished_at, status, rows_upserted, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Provider, entity, r.CompetitionID, nullString(r.SeasonID), nullIntFromZero(r.Year),
		formatTime(r.StartedAt), formatTime(r.FinishedAt),
		r.Status, r.RowsUpserted, nullString(r.Error))
	if err != nil {
		return fmt.Errorf("sqlite: record sync run %s/%s: %w", r.CompetitionID, r.SeasonID, err)
	}
	return nil
}

func (s *Store) LatestSyncRun(ctx context.Context, competitionID, seasonID string) (models.SyncRun, error) {
	var r models.SyncRun
	var startedAt, finishedAt string
	var errText sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, provider, competition_id, season_id, started_at, finished_at,
			status, rows_upserted, error
		FROM sync_log
		WHERE competition_id = ? AND season_id = ?
		ORDER BY finished_at DESC, id DESC LIMIT 1`, competitionID, seasonID).
		Scan(&r.ID, &r.Provider, &r.CompetitionID, &r.SeasonID, &startedAt, &finishedAt,
			&r.Status, &r.RowsUpserted, &errText)
	if errors.Is(err, sql.ErrNoRows) {
		return r, fmt.Errorf("sqlite: sync_log %s/%s: %w", competitionID, seasonID, storage.ErrNotFound)
	}
	if err != nil {
		return r, fmt.Errorf("sqlite: latest sync run %s/%s: %w", competitionID, seasonID, err)
	}

	if t, err := parseTime(startedAt); err == nil {
		r.StartedAt = t
	}
	if t, err := parseTime(finishedAt); err == nil {
		r.FinishedAt = t
	}
	r.Error = errText.String
	return r, nil
}

// --- TrophiesRepo ---

func (s *Store) InsertTrophy(ctx context.Context, t models.Trophy) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO trophy (team_id, competition_id, title, season_label, trophy_type, source)
		VALUES (?, ?, ?, ?, ?, ?)`,
		t.TeamID, nullString(t.CompetitionID), t.Title, nullString(t.SeasonLabel), t.TrophyType, t.Source)
	if err != nil {
		return fmt.Errorf("sqlite: insert trophy %s/%s: %w", t.TeamID, t.Title, err)
	}
	return nil
}

func (s *Store) DeleteTrophiesBySource(ctx context.Context, teamID, source string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM trophy WHERE team_id = ? AND source = ?`, teamID, source)
	if err != nil {
		return fmt.Errorf("sqlite: delete trophies %s/%s: %w", teamID, source, err)
	}
	return nil
}

func (s *Store) TeamTrophies(ctx context.Context, teamID string) ([]models.Trophy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, team_id, competition_id, title, season_label, trophy_type, source
		FROM trophy WHERE team_id = ?
		ORDER BY season_label DESC, title`, teamID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: team trophies %s: %w", teamID, err)
	}
	defer rows.Close()

	var out []models.Trophy
	for rows.Next() {
		var t models.Trophy
		var comp, season sql.NullString
		if err := rows.Scan(&t.ID, &t.TeamID, &comp, &t.Title, &season, &t.TrophyType, &t.Source); err != nil {
			return nil, fmt.Errorf("sqlite: scan trophy: %w", err)
		}
		t.CompetitionID = comp.String
		t.SeasonLabel = season.String
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- TeamColorsRepo ---

func (s *Store) UpsertTeamColors(ctx context.Context, c models.TeamColors) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO team_colors (team_id, primary_hex, secondary_hex, tertiary_hex, source)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(team_id) DO UPDATE SET primary_hex = excluded.primary_hex,
			secondary_hex = excluded.secondary_hex, tertiary_hex = excluded.tertiary_hex,
			source = excluded.source`,
		c.TeamID, c.PrimaryHex, nullString(c.SecondaryHex), nullString(c.TertiaryHex), c.Source)
	if err != nil {
		return fmt.Errorf("sqlite: upsert team_colors %s: %w", c.TeamID, err)
	}
	return nil
}

func (s *Store) GetTeamColors(ctx context.Context, teamID string) (models.TeamColors, error) {
	var c models.TeamColors
	var secondary, tertiary sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT team_id, primary_hex, secondary_hex, tertiary_hex, source
		FROM team_colors WHERE team_id = ?`, teamID).
		Scan(&c.TeamID, &c.PrimaryHex, &secondary, &tertiary, &c.Source)
	if errors.Is(err, sql.ErrNoRows) {
		return c, fmt.Errorf("sqlite: team_colors %s: %w", teamID, storage.ErrNotFound)
	}
	if err != nil {
		return c, fmt.Errorf("sqlite: get team_colors %s: %w", teamID, err)
	}
	c.SecondaryHex = secondary.String
	c.TertiaryHex = tertiary.String
	return c, nil
}

func (s *Store) TeamColorsFor(ctx context.Context, teamIDs []string) (map[string]models.TeamColors, error) {
	out := make(map[string]models.TeamColors, len(teamIDs))
	if len(teamIDs) == 0 {
		return out, nil
	}
	in, args := inClause(teamIDs)
	rows, err := s.db.QueryContext(ctx, `
		SELECT team_id, primary_hex, secondary_hex, tertiary_hex, source
		FROM team_colors WHERE team_id IN (`+in+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: team_colors for %d teams: %w", len(teamIDs), err)
	}
	defer rows.Close()
	for rows.Next() {
		var c models.TeamColors
		var secondary, tertiary sql.NullString
		if err := rows.Scan(&c.TeamID, &c.PrimaryHex, &secondary, &tertiary, &c.Source); err != nil {
			return nil, fmt.Errorf("sqlite: scan team_colors: %w", err)
		}
		c.SecondaryHex, c.TertiaryHex = secondary.String, tertiary.String
		out[c.TeamID] = c
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
			source, updated_at, group_label
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(team_id, competition_id, season_id) DO UPDATE SET
			position = excluded.position, matches_played = excluded.matches_played,
			group_label = excluded.group_label,
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
		nullInt(st.Corners), nullFloat(st.XG), nullFloat(st.XGA), st.Source, formatTime(st.UpdatedAt),
		nullString(st.GroupLabel))
	if err != nil {
		return fmt.Errorf("sqlite: upsert team_season_stats %s/%s/%s: %w", st.TeamID, st.CompetitionID, st.SeasonID, err)
	}
	return s.upsertTeamSeasonStatExtras(ctx, st)
}

// upsertTeamSeasonStatExtras persists the long-tail key/value stats. Keys
// absent from the incoming map are left alone rather than deleted, so a
// provider that supplies only some of them doesn't wipe another
// provider's contributions for the same team-season.
func (s *Store) upsertTeamSeasonStatExtras(ctx context.Context, st models.TeamSeasonStats) error {
	if len(st.Extras) == 0 {
		return nil
	}

	// Iterating a map yields keys in random order; sorting keeps the
	// statement sequence deterministic, which matters when reading a
	// query log while debugging.
	keys := make([]string, 0, len(st.Extras))
	for k := range st.Extras {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO team_season_stat_extra (team_id, competition_id, season_id, stat_key, stat_value)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(team_id, competition_id, season_id, stat_key)
			DO UPDATE SET stat_value = excluded.stat_value`,
			st.TeamID, st.CompetitionID, st.SeasonID, key, st.Extras[key])
		if err != nil {
			return fmt.Errorf("sqlite: upsert team_season_stat_extra %s/%s: %w", st.TeamID, key, err)
		}
	}
	return nil
}

func (s *Store) TeamSeasonStatExtras(ctx context.Context, teamID, competitionID, seasonID string) (map[string]float64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT stat_key, stat_value FROM team_season_stat_extra
		WHERE team_id = ? AND competition_id = ? AND season_id = ?`,
		teamID, competitionID, seasonID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: team_season_stat_extra %s/%s/%s: %w", teamID, competitionID, seasonID, err)
	}
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		var key string
		var value sql.NullFloat64
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("sqlite: scan team_season_stat_extra: %w", err)
		}
		if value.Valid {
			out[key] = value.Float64
		}
	}
	return out, rows.Err()
}

func (s *Store) SeasonStatExtra(ctx context.Context, competitionID, seasonID, key string) (map[string]float64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT team_id, stat_value FROM team_season_stat_extra
		WHERE competition_id = ? AND season_id = ? AND stat_key = ?`,
		competitionID, seasonID, key)
	if err != nil {
		return nil, fmt.Errorf("sqlite: team_season_stat_extra %s/%s %s: %w", competitionID, seasonID, key, err)
	}
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		var team string
		var value sql.NullFloat64
		if err := rows.Scan(&team, &value); err != nil {
			return nil, fmt.Errorf("sqlite: scan team_season_stat_extra: %w", err)
		}
		if value.Valid {
			out[team] = value.Float64
		}
	}
	return out, rows.Err()
}

func (s *Store) Standings(ctx context.Context, competitionID, seasonID string) ([]models.TeamSeasonStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT team_id, competition_id, season_id, position, matches_played, wins, draws, losses,
			goals_for, goals_against, goal_diff, points, possession_avg, fouls_committed,
			fouls_suffered, yellow_cards, red_cards, shots, shots_on_target, corners, xg, xga,
			source, updated_at, COALESCE(group_label, '')
		FROM team_season_stats
		WHERE competition_id = ? AND season_id = ?
		ORDER BY COALESCE(group_label, ''), position IS NULL, position`, competitionID, seasonID)
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
			source, updated_at, COALESCE(group_label, '')
		FROM team_season_stats
		WHERE team_id = ?
		-- Season IDs are "<competition>:<label>", so ordering on them would
		-- sort by competition first. Order by the season's own label.
		ORDER BY (SELECT se.label FROM season se WHERE se.id = team_season_stats.season_id) DESC,
			competition_id`, teamID)
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
			&st.Source, &updatedAt, &st.GroupLabel); err != nil {
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
		INSERT INTO player (id, name, name_folded, date_of_birth, nationality, position, gender)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, name_folded = excluded.name_folded,
			date_of_birth = excluded.date_of_birth, nationality = excluded.nationality,
			position = excluded.position, gender = excluded.gender`,
		p.ID, p.Name, textfold.Fold(p.Name), timePtrToNullString(p.DateOfBirth),
		p.Nationality, p.Position, p.Gender)
	if err != nil {
		return fmt.Errorf("sqlite: upsert player %s: %w", p.ID, err)
	}
	return nil
}

const playerColumns = `id, name, date_of_birth, COALESCE(nationality, ''), COALESCE(position, ''), gender`

func scanPlayer(sc interface{ Scan(...any) error }) (models.Player, error) {
	var p models.Player
	var dob sql.NullString
	if err := sc.Scan(&p.ID, &p.Name, &dob, &p.Nationality, &p.Position, &p.Gender); err != nil {
		return p, err
	}
	p.DateOfBirth = parseDatePtr(dob)
	return p, nil
}

func (s *Store) GetPlayer(ctx context.Context, id string) (models.Player, error) {
	p, err := scanPlayer(s.db.QueryRowContext(ctx,
		`SELECT `+playerColumns+` FROM player WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("sqlite: player %s: %w", id, storage.ErrNotFound)
	}
	if err != nil {
		return p, fmt.Errorf("sqlite: get player %s: %w", id, err)
	}
	return p, nil
}

func (s *Store) GetPlayers(ctx context.Context, ids []string) (map[string]models.Player, error) {
	out := make(map[string]models.Player, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	placeholders, args := inClause(ids)
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+playerColumns+` FROM player WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: get players: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan player: %w", err)
		}
		out[p.ID] = p
	}
	return out, rows.Err()
}

func (s *Store) FindPlayersByName(ctx context.Context, query string) ([]models.Player, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+playerColumns+` FROM player
		 WHERE name_folded LIKE '%' || ? || '%' ORDER BY name LIMIT 200`, textfold.Fold(query))
	if err != nil {
		return nil, fmt.Errorf("sqlite: find players by name %q: %w", query, err)
	}
	defer rows.Close()

	var out []models.Player
	for rows.Next() {
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan player: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- PlayerStatsRepo ---

func (s *Store) UpsertPlayerSeasonStats(ctx context.Context, st models.PlayerSeasonStats) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO player_season_stats (
			player_id, team_id, competition_id, season_id, appearances, minutes, goals, assists,
			xg, xa, shots, key_passes, non_penalty_goals, npxg,
			yellow_cards, red_cards, source, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(player_id, team_id, competition_id, season_id) DO UPDATE SET
			appearances = excluded.appearances, minutes = excluded.minutes, goals = excluded.goals,
			assists = excluded.assists, xg = excluded.xg, xa = excluded.xa,
			shots = COALESCE(excluded.shots, player_season_stats.shots),
			key_passes = COALESCE(excluded.key_passes, player_season_stats.key_passes),
			non_penalty_goals = COALESCE(excluded.non_penalty_goals, player_season_stats.non_penalty_goals),
			npxg = COALESCE(excluded.npxg, player_season_stats.npxg),
			yellow_cards = excluded.yellow_cards, red_cards = excluded.red_cards,
			source = excluded.source, updated_at = excluded.updated_at`,
		st.PlayerID, st.TeamID, st.CompetitionID, st.SeasonID, st.Appearances, st.Minutes,
		st.Goals, st.Assists, nullFloat(st.XG), nullFloat(st.XA),
		nullInt(st.Shots), nullInt(st.KeyPasses), nullInt(st.NonPenaltyGoals), nullFloat(st.NPxG),
		st.YellowCards, st.RedCards, st.Source, formatTime(st.UpdatedAt))
	if err != nil {
		return fmt.Errorf("sqlite: upsert player_season_stats %s/%s: %w", st.PlayerID, st.TeamID, err)
	}
	return nil
}

const playerStatsColumns = `player_id, team_id, competition_id, season_id, appearances, minutes,
	goals, assists, xg, xa, shots, key_passes, non_penalty_goals, npxg,
	yellow_cards, red_cards, source, updated_at`

func (s *Store) RosterForTeamSeason(ctx context.Context, teamID, competitionID, seasonID string) ([]models.PlayerSeasonStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+playerStatsColumns+`
		FROM player_season_stats
		WHERE team_id = ? AND competition_id = ? AND season_id = ?
		ORDER BY minutes DESC`, teamID, competitionID, seasonID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: roster %s/%s/%s: %w", teamID, competitionID, seasonID, err)
	}
	defer rows.Close()
	return scanPlayerSeasonStats(rows)
}

// PlayerHistory returns every season line for a player. Ordering by season
// then competition keeps a player who appeared in two competitions in the
// same season adjacent rather than interleaved with other seasons.
func (s *Store) PlayerHistory(ctx context.Context, playerID string) ([]models.PlayerSeasonStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+playerStatsColumns+`
		FROM player_season_stats
		WHERE player_id = ?
		ORDER BY (SELECT se.label FROM season se WHERE se.id = player_season_stats.season_id) DESC,
			competition_id`, playerID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: player history %s: %w", playerID, err)
	}
	defer rows.Close()
	return scanPlayerSeasonStats(rows)
}

func scanPlayerSeasonStats(rows *sql.Rows) ([]models.PlayerSeasonStats, error) {
	var out []models.PlayerSeasonStats
	for rows.Next() {
		var st models.PlayerSeasonStats
		var xg, xa, npxg sql.NullFloat64
		var shots, keyPasses, npg sql.NullInt64
		var updatedAt string
		if err := rows.Scan(&st.PlayerID, &st.TeamID, &st.CompetitionID, &st.SeasonID, &st.Appearances,
			&st.Minutes, &st.Goals, &st.Assists, &xg, &xa, &shots, &keyPasses, &npg, &npxg,
			&st.YellowCards, &st.RedCards, &st.Source, &updatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan player_season_stats: %w", err)
		}
		parsed, err := parseTime(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite: parse player_season_stats.updated_at %q: %w", updatedAt, err)
		}
		st.UpdatedAt = parsed
		scanNullFloat(&st.XG, xg)
		scanNullFloat(&st.XA, xa)
		scanNullFloat(&st.NPxG, npxg)
		scanNullInt(&st.Shots, shots)
		scanNullInt(&st.KeyPasses, keyPasses)
		scanNullInt(&st.NonPenaltyGoals, npg)
		out = append(out, st)
	}
	return out, rows.Err()
}
