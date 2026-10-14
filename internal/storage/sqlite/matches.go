package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

// --- MatchesRepo ---

// UpsertMatches writes every match and its stats in one transaction: a
// year of friendlies is a few hundred matches, and one commit for them all
// is the difference between instant and a visible pause.
func (s *Store) UpsertMatches(ctx context.Context, matches []models.Match) error {
	if len(matches) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: upsert matches: %w", err)
	}
	defer tx.Rollback()

	for _, m := range matches {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO match (id, competition_id, season_id, kickoff, status, round,
				home_team_id, away_team_id, neutral, home_score, away_score, home_pens, away_pens,
				source, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET competition_id = excluded.competition_id,
				season_id = excluded.season_id, kickoff = excluded.kickoff, status = excluded.status,
				round = excluded.round, home_team_id = excluded.home_team_id,
				away_team_id = excluded.away_team_id, neutral = excluded.neutral,
				home_score = excluded.home_score, away_score = excluded.away_score,
				home_pens = excluded.home_pens, away_pens = excluded.away_pens,
				source = excluded.source, updated_at = excluded.updated_at`,
			m.ID, m.CompetitionID, m.SeasonID, formatTime(m.Kickoff), m.Status, nullString(m.Round),
			m.HomeTeamID, m.AwayTeamID, m.Neutral, nullInt(m.HomeScore), nullInt(m.AwayScore),
			nullInt(m.HomePens), nullInt(m.AwayPens), m.Source, formatTime(m.UpdatedAt))
		if err != nil {
			return fmt.Errorf("sqlite: upsert match %s: %w", m.ID, err)
		}
		for _, side := range []struct {
			team  string
			stats *models.MatchTeamStats
		}{{m.HomeTeamID, m.HomeStats}, {m.AwayTeamID, m.AwayStats}} {
			if side.stats == nil {
				continue
			}
			st := side.stats
			_, err := tx.ExecContext(ctx, `
				INSERT INTO match_team_stats (match_id, team_id, possession, shots, shots_on_target, corners, fouls)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(match_id, team_id) DO UPDATE SET
					possession = COALESCE(excluded.possession, match_team_stats.possession),
					shots = COALESCE(excluded.shots, match_team_stats.shots),
					shots_on_target = COALESCE(excluded.shots_on_target, match_team_stats.shots_on_target),
					corners = COALESCE(excluded.corners, match_team_stats.corners),
					fouls = COALESCE(excluded.fouls, match_team_stats.fouls)`,
				m.ID, side.team, nullFloat(st.Possession), nullInt(st.Shots), nullInt(st.ShotsOnTarget),
				nullInt(st.Corners), nullInt(st.Fouls))
			if err != nil {
				return fmt.Errorf("sqlite: upsert match_team_stats %s/%s: %w", m.ID, side.team, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit matches: %w", err)
	}
	return nil
}

// matchSelect reads a match with both sides' stats. The stats are joined
// by side rather than fetched per match: a nation's two years is a couple
// of dozen matches, and a box score for each would otherwise be a query
// each.
const matchSelect = `
	SELECT m.id, m.competition_id, m.season_id, m.kickoff, m.status, COALESCE(m.round, ''),
		m.home_team_id, m.away_team_id, m.neutral, m.home_score, m.away_score, m.home_pens, m.away_pens,
		m.source, m.updated_at,
		hs.match_id IS NOT NULL, hs.possession, hs.shots, hs.shots_on_target, hs.corners, hs.fouls,
		aws.match_id IS NOT NULL, aws.possession, aws.shots, aws.shots_on_target, aws.corners, aws.fouls
	FROM match m
	JOIN competition c ON c.id = m.competition_id
	LEFT JOIN match_team_stats hs ON hs.match_id = m.id AND hs.team_id = m.home_team_id
	LEFT JOIN match_team_stats aws ON aws.match_id = m.id AND aws.team_id = m.away_team_id`

func scanMatches(rows *sql.Rows) ([]models.Match, error) {
	var out []models.Match
	for rows.Next() {
		var m models.Match
		var kickoff, updated string
		var hs, as, ap, hp sql.NullInt64
		var home, away statCols
		if err := rows.Scan(&m.ID, &m.CompetitionID, &m.SeasonID, &kickoff, &m.Status, &m.Round,
			&m.HomeTeamID, &m.AwayTeamID, &m.Neutral, &hs, &as, &hp, &ap, &m.Source, &updated,
			&home.present, &home.poss, &home.shots, &home.sot, &home.corners, &home.fouls,
			&away.present, &away.poss, &away.shots, &away.sot, &away.corners, &away.fouls); err != nil {
			return nil, fmt.Errorf("sqlite: scan match: %w", err)
		}
		var err error
		if m.Kickoff, err = parseTime(kickoff); err != nil {
			return nil, fmt.Errorf("sqlite: match %s kickoff %q: %w", m.ID, kickoff, err)
		}
		if m.UpdatedAt, err = parseTime(updated); err != nil {
			return nil, fmt.Errorf("sqlite: match %s updated_at %q: %w", m.ID, updated, err)
		}
		scanNullInt(&m.HomeScore, hs)
		scanNullInt(&m.AwayScore, as)
		scanNullInt(&m.HomePens, hp)
		scanNullInt(&m.AwayPens, ap)
		m.HomeStats, m.AwayStats = home.stats(), away.stats()
		out = append(out, m)
	}
	return out, rows.Err()
}

type statCols struct {
	present                    bool
	poss                       sql.NullFloat64
	shots, sot, corners, fouls sql.NullInt64
}

func (c statCols) stats() *models.MatchTeamStats {
	if !c.present {
		return nil
	}
	st := &models.MatchTeamStats{}
	scanNullFloat(&st.Possession, c.poss)
	scanNullInt(&st.Shots, c.shots)
	scanNullInt(&st.ShotsOnTarget, c.sot)
	scanNullInt(&st.Corners, c.corners)
	scanNullInt(&st.Fouls, c.fouls)
	return st
}

// filterClause turns a match filter and a set of calendar years into SQL
// conditions on the aliases in matchSelect.
func filterClause(filter models.MatchFilter, years []int) (string, []any) {
	var conds []string
	var args []any
	switch filter.Preset {
	case models.PresetCompetitive:
		conds = append(conds, `c.category IN (?, ?, ?)`)
		args = append(args, models.CategoryTournament, models.CategoryQualifier, models.CategoryNationsLeague)
	case models.PresetTournaments:
		conds = append(conds, `c.category = ?`)
		args = append(args, models.CategoryTournament)
	case models.PresetAll:
	default:
		if filter.Competitions != nil {
			if len(filter.Competitions) == 0 {
				conds = append(conds, `0`)
			} else {
				ph, a := inClause(filter.Competitions)
				conds = append(conds, `m.competition_id IN (`+ph+`)`)
				args = append(args, a...)
			}
		}
	}
	if len(years) > 0 {
		ys := make([]string, len(years))
		for i, y := range years {
			ys[i] = strconv.Itoa(y)
		}
		ph, a := inClause(ys)
		conds = append(conds, `substr(m.kickoff, 1, 4) IN (`+ph+`)`)
		args = append(args, a...)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(conds, " AND "), args
}

func (s *Store) MatchesForTeam(ctx context.Context, teamID string, filter models.MatchFilter, years []int) ([]models.Match, error) {
	where, args := filterClause(filter, years)
	rows, err := s.db.QueryContext(ctx, matchSelect+`
		WHERE (m.home_team_id = ? OR m.away_team_id = ?)`+where+`
		ORDER BY m.kickoff DESC, m.id DESC`, append([]any{teamID, teamID}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: matches for team %s: %w", teamID, err)
	}
	defer rows.Close()
	return scanMatches(rows)
}

func (s *Store) MatchesForCompetitionSeason(ctx context.Context, competitionID, seasonID string) ([]models.Match, error) {
	rows, err := s.db.QueryContext(ctx, matchSelect+`
		WHERE m.competition_id = ? AND m.season_id = ?
		ORDER BY m.kickoff, m.id`, competitionID, seasonID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: matches for %s/%s: %w", competitionID, seasonID, err)
	}
	defer rows.Close()
	return scanMatches(rows)
}

// TeamMatchSummary is computed from the filtered matches on read, so any
// filter gives a correct record without anything precomputed to go stale.
func (s *Store) TeamMatchSummary(ctx context.Context, teamID string, filter models.MatchFilter, years []int) (models.MatchSummary, error) {
	matches, err := s.MatchesForTeam(ctx, teamID, filter, years)
	if err != nil {
		return models.MatchSummary{}, err
	}
	return models.SummarizeMatches(teamID, matches), nil
}

func (s *Store) TeamMatchYears(ctx context.Context, teamID string) ([]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT CAST(substr(kickoff, 1, 4) AS INTEGER) AS y FROM match
		WHERE home_team_id = ? OR away_team_id = ?
		ORDER BY y DESC`, teamID, teamID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: match years %s: %w", teamID, err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var y int
		if err := rows.Scan(&y); err != nil {
			return nil, fmt.Errorf("sqlite: scan match year: %w", err)
		}
		out = append(out, y)
	}
	return out, rows.Err()
}

