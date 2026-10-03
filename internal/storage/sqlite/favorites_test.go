package sqlite

import (
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

func TestFavoritesAddListRemove(t *testing.T) {
	s := newTestStore(t)
	t0 := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)

	if favs, err := s.ListFavorites(ctx); err != nil || len(favs) != 0 {
		t.Fatalf("empty store: %v, %v", favs, err)
	}
	mustNoErr(t, s.AddFavorite(ctx, models.Favorite{Kind: models.FavoritePlayer, ID: "p-saka", AddedAt: t0.Add(2 * time.Minute)}))
	mustNoErr(t, s.AddFavorite(ctx, models.Favorite{Kind: models.FavoriteCompetition, ID: "eng-premier-league", AddedAt: t0}))
	mustNoErr(t, s.AddFavorite(ctx, models.Favorite{Kind: models.FavoriteTeam, ID: "eng-arsenal", AddedAt: t0.Add(time.Minute)}))
	// The same ID under another kind is a separate favourite.
	mustNoErr(t, s.AddFavorite(ctx, models.Favorite{Kind: models.FavoriteTeam, ID: "p-saka", AddedAt: t0.Add(3 * time.Minute)}))

	favs, err := s.ListFavorites(ctx)
	mustNoErr(t, err)
	want := []models.Favorite{
		{Kind: models.FavoriteCompetition, ID: "eng-premier-league", AddedAt: t0},
		{Kind: models.FavoriteTeam, ID: "eng-arsenal", AddedAt: t0.Add(time.Minute)},
		{Kind: models.FavoritePlayer, ID: "p-saka", AddedAt: t0.Add(2 * time.Minute)},
		{Kind: models.FavoriteTeam, ID: "p-saka", AddedAt: t0.Add(3 * time.Minute)},
	}
	if len(favs) != len(want) {
		t.Fatalf("got %+v", favs)
	}
	for i := range want {
		if favs[i].Kind != want[i].Kind || favs[i].ID != want[i].ID || !favs[i].AddedAt.Equal(want[i].AddedAt) {
			t.Errorf("favs[%d] = %+v, want %+v", i, favs[i], want[i])
		}
	}

	mustNoErr(t, s.RemoveFavorite(ctx, models.FavoriteTeam, "p-saka"))
	mustNoErr(t, s.RemoveFavorite(ctx, models.FavoriteTeam, "never-starred"))
	favs, _ = s.ListFavorites(ctx)
	if len(favs) != 3 || favs[2].Kind != models.FavoritePlayer {
		t.Errorf("after removal: %+v", favs)
	}
}

// Starring something twice keeps its place on the dashboard.
func TestAddFavoriteTwiceKeepsTheOriginalTime(t *testing.T) {
	s := newTestStore(t)
	t0 := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	mustNoErr(t, s.AddFavorite(ctx, models.Favorite{Kind: models.FavoriteTeam, ID: "eng-arsenal", AddedAt: t0}))
	mustNoErr(t, s.AddFavorite(ctx, models.Favorite{Kind: models.FavoriteTeam, ID: "eng-arsenal", AddedAt: t0.Add(time.Hour)}))
	favs, _ := s.ListFavorites(ctx)
	if len(favs) != 1 || !favs[0].AddedAt.Equal(t0) {
		t.Errorf("got %+v", favs)
	}
}

func TestFavoriteKindIsChecked(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddFavorite(ctx, models.Favorite{Kind: "venue", ID: "x", AddedAt: time.Now()}); err == nil {
		t.Error("an unknown kind should be rejected by the schema")
	}
}
