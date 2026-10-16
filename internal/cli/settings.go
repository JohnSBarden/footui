package cli

import (
	"fmt"
	"os"
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

// settings are the resolved values every command runs with. Each is
// taken from, in order: its command-line flag, its environment variable,
// the config file, and the built-in default.
type settings struct {
	theme      styles.Mode
	staleAfter time.Duration
	dbPath     string
	noColor    bool
	// sources records where each value came from, keyed by setting name.
	sources map[string]string
}

// envSetting reads a FOOTUI_ variable, falling back to the FOOTY_ name it
// had before the command was renamed, so existing shell profiles keep
// working.
func envSetting(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return os.Getenv(strings.Replace(name, "FOOTUI_", "FOOTY_", 1))
}

// configPath is the settings file in use: $FOOTUI_CONFIG when set,
// otherwise the XDG default.
func configPath() string {
	if p := envSetting("FOOTUI_CONFIG"); p != "" {
		return p
	}
	return config.DefaultPath()
}

// resolveSettings merges flags, environment, and the config file. It runs
// before every command, so a malformed config file fails fast with the
// file's name rather than surfacing halfway through a refresh.
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

	// theme: --theme, FOOTUI_THEME, file, weak.
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

	// stale_after: --stale-after, file, 6h.
	switch {
	case changed("stale-after"):
		s.staleAfter, s.sources["stale_after"] = g.staleAfter, fromFlag
	case file.StaleAfter != nil:
		s.staleAfter, s.sources["stale_after"] = time.Duration(*file.StaleAfter), fromFile
	default:
		s.staleAfter, s.sources["stale_after"] = DefaultStaleAfter, fromDefault
	}

	// db: --db, file, XDG default.
	switch {
	case changed("db"):
		s.dbPath, s.sources["db"] = g.dbPath, fromFlag
	case file.DB != nil:
		s.dbPath, s.sources["db"] = *file.DB, fromFile
	default:
		s.dbPath, s.sources["db"] = config.DefaultDBPath(), fromDefault
	}

	// no_color: --no-color, NO_COLOR, file, false.
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

	g.settings = s
	return nil
}