// MatchesKickedOff compares kickoffs as text: both sides are RFC3339 in
// UTC, which sorts chronologically.
func (s *Store) MatchesKickedOff(ctx context.Context, competitionID string, year int, since, until time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM match
		WHERE competition_id = ? AND substr(kickoff, 1, 4) = ?
			AND status IN (?, ?) AND kickoff > ? AND kickoff <= ?`,
		competitionID, strconv.Itoa(year), models.MatchScheduled, models.MatchInPlay,
		formatTime(since), formatTime(until)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("sqlite: matches kicked off %s/%d: %w", competitionID, year, err)
	}
	return n, nil
}

func (s *Store) GetTeamMatchFilter(ctx context.Context, teamID string) (models.MatchFilter, error) {
	var preset, comps sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT preset, competitions FROM team_match_filter WHERE team_id = ?`, teamID).Scan(&preset, &comps)
	if errors.Is(err, sql.ErrNoRows) {
		return models.MatchFilter{}, nil
	}
	if err != nil {
		return models.MatchFilter{}, fmt.Errorf("sqlite: team_match_filter %s: %w", teamID, err)
	}
	f := models.MatchFilter{Preset: preset.String}
	if comps.Valid {
		if err := json.Unmarshal([]byte(comps.String), &f.Competitions); err != nil {
			// A filter is a convenience: an unreadable one is the default,
			// not a screen that won't open.
			return models.MatchFilter{}, nil
		}
		if f.Competitions == nil {
			f.Competitions = []string{}
		}
	}
	return f, nil
}

