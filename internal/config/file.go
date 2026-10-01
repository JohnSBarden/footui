package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// File is the user's settings file. Every field is optional; a nil field
// means "not set here", so the caller can tell an explicit value from an
// absent one and fall through to the default.
type File struct {
	// Theme is the club theme mode, "weak" or "strong".
	Theme *string `yaml:"theme"`
	// StaleAfter is how old a followed league's current season may get
	// before opening the interface refreshes it. "0" disables.
	StaleAfter *Duration `yaml:"stale_after"`
	// DB overrides the database path. A leading ~/ is expanded.
	DB *string `yaml:"db"`
	// NoColor disables color, as --no-color does.
	NoColor *bool `yaml:"no_color"`
}

// Duration is a time.Duration written the way Go parses them ("6h",
// "90m", "0").
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("line %d: stale_after: %q is not a duration (try 6h or 30m)", n.Line, s)
	}
	*d = Duration(v)
	return nil
}

// DefaultPath returns $XDG_CONFIG_HOME/footui/config.yaml, falling back to
// ~/.config/footui/config.yaml.
func DefaultPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "footui", "config.yaml")
}

// Load reads the settings file at path. A missing file is not an error —
// it is the normal state for anyone who hasn't run `footy config init` —
// and yields an empty File.
//
// Unknown keys are an error. A misspelt setting that silently did nothing
// would be worse than refusing to start: the user would think their
// change had taken.
func Load(path string) (File, error) {
	var f File
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, fmt.Errorf("config: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	// An empty or all-comment file decodes as io.EOF: nothing set.
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return File{}, fmt.Errorf("config %s: %w", path, err)
	}
	if f.DB != nil {
		expanded := expandHome(*f.DB)
		f.DB = &expanded
	}
	return f, nil
}

func expandHome(p string) string {
	if len(p) >= 2 && p[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// Template is what `footy config init` writes: every setting, commented
// out at its default, so the file documents itself.
const Template = `# footy settings. Every line is optional; a flag on the command line
# always wins over what's here, and an environment variable (where one
# exists) wins over this file.

# Club theme: weak keeps your terminal's background and uses club colors
# for highlights; strong paints club screens in the club's colors.
# Environment: FOOTY_THEME. Flag: --theme. Toggle while running: t.
# theme: weak

# When the interface opens, leagues you follow whose current season is
# older than this are refreshed in the background. 0 turns it off.
# Flag: --stale-after.
# stale_after: 6h

# Where the database lives. Flag: --db.
# db: ~/.local/share/footui/footy.db

# Turn color off entirely. Environment: NO_COLOR. Flag: --no-color.
# no_color: false
`

// WriteTemplate creates the settings file at path from Template, refusing
// to overwrite one that exists.
func WriteTemplate(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists — edit it, or remove it first", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return os.WriteFile(path, []byte(Template), 0o644)
}
