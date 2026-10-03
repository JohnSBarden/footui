package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/browse"
	"github.com/JohnSBarden/footui/internal/tui/screens/dashboard"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// favScreen is a screen with something to favourite, recording the
// FavoritesChangedMsgs it is handed.
type favScreen struct {
	fakeScreen
	target  core.FavoriteTarget
	changed *int
}

func (f favScreen) FavoriteTarget() (core.FavoriteTarget, bool) { return f.target, f.target.ID != "" }

func (f favScreen) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	if _, ok := msg.(core.FavoritesChangedMsg); ok {
		*f.changed++
	}
	f.fakeScreen.Update(msg)
	return f, nil
}

func favApp(t *testing.T, capturing bool) (*App, storage.Repositories, favScreen) {
	t.Helper()
	repos := storagetest.New(t)
	screen := favScreen{
		fakeScreen: newFake("Team"),
		target:     core.FavoriteTarget{Kind: models.FavoriteTeam, ID: "eng-arsenal", Name: "Arsenal"},
		changed:    new(int),
	}
	screen.capturing = capturing
	deps := NewDeps(context.Background(), repos, styles.Plain())
	return NewApp(deps, screen), repos, screen
}

// pressAndSettle sends a key and feeds back the message its command produces, as
// the runtime would.
func pressAndSettle(app *App, k string) {
	_, cmd := app.Update(keyPress(k))
	if cmd == nil {
		return
	}
	if msg := cmd(); msg != nil {
		_, next := app.Update(msg)
		if next != nil {
			if batch, ok := next().(tea.BatchMsg); ok {
				for _, c := range batch {
					if c != nil {
						c()
					}
				}
			}
		}
	}
}

func TestFavouriteKeyStarsAndUnstarsAndPersists(t *testing.T) {
	app, repos, screen := favApp(t, false)

	pressAndSettle(app, "f")
	favs, _ := repos.ListFavorites(context.Background())
	if len(favs) != 1 || favs[0].ID != "eng-arsenal" || favs[0].Kind != models.FavoriteTeam {
		t.Fatalf("after f: stored %+v", favs)
	}
	if !app.deps.Favorites.Has(models.FavoriteTeam, "eng-arsenal") {
		t.Error("the shared set should know about the new favourite")
	}
	if !strings.Contains(app.status, "★ Arsenal added to favourites") {
		t.Errorf("status %q", app.status)
	}
	if *screen.changed != 1 {
		t.Errorf("screens were told %d times, want once", *screen.changed)
	}

	pressAndSettle(app, "f")
	favs, _ = repos.ListFavorites(context.Background())
	if len(favs) != 0 || app.deps.Favorites.Has(models.FavoriteTeam, "eng-arsenal") {
		t.Errorf("after a second f: stored %+v", favs)
	}
	if !strings.Contains(app.status, "Arsenal removed from favourites") {
		t.Errorf("status %q", app.status)
	}
}

// In a text box f is a letter; ctrl+f still stars.
func TestFavouriteKeyFromATextBox(t *testing.T) {
	app, repos, screen := favApp(t, true)

	pressAndSettle(app, "f")
	if favs, _ := repos.ListFavorites(context.Background()); len(favs) != 0 {
		t.Errorf("f in a text box starred %+v", favs)
	}
	if got := *screen.keys; len(got) != 1 || got[0] != "f" {
		t.Errorf("the text box got %v, want the letter f", got)
	}

	pressAndSettle(app, "ctrl+f")
	if favs, _ := repos.ListFavorites(context.Background()); len(favs) != 1 {
		t.Errorf("ctrl+f should star from a text box, stored %+v", favs)
	}
}

func TestFooterOffersFavouriteOnlyWithATarget(t *testing.T) {
	app, _, _ := favApp(t, false)
	if !strings.Contains(app.footer(), "★ favourite") {
		t.Errorf("footer %q should offer the favourite key", app.footer())
	}
	plain := newApp(newFake("Catalog"))
	if strings.Contains(plain.footer(), "favourite") {
		t.Errorf("footer %q offers a key that would do nothing", plain.footer())
	}
	app, _, _ = favApp(t, true)
	if !strings.Contains(app.footer(), "ctrl+f") {
		t.Errorf("from a text box the footer should offer ctrl+f: %q", app.footer())
	}
}

func TestNothingToFavouriteIsExplained(t *testing.T) {
	app := newApp(newFake("Catalog"))
	app.Update(keyPress("f"))
	if !strings.Contains(app.status, "nothing to favourite") {
		t.Errorf("status %q", app.status)
	}
}

func TestHomeIsTheDashboardOnceAnythingIsFavourited(t *testing.T) {
	deps := NewDeps(context.Background(), storagetest.New(t), styles.Plain())
	if _, ok := NewHomeScreen(deps).(*browse.Model); !ok {
		t.Error("with no favourites, footy should open on Browse")
	}
	deps.Favorites = core.NewFavorites([]models.Favorite{{Kind: models.FavoriteTeam, ID: "eng-arsenal"}})
	if _, ok := NewHomeScreen(deps).(*dashboard.Model); !ok {
		t.Error("with a favourite, footy should open on the dashboard")
	}
}
