package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func withConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOOTUI_CONFIG", p)
	// A developer's pre-rename variables must not leak into the tests.
	t.Setenv("FOOTY_CONFIG", "")
	t.Setenv("FOOTY_THEME", "")
	return p
}

// setting pulls one row out of `footui config show`: its value and source.
func setting(t *testing.T, out, name string) (value, source string) {
	t.Helper()
	m := regexp.MustCompile(`(?m)^\s+` + name + `\s+(\S.*?)\s+\((.+)\)$`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no %s row in:\n%s", name, out)
	}
	return m[1], m[2]
}

func TestDefaultsWhenNothingIsSet(t *testing.T) {
	t.Setenv("FOOTUI_THEME", "")
	t.Setenv("NO_COLOR", "")
	out, err := run(t, "config")
	if err != nil {
		t.Fatal(err)
	}
	if v, src := setting(t, out, "theme"); v != "weak" || src != fromDefault {
		t.Errorf("theme %q from %q", v, src)
	}
	if v, src := setting(t, out, "stale_after"); v != "6h0m0s" || src != fromDefault {
		t.Errorf("stale_after %q from %q", v, src)
	}
}

func TestConfigFileThenEnvThenFlag(t *testing.T) {
	withConfig(t, "theme: strong\nstale_after: 0\ndb: /tmp/from-file.db\n")
	t.Setenv("FOOTUI_THEME", "")

	out, _ := run(t, "config")
	if v, src := setting(t, out, "theme"); v != "strong" || src != fromFile {
		t.Errorf("file: theme %q from %q", v, src)
	}
	if v, _ := setting(t, out, "stale_after"); v != "off" {
		t.Errorf("file: stale_after %q, an explicit 0 should switch it off", v)
	}
	if v, src := setting(t, out, "db"); v != "/tmp/from-file.db" || src != fromFile {
		t.Errorf("file: db %q from %q", v, src)
	}

	t.Setenv("FOOTUI_THEME", "weak")
	out, _ = run(t, "config")
	if v, src := setting(t, out, "theme"); v != "weak" || src != fromEnv {
		t.Errorf("env should beat the file: theme %q from %q", v, src)
	}

	out, _ = run(t, "--theme", "strong", "--db", "/tmp/flag.db", "config")
	if v, src := setting(t, out, "theme"); v != "strong" || src != fromFlag {
		t.Errorf("flag should beat env: theme %q from %q", v, src)
	}
	if v, src := setting(t, out, "db"); v != "/tmp/flag.db" || src != fromFlag {
		t.Errorf("flag should beat file: db %q from %q", v, src)
	}
}

// The database a command opens is the configured one, not just the one
// `config show` reports.
func TestCommandsUseTheConfiguredDatabase(t *testing.T) {
	db := filepath.Join(t.TempDir(), "configured.db")
	withConfig(t, "db: "+db+"\n")
	out, err := run(t, "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, db) {
		t.Errorf("status should report the configured database, got:\n%s", out)
	}
	if _, err := os.Stat(db); err != nil {
		t.Errorf("the configured database was not opened: %v", err)
	}
}

func TestABrokenConfigFailsCommandsButNotTheConfigTools(t *testing.T) {
	p := withConfig(t, "thmee: strong\n")

	if _, err := run(t, "status"); err == nil || !strings.Contains(err.Error(), p) {
		t.Errorf("a typo should fail loudly, naming the file; got %v", err)
	}
	out, err := run(t, "config", "path")
	if err != nil || strings.TrimSpace(out) != p {
		t.Errorf("config path must work with a broken file: %q, %v", out, err)
	}
}

func TestBadThemeNamesItsSource(t *testing.T) {
	withConfig(t, "theme: loud\n")
	t.Setenv("FOOTUI_THEME", "")
	_, err := run(t, "config")
	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Errorf("want an error naming the config file as the source, got %v", err)
	}
}

func TestConfigInitWritesTheTemplateOnce(t *testing.T) {
	p := filepath.Join(t.TempDir(), "footui", "config.yaml")
	t.Setenv("FOOTUI_CONFIG", p)

	if _, err := run(t, "config", "init"); err != nil {
		t.Fatal(err)
	}
	if body, _ := os.ReadFile(p); !strings.Contains(string(body), "# theme: weak") {
		t.Errorf("template not written:\n%s", body)
	}
	if _, err := run(t, "config", "init"); err == nil {
		t.Error("a second init must not overwrite the user's file")
	}
}

func TestLegacyFootyEnvironmentStillWorks(t *testing.T) {
	t.Setenv("FOOTUI_THEME", "")
	t.Setenv("FOOTY_THEME", "strong")
	if got := envSetting("FOOTUI_THEME"); got != "strong" {
		t.Fatalf("legacy fallback: got %q, want strong", got)
	}
	t.Setenv("FOOTUI_THEME", "weak")
	if got := envSetting("FOOTUI_THEME"); got != "weak" {
		t.Fatalf("new name wins: got %q, want weak", got)
	}
}
