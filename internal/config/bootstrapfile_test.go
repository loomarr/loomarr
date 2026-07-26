package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bootstrap file tier (config-design §1, V5). Precedence is `env > file > default`,
// and each rung is asserted separately: a tier that silently loses to the one below it
// is the kind of bug an operator only finds when their pin stops working.

func writeFile(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, BootstrapFileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Rung 1: with no env and no file, the built-in default applies — the behaviour that
// existed before this tier, unchanged.
func TestBootstrapTier_DefaultWhenNothingElse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DATABASE_URL", "sqlite://"+dir+"/loomarr.db")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want the built-in default", c.ListenAddr)
	}
}

// Rung 2: the file fills a gap the environment left.
func TestBootstrapTier_FileBeatsDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DATABASE_URL", "sqlite://"+dir+"/loomarr.db")
	writeFile(t, dir, `{"values":{"LISTEN_ADDR":":9999"}}`)

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":9999" {
		t.Errorf("ListenAddr = %q, want the file's value to beat the default", c.ListenAddr)
	}
}

// Rung 3 — THE one that matters. A GitOps operator pins a key via env; the wizard must
// never be able to override it by writing a file. If this inverts, someone's declared
// infrastructure silently stops being what runs.
func TestBootstrapTier_EnvBeatsFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DATABASE_URL", "sqlite://"+dir+"/loomarr.db")
	t.Setenv("LISTEN_ADDR", ":7777")
	writeFile(t, dir, `{"values":{"LISTEN_ADDR":":9999"}}`)

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":7777" {
		t.Errorf("ListenAddr = %q — the FILE overrode an ENV pin (§1 precedence inverted)", c.ListenAddr)
	}
}

// Load must not leak the file's values into the process environment: main calls it once,
// but tests call it repeatedly, and a leaked var makes them order-dependent.
func TestBootstrapTier_DoesNotLeakIntoEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DATABASE_URL", "sqlite://"+dir+"/loomarr.db")
	writeFile(t, dir, `{"values":{"LOG_LEVEL":"debug"}}`)

	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	if v, ok := os.LookupEnv("LOG_LEVEL"); ok {
		t.Errorf("LOG_LEVEL leaked into the environment as %q", v)
	}
}

// A missing file is the NORMAL state (env-configured or first run), never an error.
func TestBootstrapTier_MissingFileIsNotAnError(t *testing.T) {
	got, err := LoadBootstrapFile(t.TempDir())
	if err != nil {
		t.Fatalf("a missing bootstrap file must not error: %v", err)
	}
	if got != nil {
		t.Errorf("expected no values, got %v", got)
	}
}

// A malformed file fails the boot LOUDLY. This runs before the store opens, so starting
// with configuration the operator believes is applied — and isn't — is worse than not
// starting at all.
func TestBootstrapTier_MalformedFileFailsBoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, `{not json`)
	if _, err := LoadBootstrapFile(dir); err == nil {
		t.Fatal("a malformed bootstrap file must fail the boot, not be ignored")
	}
}

// An app-managed key here is a CATEGORY error, not a typo: it would create two places
// to look for one answer. Rejected by name, and pointed at where it belongs.
func TestBootstrapTier_RejectsAppManagedKeys(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, `{"values":{"LIBRARY_URL":"http://emby:8096"}}`)
	_, err := LoadBootstrapFile(dir)
	if err == nil {
		t.Fatal("an app-managed key in the bootstrap file must be rejected")
	}
	if !strings.Contains(err.Error(), "LIBRARY_URL") {
		t.Errorf("the error must name the offending key, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Settings") {
		t.Errorf("the error should point at where the key belongs, got: %v", err)
	}
}

// Round-trip: what the wizard writes is what the next boot reads.
func TestBootstrapTier_WriteThenRead(t *testing.T) {
	dir := t.TempDir()
	want := map[string]string{"DATABASE_URL": "postgres://u:p@db:5432/loomarr"}
	if err := WriteBootstrapFile(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadBootstrapFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got["DATABASE_URL"] != want["DATABASE_URL"] {
		t.Errorf("round-trip lost the value: %v", got)
	}
	// 0600 — DATABASE_URL routinely carries a password.
	info, err := os.Stat(filepath.Join(dir, BootstrapFileName))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("bootstrap file mode = %o, want 0600 (it can hold a DSN password)", perm)
	}
}

