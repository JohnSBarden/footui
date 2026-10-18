package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/config"
	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider/espn"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/sqlite"
	"github.com/JohnSBarden/footui/internal/textfold"
	"github.com/JohnSBarden/footui/internal/tui"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// globalFlags are the flags every command shares, registered as persistent
// flags on the root command.
type globalFlags struct {
	dbPath  string
	noColor bool
	// staleAfter is how old a cached current season may be before
	// opening the TUI refreshes it; zero disables the check.
	staleAfter time.Duration
	// theme is the club theme mode name, "weak" or "strong".
	theme string

	// settings are the values commands actually use: the flags above
	// merged with the environment and the config file. Filled in by
	// resolveSettings before any command runs.
	settings settings
}

// openStore opens (creating and migrating if needed) the local database and
// returns it alongside the repositories the TUI and sync layer use. The
// caller closes the returned *sql.DB.
func (g *globalFlags) openStore() (*sql.DB, storage.Repositories, error) {
	db, err := sqlite.Open(g.dbPathOrDefault())
	if err != nil {
		return nil, nil, err
	}
	return db, sqlite.NewStore(db), nil
}

// dbPathOrDefault reports the database path a command will use, for
// user-facing messages.
func (g *globalFlags) dbPathOrDefault() string {
	if g.settings.dbPath != "" {
		return g.settings.dbPath
	}
	return config.DefaultDBPath()
}

// launchTUI opens the store, builds screen dependencies, and runs the app
// with the screen build returns. Every TUI-launching command funnels
// through here so database lifetime and theme resolution live in one place.
func (g *globalFlags) launchTUI(ctx context.Context, build func(core.Deps) (core.Screen, error)) error {
	db, repos, err := g.openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	now := time.Now()
	deps := tui.NewDeps(ctx, repos, styles.BaseFor(g.settings.theme, g.settings.noColor))
	deps.Refresh = tuiRefresher(repos, g.settings.staleAfter)
	if stale := g.settings.staleAfter; stale > 0 {
		deps.ClubDue = func(club models.Team) (string, bool) {
			return clubDue(ctx, repos, club, time.Now(), stale)
		}
	}
	deps.Catalog = tuiCatalog(now)
	deps.Appearance = &styles.Appearance{Mode: g.settings.theme}
	deps.SaveTheme = g.saveTheme
	favs, err := repos.ListFavorites(ctx)
	if err != nil {
		return err
	}
	deps.Favorites = core.NewFavorites(favs)

	deps.ProviderPause = func() (string, time.Time) { return "ESPN", espn.PausedUntil() }
	deps.Version = Version

	screen, err := build(deps)
	if err != nil {
		return err
	}
	// A database with no competition cached has nothing to show behind
	// the splash, so the splash waits for the first league to land.
	comps, err := repos.ListCompetitions(ctx)
	if err != nil {
		return err
	}
	return tui.Run(deps, screen, len(comps) == 0, staleRefreshes(ctx, repos, now, g.settings.staleAfter)...)
}

// saveTheme makes a theme chosen in the interface the default for next
// time. A theme set by flag or environment would override the file, so
// writing it there would look saved and then not stick.
func (g *globalFlags) saveTheme(mode styles.Mode) error {
	switch g.settings.sources["theme"] {
	case fromFlag:
		return errors.New("--theme overrides the settings file")
	case fromEnv:
		return errors.New("FOOTUI_THEME overrides the settings file")
	}
	return config.SetTheme(configPath(), mode.String())
}

// resolveCompetition turns a free-text competition name into exactly one
// cached competition. Matching is done against the local database rather
// than the in-code Scope list so the message can distinguish "that league
// isn't supported" from "it's supported but you haven't refreshed it yet".
func resolveCompetition(ctx context.Context, repos storage.Repositories, query string) (models.Competition, error) {
	matches, err := repos.FindCompetitionsByName(ctx, query)
	if err != nil {
		return models.Competition{}, err
	}
	// An exact ID or name beats a substring: "world-cup" is the World
	// Cup, not also the women's one whose ID contains it.
	for _, c := range matches {
		if strings.EqualFold(c.ID, query) || textfold.Fold(c.Name) == textfold.Fold(query) {
			return c, nil
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return models.Competition{}, fmt.Errorf(
			"no cached competition matches %q — run `footui refresh --competition <id>` first (see `footui competitions`)", query)
	default:
		var names []string
		for _, c := range matches {
			names = append(names, fmt.Sprintf("%s (%s)", c.Name, c.ID))
		}
		return models.Competition{}, fmt.Errorf("%q matches %d competitions: %s",
			query, len(matches), strings.Join(names, ", "))
	}
}

// cmdContext returns the context a command's work runs under, so a future
// signal handler has one place to hook into.
func cmdContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
