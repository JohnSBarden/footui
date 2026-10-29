package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/config"
	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/sources"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/sqlite"
	"github.com/JohnSBarden/footui/internal/textfold"
	"github.com/JohnSBarden/footui/internal/tui"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// globalFlags are the root command's persistent flags.
type globalFlags struct {
	dbPath  string
	noColor bool
	// staleAfter zero disables the launch refresh.
	staleAfter time.Duration
	// theme is "weak" or "strong".
	theme string

	// settings are what commands use: flags merged with env and config
	// file, filled in by resolveSettings. Read these, not the raw flags.
	settings settings
}

// openStore opens and migrates the database; the caller closes the
// *sql.DB.
func (g *globalFlags) openStore() (*sql.DB, storage.Repositories, error) {
	// Printed before the interface takes the screen, which would otherwise
	// stay blank while a big database upgrades.
	db, err := sqlite.OpenNotify(g.dbPathOrDefault(), func(pending int) {
		fmt.Fprintf(os.Stderr, "Upgrading your footui database (%s) — once after an update; this can take a minute…\n",
			components.Plural(pending, "change"))
	})
	if err != nil {
		return nil, nil, err
	}
	return db, sqlite.NewStore(db), nil
}

func (g *globalFlags) dbPathOrDefault() string {
	if g.settings.dbPath != "" {
		return g.settings.dbPath
	}
	return config.DefaultDBPath()
}

// launchTUI is the one place TUI-launching commands open the store and
// build dependencies.
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

	deps.ProviderPause = sources.PausedUntil
	deps.Players = sources.Players()
	deps.Version = Version

	screen, err := build(deps)
	if err != nil {
		return err
	}
	// With nothing cached, the splash waits for the first league to land.
	comps, err := repos.ListCompetitions(ctx)
	if err != nil {
		return err
	}
	return tui.Run(deps, screen, len(comps) == 0, staleRefreshes(ctx, repos, now, g.settings.staleAfter)...)
}

// saveTheme refuses when a flag or env var sets the theme, since the
// saved value would not stick.
func (g *globalFlags) saveTheme(mode styles.Mode) error {
	switch g.settings.sources["theme"] {
	case fromFlag:
		return errors.New("--theme overrides the settings file")
	case fromEnv:
		return errors.New("FOOTUI_THEME overrides the settings file")
	}
	return config.SetTheme(configPath(), mode.String())
}

// resolveCompetition matches one cached competition by free-text name.
func resolveCompetition(ctx context.Context, repos storage.Repositories, query string) (models.Competition, error) {
	matches, err := repos.FindCompetitionsByName(ctx, query)
	if err != nil {
		return models.Competition{}, err
	}
	// An exact ID or name beats a substring ("world-cup" vs the women's).
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

func cmdContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