// The writer refuses app-managed keys too — the reader's rule enforced at both ends, so
// a bad file can't be created in the first place.
func TestBootstrapTier_WriteRejectsAppManagedKeys(t *testing.T) {
	if err := WriteBootstrapFile(t.TempDir(), map[string]string{"LLM_URL": "http://ollama:11434"}); err == nil {
		t.Fatal("writing an app-managed key must be refused")
	}
}

// The wizard asks this before offering an editable field: an env-pinned key must render
// as pinned, not as something the operator can change and then watch revert.
func TestPinnedByEnv(t *testing.T) {
	if PinnedByEnv("LOOMARR_DEFINITELY_UNSET_KEY") {
		t.Error("an unset var must not report as pinned")
	}
	t.Setenv("LOOMARR_TEST_PIN", "x")
	if !PinnedByEnv("LOOMARR_TEST_PIN") {
		t.Error("a set var must report as pinned")
	}
}

// DataDirFor decides where the file lives. A SQLite URL puts it beside the database so
// the documented /data volume carries both.
func TestDataDirFor(t *testing.T) {
	if got := DataDirFor("sqlite:///data/loomarr.db"); got != "/data" {
		t.Errorf("sqlite dir = %q, want /data", got)
	}
	if got := DataDirFor("postgres://u:p@db:5432/loomarr"); got != "/data" {
		t.Errorf("postgres dir = %q, want the conventional /data", got)
	}
}

// The migration trap: DataDirFor is scheme-dependent, so migrating SQLite→Postgres moves
// where the next boot LOOKS for the bootstrap file. With the database somewhere other
// than /data, the file recording the switch would be written beside the SQLite database
// and never read again — the app boots back onto SQLite, having apparently migrated.
// The search path is what stops that, so it is asserted directly.
func TestBootstrapDirsForSpansTheSchemeChange(t *testing.T) {
	// A non-default SQLite path: its own directory first (an operator who pinned that
	// path meant it), then the conventional one the Postgres boot will read.
	got := BootstrapDirsFor("sqlite:///srv/loomarr/loomarr.db")
	want := []string{"/srv/loomarr", "/data"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("dirs = %v, want %v", got, want)
	}

	// Already conventional, or Postgres: one directory, no pointless duplicate.
	for _, url := range []string{"sqlite:///data/loomarr.db", "postgres://u:p@db:5432/loomarr"} {
		if got := BootstrapDirsFor(url); len(got) != 1 || got[0] != "/data" {
			t.Errorf("dirs for %q = %v, want [/data]", url, got)
		}
	}
}

// The search must not turn a malformed file into a silent fallthrough: a file that
// exists and is wrong is an operator error to surface, not a reason to quietly use a
// different one from another directory.
func TestBootstrapSearchStopsOnAMalformedFile(t *testing.T) {
	bad, good := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, BootstrapFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteBootstrapFile(good, map[string]string{"LOG_LEVEL": "debug"}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadBootstrapSearch([]string{bad, good}); err == nil {
		t.Fatal("a malformed first file must fail the boot, not fall through to the second")
	}
}

// The normal case the search adds: nothing in the first directory, a real file in the
// second. This is exactly the post-migration boot.
func TestBootstrapSearchFallsThroughAMissingFile(t *testing.T) {
	missing, good := t.TempDir(), t.TempDir()
	if err := WriteBootstrapFile(good, map[string]string{"DATABASE_URL": "postgres://u:p@db:5432/loomarr"}); err != nil {
		t.Fatal(err)
	}
	values, err := loadBootstrapSearch([]string{missing, good})
	if err != nil {
		t.Fatal(err)
	}
	if values["DATABASE_URL"] != "postgres://u:p@db:5432/loomarr" {
		t.Errorf("DATABASE_URL = %q, want the value from the second directory", values["DATABASE_URL"])
	}
}
