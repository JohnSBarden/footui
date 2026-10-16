package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMissingFileIsEmptyNotAnError(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("a missing file is the normal case: %v", err)
	}
	if f.Theme != nil || f.StaleAfter != nil || f.DB != nil || f.NoColor != nil {
		t.Errorf("nothing should be set: %+v", f)
	}
}

// The template is all comments, so a freshly initialised file must load
// and change nothing.
func TestTemplateLoadsAsEmpty(t *testing.T) {
	f, err := Load(write(t, Template))
	if err != nil {
		t.Fatal(err)
	}
	if f.Theme != nil || f.StaleAfter != nil {
		t.Errorf("the commented template set something: %+v", f)
	}
}

func TestValuesAreRead(t *testing.T) {
	home, _ := os.UserHomeDir()
	f, err := Load(write(t, "theme: strong\nstale_after: 90m\ndb: ~/x/footui.db\nno_color: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if *f.Theme != "strong" {
		t.Errorf("theme %q", *f.Theme)
	}
	if time.Duration(*f.StaleAfter) != 90*time.Minute {
		t.Errorf("stale_after %v", time.Duration(*f.StaleAfter))
	}
	if *f.DB != filepath.Join(home, "x/footui.db") {
		t.Errorf("db %q, want ~ expanded", *f.DB)
	}
	if !*f.NoColor {
		t.Error("no_color should be true")
	}
}

func TestZeroDisablesRatherThanMeaningUnset(t *testing.T) {
	f, err := Load(write(t, "stale_after: 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if f.StaleAfter == nil || *f.StaleAfter != 0 {
		t.Errorf("an explicit 0 must be kept, got %v", f.StaleAfter)
	}
}

func TestMistakesAreReportedWithTheFile(t *testing.T) {
	for name, body := range map[string]string{
		"typo":         "thmee: strong\n",
		"bad duration": "stale_after: soon\n",
		"bad yaml":     "theme: [unclosed\n",
	} {
		p := write(t, body)
		_, err := Load(p)
		if err == nil {
			t.Errorf("%s: should be an error", name)
			continue
		}
		if !strings.Contains(err.Error(), p) {
			t.Errorf("%s: error should name the file, got %v", name, err)
		}
	}
}

func TestWriteTemplateRefusesToOverwrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.yaml")
	if err := WriteTemplate(p); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := WriteTemplate(p); err == nil {
		t.Error("an existing settings file must not be overwritten")
	}
}

func TestDefaultPathFollowsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/cfg")
	if got := DefaultPath(); got != "/cfg/footui/config.yaml" {
		t.Errorf("got %q", got)
	}
}

func TestSetThemeUncommentsTheTemplateLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := SetTheme(path, "strong"); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Theme == nil || *f.Theme != "strong" {
		t.Fatalf("theme = %v, want strong", f.Theme)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "# stale_after: 6h") {
		t.Error("the rest of the template's comments should survive")
	}
}

func TestSetThemeReplacesAnExistingValueAndKeepsComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	orig := "# mine\ntheme: strong\nstale_after: 2h\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetTheme(path, "weak"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if want := "# mine\ntheme: weak\nstale_after: 2h\n"; string(data) != want {
		t.Errorf("got %q, want %q", data, want)
	}
}

func TestSetThemeAppendsWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("no_color: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetTheme(path, "strong"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if want := "no_color: false\ntheme: strong\n"; string(data) != want {
		t.Errorf("got %q, want %q", data, want)
	}
}
