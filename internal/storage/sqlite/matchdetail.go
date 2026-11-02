package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

// detailStatColumns pairs match_detail_stats's integer columns with their
// fields, so the write and the read list them once.
var detailStatColumns = []struct {
	name  string
	field func(*models.MatchDetailStats) **int
}{
	{"shots", func(s *models.MatchDetailStats) **int { return &s.Shots }},
	{"shots_on_target", func(s *models.MatchDetailStats) **int { return &s.ShotsOnTarget }},
	{"blocked_shots", func(s *models.MatchDetailStats) **int { return &s.BlockedShots }},
	{"passes", func(s *models.MatchDetailStats) **int { return &s.Passes }},
	{"accurate_passes", func(s *models.MatchDetailStats) **int { return &s.AccuratePasses }},
	{"crosses", func(s *models.MatchDetailStats) **int { return &s.Crosses }},
	{"accurate_crosses", func(s *models.MatchDetailStats) **int { return &s.AccurateCrosses }},
	{"long_balls", func(s *models.MatchDetailStats) **int { return &s.LongBalls }},
	{"accurate_long_balls", func(s *models.MatchDetailStats) **int { return &s.AccurateLongBalls }},
	{"tackles", func(s *models.MatchDetailStats) **int { return &s.Tackles }},
	{"tackles_won", func(s *models.MatchDetailStats) **int { return &s.TacklesWon }},
	{"interceptions", func(s *models.MatchDetailStats) **int { return &s.Interceptions }},
	{"clearances", func(s *models.MatchDetailStats) **int { return &s.Clearances }},
	{"saves", func(s *models.MatchDetailStats) **int { return &s.Saves }},
	{"offsides", func(s *models.MatchDetailStats) **int { return &s.Offsides }},
	{"corners", func(s *models.MatchDetailStats) **int { return &s.Corners }},
	{"fouls", func(s *models.MatchDetailStats) **int { return &s.Fouls }},
	{"yellow_cards", func(s *models.MatchDetailStats) **int { return &s.YellowCards }},
	{"red_cards", func(s *models.MatchDetailStats) **int { return &s.RedCards }},
}

