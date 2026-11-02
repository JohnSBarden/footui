package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/config"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Where a setting's value came from, for `footui config show`.
const (
	fromFlag    = "flag"
	fromEnv     = "environment"
	fromFile    = "config file"
	fromDefault = "default"
)

// settings are the resolved values: flag, then environment, then config
// file, then default.
type settings struct {
	theme      styles.Mode
	accent     styles.Accent
	staleAfter time.Duration
	dbPath     string
	noColor    bool
	debug      bool
	// sources records where each value came from, keyed by setting name.
	sources map[string]string
}

// envSetting falls back to the legacy FOOTY_ name for old shell profiles.
func envSetting(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return os.Getenv(strings.Replace(name, "FOOTUI_", "FOOTY_", 1))
}

func configPath() string {
	if p := envSetting("FOOTUI_CONFIG"); p != "" {
		return p
	}
	return config.DefaultPath()
}

// resolveSettings runs before every command so a bad config file fails
// fast.
func (g *globalFlags) resolveSettings(cmd *cobra.Command) error {
	file, err := config.Load(configPath())
	if err != nil {
		return err
	}
	s := settings{sources: map[string]string{}}
	changed := func(name string) bool {
		f := cmd.Flags().Lookup(name)
		return f != nil && f.Changed
	}

	themeName, src := "", fromDefault
	switch {
	case changed("theme"):
		themeName, src = g.theme, fromFlag
	case envSetting("FOOTUI_THEME") != "":
		themeName, src = envSetting("FOOTUI_THEME"), fromEnv
	case file.Theme != nil:
		themeName, src = *file.Theme, fromFile
	}
	mode, err := styles.ParseMode(themeName)
	if err != nil {
		return fmt.Errorf("%s (from %s)", err, src)
	}
	s.theme, s.sources["theme"] = mode, src

	accentName, src := "", fromDefault
	switch {
	case envSetting("FOOTUI_ACCENT") != "":
		accentName, src = envSetting("FOOTUI_ACCENT"), fromEnv
	case file.Accent != nil:
		accentName, src = *file.Accent, fromFile
	}
	accent, err := styles.ParseAccent(accentName)
	if err != nil {
		return fmt.Errorf("%s (from %s)", err, src)
	}
	s.accent, s.sources["accent"] = accent, src
	// Must be set before any theme is built.
	styles.UseAccent(accent)

	switch {
	case changed("stale-after"):
		s.staleAfter, s.sources["stale_after"] = g.staleAfter, fromFlag
	case file.StaleAfter != nil:
		s.staleAfter, s.sources["stale_after"] = time.Duration(*file.StaleAfter), fromFile
	default:
		s.staleAfter, s.sources["stale_after"] = DefaultStaleAfter, fromDefault
	}

	switch {
	case changed("db"):
		s.dbPath, s.sources["db"] = g.dbPath, fromFlag
	case file.DB != nil:
		s.dbPath, s.sources["db"] = *file.DB, fromFile
	default:
		s.dbPath, s.sources["db"] = config.DefaultDBPath(), fromDefault
	}

	switch {
	case changed("no-color"):
		s.noColor, s.sources["no_color"] = g.noColor, fromFlag
	case os.Getenv("NO_COLOR") != "":
		s.noColor, s.sources["no_color"] = true, fromEnv
	case file.NoColor != nil:
		s.noColor, s.sources["no_color"] = *file.NoColor, fromFile
	default:
		s.sources["no_color"] = fromDefault
	}

	switch env := envSetting("FOOTUI_DEBUG"); {
	case changed("debug"):
		s.debug, s.sources["debug"] = g.debug, fromFlag
	case env != "":
		on, err := strconv.ParseBool(env)
		if err != nil {
			return fmt.Errorf("FOOTUI_DEBUG=%q: want 1 or 0 (from %s)", env, fromEnv)
		}
		s.debug, s.sources["debug"] = on, fromEnv
	case file.Debug != nil:
		s.debug, s.sources["debug"] = *file.Debug, fromFile
	default:
		s.sources["debug"] = fromDefault
	}

	g.settings = s
	return nil
}
