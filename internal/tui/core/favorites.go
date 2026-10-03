package core

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// Favorites is the set of starred entities, shared by pointer through Deps
// like Appearance, so a star toggled on one screen shows on every other.
// The root app is the only writer: it persists the change, updates the
// set, then broadcasts FavoritesChangedMsg. A nil *Favorites is an empty
// set.
type Favorites struct {
	set map[favoriteKey]bool
}

type favoriteKey struct {
	kind models.FavoriteKind
	id   string
}

// NewFavorites returns a set holding favs.
func NewFavorites(favs []models.Favorite) *Favorites {
	f := &Favorites{set: map[favoriteKey]bool{}}
	for _, fav := range favs {
		f.set[favoriteKey{fav.Kind, fav.ID}] = true
	}
	return f
}

// Has reports whether the entity is a favourite.
func (f *Favorites) Has(kind models.FavoriteKind, id string) bool {
	return f != nil && f.set[favoriteKey{kind, id}]
}

// Len is how many favourites there are.
func (f *Favorites) Len() int {
	if f == nil {
		return 0
	}
	return len(f.set)
}

// Set records whether the entity is a favourite.
func (f *Favorites) Set(kind models.FavoriteKind, id string, on bool) {
	if f.set == nil {
		f.set = map[favoriteKey]bool{}
	}
	if on {
		f.set[favoriteKey{kind, id}] = true
	} else {
		delete(f.set, favoriteKey{kind, id})
	}
}

// Star prefixes title with ★ when the entity is a favourite, so every
// screen marks its own favourite state the same way.
func (d Deps) Star(kind models.FavoriteKind, id, title string) string {
	if d.Favorites.Has(kind, id) {
		return "★ " + title
	}
	return title
}

// FavoriteTarget is what the favourite key would star or un-star from the
// current screen.
type FavoriteTarget struct {
	Kind models.FavoriteKind
	ID   string
	// Name is how the status line refers to it.
	Name string
}

// Favoritable is implemented by screens with something to favourite: the
// league, club or player they show, or the highlighted row or card.
type Favoritable interface {
	FavoriteTarget() (FavoriteTarget, bool)
}

// FavoritesChangedMsg is broadcast to every screen after a favourite is
// added or removed, so stars and the dashboard redraw.
type FavoritesChangedMsg struct{}

// FavoriteToggledMsg reports the outcome of persisting a favourite toggle
// to the root app. On is the new state; Err is set when the write failed,
// in which case nothing changed.
type FavoriteToggledMsg struct {
	Target FavoriteTarget
	On     bool
	Err    error
}

// ToggleFavorite returns a command that persists the opposite of the
// target's current state.
func ToggleFavorite(deps Deps, target FavoriteTarget, now func() time.Time) tea.Cmd {
	on := !deps.Favorites.Has(target.Kind, target.ID)
	return func() tea.Msg {
		var err error
		if on {
			err = deps.Repos.AddFavorite(deps.Ctx, models.Favorite{Kind: target.Kind, ID: target.ID, AddedAt: now()})
		} else {
			err = deps.Repos.RemoveFavorite(deps.Ctx, target.Kind, target.ID)
		}
		return FavoriteToggledMsg{Target: target, On: on, Err: err}
	}
}
