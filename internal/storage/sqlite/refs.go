package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

// --- ExternalRefRepo ---

func (s *Store) ExternalRef(ctx context.Context, entityType, provider, providerID string) (string, error) {
	var canonical string
	err := s.db.QueryRowContext(ctx, `
		SELECT canonical_id FROM external_ref
		WHERE entity_type = ? AND provider = ? AND provider_id = ?`,
		entityType, provider, providerID).Scan(&canonical)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("sqlite: external_ref %s/%s/%s: %w", entityType, provider, providerID, storage.ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("sqlite: external_ref %s/%s/%s: %w", entityType, provider, providerID, err)
	}
	return canonical, nil
}

func (s *Store) ProviderID(ctx context.Context, entityType, canonicalID, provider string) (string, error) {
	var id string
	// Two provider IDs can resolve to one entity (a provider that
	// re-numbered a player); either reaches the same stats, so the
	// choice only has to be stable.
	err := s.db.QueryRowContext(ctx, `
		SELECT provider_id FROM external_ref
		WHERE entity_type = ? AND canonical_id = ? AND provider = ?
		ORDER BY provider_id LIMIT 1`,
		entityType, canonicalID, provider).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("sqlite: provider id %s/%s/%s: %w", entityType, canonicalID, provider, storage.ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("sqlite: provider id %s/%s/%s: %w", entityType, canonicalID, provider, err)
	}
	return id, nil
}

// PutExternalRef records (or corrects) where a provider's ID resolves.
func (s *Store) PutExternalRef(ctx context.Context, entityType, canonicalID, provider, providerID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO external_ref (entity_type, canonical_id, provider, provider_id)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(entity_type, provider, provider_id) DO UPDATE SET canonical_id = excluded.canonical_id`,
		entityType, canonicalID, provider, providerID)
	if err != nil {
		return fmt.Errorf("sqlite: put external_ref %s/%s/%s: %w", entityType, provider, providerID, err)
	}
	return nil
}

func (s *Store) TeamPlayers(ctx context.Context, teamID string) ([]models.Player, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+playerColumns+` FROM player
		WHERE id IN (SELECT DISTINCT player_id FROM player_season_stats WHERE team_id = ?)
		ORDER BY name`, teamID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: team players %s: %w", teamID, err)
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
