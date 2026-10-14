package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// favoriteTarget reports what the favourite key would star from the
// current screen, if anything.
func (a *App) favoriteTarget() (core.FavoriteTarget, bool) {
	if f, ok := a.current().(core.Favoritable); ok {
		return f.FavoriteTarget()
	}
	return core.FavoriteTarget{}, false
}

// favoriteBinding is the favourite key the footer should advertise, or
// false when there's nothing to favourite here.
func (a *App) favoriteBinding() (key.Binding, bool) {
	if _, ok := a.favoriteTarget(); !ok {
		return key.Binding{}, false
	}
	if a.capturingText() {
		return keys.FavoriteFromTextEntry, true
	}
	return keys.Favorite, true
}

// toggleFavorite handles the favourite key.
func (a *App) toggleFavorite() tea.Cmd {
	target, ok := a.favoriteTarget()
	if !ok {
		a.status = "nothing to favourite here — open a league, club or player"
		return nil
	}
	if a.deps.Favorites == nil {
		a.deps.Favorites = core.NewFavorites(nil)
	}
	return core.ToggleFavorite(a.deps, target, time.Now)
}

// favoriteToggled applies a persisted toggle: the shared set changes only
// once the database has, so a failed write never shows a star that won't
// be there next launch.
func (a *App) favoriteToggled(msg core.FavoriteToggledMsg) tea.Cmd {
	if msg.Err != nil {
		a.status = "couldn't save the favourite: " + msg.Err.Error()
		return nil
	}
	a.deps.Favorites.Set(msg.Target.Kind, msg.Target.ID, msg.On)
	if msg.On {
		a.status = "★ " + msg.Target.Name + " added to favourites — footy opens on them next time"
	} else {
		a.status = msg.Target.Name + " removed from favourites"
	}
	changed := a.broadcast(core.FavoritesChangedMsg{})
	if msg.On && msg.Target.Kind == models.FavoriteTeam && a.deps.Refresh != nil {
		// A nation is followed through its matches, which aren't fetched
		// until something asks: starring it asks. It costs nothing when
		// another nation of its gender already brought them in.
		if t, err := a.deps.Repos.GetTeam(a.deps.Ctx, msg.Target.ID); err == nil && t.National {
			status := a.status
			cmd := a.enqueue(core.RefreshRequest{TeamID: t.ID, Name: t.Name})
			a.status = status
			return tea.Batch(changed, cmd)
		}
	}
	return changed
}
