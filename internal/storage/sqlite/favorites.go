package sqlite

import (
	"context"
	"fmt"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// --- FavoritesRepo ---

// AddFavorite keeps an existing favourite's AddedAt, so re-adding never
// reorders the dashboard.
func (s *Store) AddFavorite(ctx context.Context, f models.Favorite) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO favorite (entity_type, entity_id, added_at) VALUES (?, ?, ?)
		ON CONFLICT(entity_type, entity_id) DO NOTHING`,
		string(f.Kind), f.ID, formatTime(f.AddedAt))
	if err != nil {
		return fmt.Errorf("sqlite: add favorite %s/%s: %w", f.Kind, f.ID, err)
	}
	return nil
}

// RemoveFavorite is not an error for a non-favourite.
func (s *Store) RemoveFavorite(ctx context.Context, kind models.FavoriteKind, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM favorite WHERE entity_type = ? AND entity_id = ?`, string(kind), id)
	if err != nil {
		return fmt.Errorf("sqlite: remove favorite %s/%s: %w", kind, id, err)
	}
	return nil
}

// ListFavorites returns every favourite, oldest first.
func (s *Store) ListFavorites(ctx context.Context) ([]models.Favorite, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT entity_type, entity_id, added_at FROM favorite
		ORDER BY added_at, entity_type, entity_id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list favorites: %w", err)
	}
	defer rows.Close()

	var out []models.Favorite
	for rows.Next() {
		var f models.Favorite
		var kind, added string
		if err := rows.Scan(&kind, &f.ID, &added); err != nil {
			return nil, fmt.Errorf("sqlite: scan favorite: %w", err)
		}
		f.Kind = models.FavoriteKind(kind)
		if f.AddedAt, err = parseTime(added); err != nil {
			return nil, fmt.Errorf("sqlite: favorite %s/%s added_at %q: %w", kind, f.ID, added, err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