func (s *Store) SetTeamMatchFilter(ctx context.Context, teamID string, f models.MatchFilter) error {
	var comps sql.NullString
	if f.Preset == "" && f.Competitions != nil {
		b, err := json.Marshal(f.Competitions)
		if err != nil {
			return fmt.Errorf("sqlite: encode filter %s: %w", teamID, err)
		}
		comps = sql.NullString{String: string(b), Valid: true}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO team_match_filter (team_id, preset, competitions, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(team_id) DO UPDATE SET preset = excluded.preset,
			competitions = excluded.competitions, updated_at = excluded.updated_at`,
		teamID, nullString(f.Preset), comps, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("sqlite: set team_match_filter %s: %w", teamID, err)
	}
	return nil
}

// --- national teams ---

func (s *Store) NationalTeams(ctx context.Context, gender string) ([]models.Team, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+teamColumns+` FROM team
		WHERE national = 1 AND (? = '' OR gender = ?) ORDER BY name, id`, gender, gender)
	if err != nil {
		return nil, fmt.Errorf("sqlite: national teams: %w", err)
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

// --- year runs ---

const syncRunColumns = `id, provider, competition_id, COALESCE(season_id, ''), COALESCE(year, 0),
	started_at, finished_at, status, rows_upserted, error`

func scanSyncRun(sc interface{ Scan(...any) error }) (models.SyncRun, error) {
	var r models.SyncRun
	var startedAt, finishedAt string
	var errText sql.NullString
	if err := sc.Scan(&r.ID, &r.Provider, &r.CompetitionID, &r.SeasonID, &r.Year,
		&startedAt, &finishedAt, &r.Status, &r.RowsUpserted, &errText); err != nil {
		return r, err
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

func (s *Store) LatestYearRun(ctx context.Context, competitionID string, year int) (models.SyncRun, error) {
	r, err := scanSyncRun(s.db.QueryRowContext(ctx, `
		SELECT `+syncRunColumns+` FROM sync_log
		WHERE competition_id = ? AND year = ?
		ORDER BY finished_at DESC, id DESC LIMIT 1`, competitionID, year))
	if errors.Is(err, sql.ErrNoRows) {
		return r, fmt.Errorf("sqlite: sync_log %s/%d: %w", competitionID, year, storage.ErrNotFound)
	}
	if err != nil {
		return r, fmt.Errorf("sqlite: latest year run %s/%d: %w", competitionID, year, err)
	}
	return r, nil
}

func (s *Store) LatestYearRuns(ctx context.Context) ([]models.SyncRun, error) {
	// The newest row per (competition, year): no row of the same pair is
	// newer.
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+syncRunColumns+` FROM sync_log l
		WHERE year IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM sync_log n WHERE n.competition_id = l.competition_id AND n.year = l.year
				AND (n.finished_at > l.finished_at OR (n.finished_at = l.finished_at AND n.id > l.id)))
		ORDER BY competition_id, year`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: latest year runs: %w", err)
	}
	defer rows.Close()
	var out []models.SyncRun
	for rows.Next() {
		r, err := scanSyncRun(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan sync run: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) LastRateLimited(ctx context.Context) (time.Time, error) {
	var at sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT MAX(finished_at) FROM sync_log WHERE status = ?`, models.SyncStatusRateLimited).Scan(&at); err != nil {
		return time.Time{}, fmt.Errorf("sqlite: last rate limited: %w", err)
	}
	if !at.Valid {
		return time.Time{}, fmt.Errorf("sqlite: never rate limited: %w", storage.ErrNotFound)
	}
	return parseTime(at.String)
}
