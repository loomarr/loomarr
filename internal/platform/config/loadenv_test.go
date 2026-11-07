package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// LoadEnv parses an explicit environment map. The process environment is never read
// or written, so precedence (env > file > default) needs no os.Setenv to steer it.
func TestLoadEnvIgnoresTheProcessEnvironment(t *testing.T) {
	t.Setenv("LISTEN_ADDR", ":1")
	t.Setenv("DATABASE_URL", "postgres://ambient")
	c, err := LoadEnv(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":8080" || c.DatabaseURL != "sqlite:///data/loomarr.db" {
		t.Errorf("LoadEnv read the process environment: listen=%q db=%q", c.ListenAddr, c.DatabaseURL)
	}
}

func TestLoadEnvFileBeneathEnvWithoutTouchingProcessEnv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, `{"values":{"LISTEN_ADDR":":9999","LOG_LEVEL":"debug"}}`)
	c, err := LoadEnv(map[string]string{
		"DATABASE_URL": "sqlite://" + dir + "/loomarr.db",
		"LOG_LEVEL":    "warn",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":9999" {
		t.Errorf("ListenAddr = %q, want the file value", c.ListenAddr)
	}
	if c.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want the env pin over the file", c.LogLevel)
	}
	if _, ok := os.LookupEnv("LISTEN_ADDR"); ok {
		t.Error("LISTEN_ADDR leaked into the process environment")
	}
}

// A pinned-but-empty var still pins: the file does not fill it (the parser then applies the
// built-in default, as an empty pin always has), matching os.LookupEnv.
func TestLoadEnvEmptyPinBeatsFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, `{"values":{"LISTEN_ADDR":":9999"}}`)
	c, err := LoadEnv(map[string]string{"DATABASE_URL": "sqlite://" + dir + "/loomarr.db", "LISTEN_ADDR": ""})
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want the empty env pin to keep the file out", c.ListenAddr)
	}
}

func TestLoadEnvMetricsTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("  from-file \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadEnv(map[string]string{"LOOMARR_METRICS_TOKEN_FILE": path})
	if err != nil || c.MetricsToken != "from-file" {
		t.Fatalf("LoadEnv = %q, %v; want from-file", c.MetricsToken, err)
	}
	if _, err := LoadEnv(map[string]string{"LOOMARR_METRICS_TOKEN": "x", "LOOMARR_METRICS_TOKEN_FILE": path}); err == nil {
		t.Fatal("both set must be ambiguous")
	}
}

func TestReadSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s")
	if err := os.WriteFile(path, []byte("\t value\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSecret(path)
	if err != nil || got != "value" {
		t.Fatalf("ReadSecret = %q, %v; want value", got, err)
	}
	_, err = ReadSecret(filepath.Join(dir, "missing"))
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing file error = %v, want one naming the path", err)
	}
}

// DataDir is decided once and read back: after it is persisted, a later boot whose
// DATABASE_URL points elsewhere (the SQLite to PostgreSQL cutover) keeps the same
// directory, so the generated installation key does not move.
func TestDataDirIsDecidedOnceAndReadBack(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"DATABASE_URL": "sqlite://" + dir + "/loomarr.db"}
	first, err := LoadEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	if first.DataDir != dir {
		t.Fatalf("first boot DataDir = %q, want %q", first.DataDir, dir)
	}
	if err := first.PersistDataDir(); err != nil {
		t.Fatal(err)
	}
	// A later boot reads the recorded directory instead of re-deriving it from DATABASE_URL.
	if err := UpdateBootstrapFile(dir, map[string]string{"DATA_DIR": "/srv/recorded"}); err != nil {
		t.Fatal(err)
	}
	second, err := LoadEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	if second.DataDir != "/srv/recorded" {
		t.Errorf("second boot DataDir = %q, want the recorded /srv/recorded", second.DataDir)
	}
}

func TestPersistDataDirKeepsOtherValuesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, `{"values":{"LISTEN_ADDR":":9999"}}`)
	env := map[string]string{"DATABASE_URL": "sqlite://" + dir + "/loomarr.db"}
	c, err := LoadEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PersistDataDir(); err != nil {
		t.Fatal(err)
	}
	values, err := LoadBootstrapFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if values["DATA_DIR"] != dir || values["LISTEN_ADDR"] != ":9999" {
		t.Fatalf("file values = %v, want DATA_DIR and the untouched LISTEN_ADDR", values)
	}
	again, err := LoadEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, BootstrapFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := again.PersistDataDir(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(filepath.Join(dir, BootstrapFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(info.ModTime()) {
		t.Error("an already-persisted DataDir rewrote the file")
	}
}

// A fresh install has no file; the first boot creates one beside the data it decided.
func TestPersistDataDirCreatesTheFileOnFirstBoot(t *testing.T) {
	dir := t.TempDir()
	c, err := LoadEnv(map[string]string{"DATABASE_URL": "sqlite://" + dir + "/loomarr.db"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PersistDataDir(); err != nil {
		t.Fatal(err)
	}
	values, err := LoadBootstrapFile(dir)
	if err != nil || values["DATA_DIR"] != dir {
		t.Fatalf("values = %v, %v; want DATA_DIR=%s", values, err, dir)
	}
}

// TZ fed nothing (the Go runtime reads the process TZ itself), so the file no longer carries it.
func TestBootstrapFileRejectsTZ(t *testing.T) {
	if err := WriteBootstrapFile(t.TempDir(), map[string]string{"TZ": "UTC"}); err == nil {
		t.Fatal("TZ is not a bootstrap key")
	}
}

// LOOMARR_AGENT_DISABLE_LIVETV_TUNER is the env-only agent-lane flag; any non-blank value turns it on.
func TestTunerPublishingDisabledFlag(t *testing.T) {
	for value, want := range map[string]bool{"": false, "  ": false, "1": true, "yes": true} {
		c, err := LoadEnv(map[string]string{"LOOMARR_AGENT_DISABLE_LIVETV_TUNER": value})
		if err != nil {
			t.Fatal(err)
		}
		if got := c.TunerPublishingDisabled(); got != want {
			t.Errorf("flag %q: TunerPublishingDisabled = %v, want %v", value, got, want)
		}
	}
	c, err := LoadEnv(map[string]string{})
	if err != nil || c.TunerPublishingDisabled() {
		t.Fatalf("unset flag = %v, %v; want false", c.TunerPublishingDisabled(), err)
	}
}

// Demo mode needs the whole account: half of one would offer visitors a sign-in that cannot work.
func TestLoadEnvDemoAccount(t *testing.T) {
	c, err := LoadEnv(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if c.DemoMode() {
		t.Error("demo mode is on with no demo account")
	}
	c, err = LoadEnv(map[string]string{"LOOMARR_DEMO_USERNAME": " guest ", "LOOMARR_DEMO_PASSWORD": "loomarr-guest"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.DemoMode() || c.DemoUsername != "guest" || c.DemoPassword != "loomarr-guest" {
		t.Errorf("demo account = %q/%q, mode %v", c.DemoUsername, c.DemoPassword, c.DemoMode())
	}
	for _, half := range []map[string]string{
		{"LOOMARR_DEMO_USERNAME": "guest"},
		{"LOOMARR_DEMO_PASSWORD": "loomarr-guest"},
		{"LOOMARR_DEMO_USERNAME": "guest", "LOOMARR_DEMO_PASSWORD": "  "},
	} {
		if _, err := LoadEnv(half); err == nil || !strings.Contains(err.Error(), "LOOMARR_DEMO_USERNAME") {
			t.Errorf("LoadEnv(%v) error = %v, want the pair refused", half, err)
		}
	}
}