func detailStatNames() string {
	names := make([]string, len(detailStatColumns))
	for i, c := range detailStatColumns {
		names[i] = c.name
	}
	return strings.Join(names, ", ")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// PutMatchDetail replaces everything stored for d.MatchID in one
// transaction.
func (s *Store) PutMatchDetail(ctx context.Context, d models.MatchDetail) error {
	if d.MatchID == "" {
		return errors.New("sqlite: match detail without a match ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin match detail %s: %w", d.MatchID, err)
	}
	defer tx.Rollback()
	fail := func(what string, err error) error {
		return fmt.Errorf("sqlite: match detail %s %s: %w", d.MatchID, what, err)
	}

	for _, table := range []string{"match_commentary", "match_lineup", "match_event", "match_detail_stats", "match_detail"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE match_id = ?", d.MatchID); err != nil {
			return fail("clear "+table, err)
		}
	}
	var attendance sql.NullInt64
	if d.Attendance > 0 {
		attendance = sql.NullInt64{Int64: int64(d.Attendance), Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO match_detail (match_id, final, venue, attendance, home_formation, away_formation, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.MatchID, boolInt(d.Final), nullString(d.Venue), attendance,
		nullString(d.Home.Formation), nullString(d.Away.Formation), formatTime(d.FetchedAt)); err != nil {
		return fail("insert", err)
	}

	statsSQL := "INSERT INTO match_detail_stats (match_id, side, possession, " + detailStatNames() +
		") VALUES (?, ?, ?" + strings.Repeat(", ?", len(detailStatColumns)) + ")"
	for _, side := range []string{models.SideHome, models.SideAway} {
		st := d.Side(side == models.SideHome).Stats
		args := []any{d.MatchID, side, nullFloat(st.Possession)}
		for _, c := range detailStatColumns {
			args = append(args, nullInt(*c.field(&st)))
		}
		if _, err := tx.ExecContext(ctx, statsSQL, args...); err != nil {
			return fail("stats", err)
		}

		for i, p := range d.Side(side == models.SideHome).Lineup {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO match_lineup (match_id, side, seq, name, player_id, shirt, position, place, starter,
					on_min, off_min, goals, assists, own_goals, yellow, red, saves)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				d.MatchID, side, i, p.Name, nullString(p.PlayerID), nullString(p.Shirt), nullString(p.Position),
				p.Place, boolInt(p.Starter), p.On, p.Off, p.Goals, p.Assists, p.OwnGoals,
				boolInt(p.Yellow), boolInt(p.Red), p.Saves); err != nil {
				return fail("lineup", err)
			}
		}
	}

	for i, e := range d.Events {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO match_event (match_id, seq, kind, side, period, minute, added,
				player, player_id, other, other_id, text)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.MatchID, i, e.Kind, e.Side, e.Period, e.Minute, e.Added,
			nullString(e.Player), nullString(e.PlayerID), nullString(e.Other), nullString(e.OtherID),
			nullString(e.Text)); err != nil {
			return fail("event", err)
		}
	}
	for i, c := range d.Commentary {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO match_commentary (match_id, seq, clock, kind, text) VALUES (?, ?, ?, ?, ?)`,
			d.MatchID, i, c.Clock, c.Kind, c.Text); err != nil {
			return fail("commentary", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fail("commit", err)
	}
	return nil
}

// MatchDetail returns storage.ErrNotFound when none was fetched.
func (s *Store) MatchDetail(ctx context.Context, matchID string) (models.MatchDetail, error) {
	d := models.MatchDetail{MatchID: matchID}
	fail := func(what string, err error) (models.MatchDetail, error) {
		return models.MatchDetail{}, fmt.Errorf("sqlite: match detail %s %s: %w", matchID, what, err)
	}

	var final int
	var venue, homeForm, awayForm sql.NullString
	var attendance sql.NullInt64
	var fetched string
	err := s.db.QueryRowContext(ctx, `
		SELECT final, venue, attendance, home_formation, away_formation, fetched_at
		FROM match_detail WHERE match_id = ?`, matchID).
		Scan(&final, &venue, &attendance, &homeForm, &awayForm, &fetched)
	if errors.Is(err, sql.ErrNoRows) {
		return fail("", storage.ErrNotFound)
	}
	if err != nil {
		return fail("", err)
	}
	d.Final, d.Venue, d.Attendance = final == 1, venue.String, int(attendance.Int64)
	d.Home.Formation, d.Away.Formation = homeForm.String, awayForm.String
	if d.FetchedAt, err = parseTime(fetched); err != nil {
		return fail("fetched_at", err)
	}

	if err := s.scanDetailStats(ctx, &d); err != nil {
		return fail("stats", err)
	}
	if err := s.scanLineups(ctx, &d); err != nil {
		return fail("lineup", err)
	}
	if err := s.scanEvents(ctx, &d); err != nil {
		return fail("events", err)
	}
	if err := s.scanCommentary(ctx, &d); err != nil {
		return fail("commentary", err)
	}
	return d, nil
}

func sideOf(d *models.MatchDetail, side string) *models.MatchSide {
	if side == models.SideHome {
		return &d.Home
	}
	return &d.Away
}

func (s *Store) scanDetailStats(ctx context.Context, d *models.MatchDetail) error {
	rows, err := s.db.QueryContext(ctx, "SELECT side, possession, "+detailStatNames()+
		" FROM match_detail_stats WHERE match_id = ?", d.MatchID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var side string
		var poss sql.NullFloat64
		ints := make([]sql.NullInt64, len(detailStatColumns))
		dest := []any{&side, &poss}
		for i := range ints {
			dest = append(dest, &ints[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		st := &sideOf(d, side).Stats
		scanNullFloat(&st.Possession, poss)
		for i, c := range detailStatColumns {
			scanNullInt(c.field(st), ints[i])
		}
	}
	return rows.Err()
}

func (s *Store) scanLineups(ctx context.Context, d *models.MatchDetail) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT side, name, player_id, shirt, position, place, starter, on_min, off_min,
			goals, assists, own_goals, yellow, red, saves
		FROM match_lineup WHERE match_id = ? ORDER BY side, seq`, d.MatchID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var side string
		var p models.LineupPlayer
		var id, shirt, pos sql.NullString
		var starter, yellow, red int
		if err := rows.Scan(&side, &p.Name, &id, &shirt, &pos, &p.Place, &starter, &p.On, &p.Off,
			&p.Goals, &p.Assists, &p.OwnGoals, &yellow, &red, &p.Saves); err != nil {
			return err
		}
		p.PlayerID, p.Shirt, p.Position = id.String, shirt.String, pos.String
		p.Starter, p.Yellow, p.Red = starter == 1, yellow == 1, red == 1
		sd := sideOf(d, side)
		sd.Lineup = append(sd.Lineup, p)
	}
	return rows.Err()
}

func (s *Store) scanEvents(ctx context.Context, d *models.MatchDetail) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT kind, side, period, minute, added, player, player_id, other, other_id, text
		FROM match_event WHERE match_id = ? ORDER BY seq`, d.MatchID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e models.MatchEvent
		var player, playerID, other, otherID, text sql.NullString
		if err := rows.Scan(&e.Kind, &e.Side, &e.Period, &e.Minute, &e.Added,
			&player, &playerID, &other, &otherID, &text); err != nil {
			return err
		}
		e.Player, e.PlayerID, e.Other, e.OtherID, e.Text = player.String, playerID.String, other.String, otherID.String, text.String
		d.Events = append(d.Events, e)
	}
	return rows.Err()
}

func (s *Store) scanCommentary(ctx context.Context, d *models.MatchDetail) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT clock, kind, text FROM match_commentary WHERE match_id = ? ORDER BY seq`, d.MatchID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c models.CommentaryLine
		if err := rows.Scan(&c.Clock, &c.Kind, &c.Text); err != nil {
			return err
		}
		d.Commentary = append(d.Commentary, c)
	}
	return rows.Err()
}
