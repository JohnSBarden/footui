package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// File is the user's settings file. A nil field means "not set here".
type File struct {
	// Theme is "weak" or "strong".
	Theme *string `yaml:"theme"`
	// Accent is "lime", "terminal", or #rrggbb.
	Accent *string `yaml:"accent"`
	// StaleAfter: "0" disables the launch refresh.
	StaleAfter *Duration `yaml:"stale_after"`
	// DB overrides the database path. A leading ~/ is expanded.
	DB      *string `yaml:"db"`
	NoColor *bool   `yaml:"no_color"`
	// Debug writes a debug log for bug reports.
	Debug *bool `yaml:"debug"`
}

// Duration is a time.Duration in Go syntax ("6h", "90m", "0").
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

// Load reads the settings file at path; a missing file yields an empty
// File. Unknown keys are an error so a misspelt setting isn't silently
// ignored.
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

// Template is what `footui config init` writes: every setting, commented
// out at its default.
const Template = `# footui settings. Every line is optional; a flag on the command line
# always wins over what's here, and an environment variable (where one
# exists) wins over this file.

# Club theme: weak keeps your terminal's background and uses club colors
# for highlights; strong paints club screens in the club's colors.
# Environment: FOOTUI_THEME. Flag: --theme. Pressing t while running
# switches theme and saves the choice here.
# theme: weak

# footui's own highlight colour, on screens that belong to no club: lime,
# any colour as #rrggbb, or terminal to draw everything in your terminal's
# own palette so footui follows its theme. Environment: FOOTUI_ACCENT.
# accent: lime

# When the interface opens, leagues you follow whose current season is
# older than this are refreshed in the background. 0 turns it off.
# Flag: --stale-after.
# stale_after: 6h

# Where the database lives. Flag: --db.
# db: ~/.local/share/footui/footui.db

# Write a debug log of every request, retry, rate-limit pause, migration
# and identity match to ~/.local/state/footui/debug.log, to attach to a
# bug report. The log panel (L) shows the same lines. Environment:
# FOOTUI_DEBUG=1. Flag: --debug.
# debug: false

# Turn color off entirely. Environment: NO_COLOR. Flag: --no-color.
# no_color: false
`

// WriteTemplate writes Template to path, refusing to overwrite.
func WriteTemplate(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists — edit it, or remove it first", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return os.WriteFile(path, []byte(Template), 0o600)
}

// SetTheme saves theme to the file at path (created from Template if
// missing). It edits the one line rather than re-encoding, so the user's
// comments and layout survive.
func SetTheme(path, theme string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		data, err = []byte(Template), nil
	}
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	line := "theme: " + theme
	lines := strings.Split(string(data), "\n")
	commented := -1
	set := false
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(trimmed, "theme:"):
			lines[i] = line
			set = true
		case commented < 0 && strings.HasPrefix(trimmed, "# theme:"):
			commented = i
		}
	}
	switch {
	case set:
	case commented >= 0:
		lines[commented] = line
	default:
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines = lines[:n-1]
		}
		lines = append(lines, line, "")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
}
