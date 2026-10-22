package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

func (a *App) favoriteTarget() (core.FavoriteTarget, bool) {
	if f, ok := a.current().(core.Favoritable); ok {
		return f.FavoriteTarget()
	}
	return core.FavoriteTarget{}, false
}

func (a *App) favoriteBinding() (key.Binding, bool) {
	if _, ok := a.favoriteTarget(); !ok {
		return key.Binding{}, false
	}
	if a.capturingText() {
		return keys.FavoriteFromTextEntry, true
	}
	return keys.Favorite, true
}

func (a *App) toggleFavorite() tea.Cmd {
	target, ok := a.favoriteTarget()
	if !ok {
		a.status = "nothing to favourite here — open a league, club, nation or player"
		return nil
	}
	if a.deps.Favorites == nil {
		a.deps.Favorites = core.NewFavorites(nil)
	}
	return core.ToggleFavorite(a.deps, target, time.Now)
}

// favoriteToggled changes the shared set only once the database has.
func (a *App) favoriteToggled(msg core.FavoriteToggledMsg) tea.Cmd {
	if msg.Err != nil {
		a.status = "couldn't save the favourite: " + msg.Err.Error()
		return nil
	}
	a.deps.Favorites.Set(msg.Target.Kind, msg.Target.ID, msg.On)
	if msg.On {
		a.status = "★ " + msg.Target.Name + " added to favourites — footui opens on them next time"
	} else {
		a.status = msg.Target.Name + " removed from favourites"
	}
	changed := a.broadcast(core.FavoritesChangedMsg{})
	if msg.On && msg.Target.Kind == models.FavoriteTeam && a.deps.Refresh != nil {
		// Starring a nation fetches its matches, which nothing else asks for.
		if t, err := a.deps.Repos.GetTeam(a.deps.Ctx, msg.Target.ID); err == nil && t.National {
			status := a.status
			cmd := a.enqueue(core.RefreshRequest{TeamID: t.ID, Name: t.Label()})
			a.status = status
			return tea.Batch(changed, cmd)
		}
	}
	return changed
}
