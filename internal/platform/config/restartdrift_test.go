package config

import (
	"strings"
	"testing"
)

// The RestartRequired flag is DERIVED from running-vs-resolved bootstrap values, so it
// cannot nag about a restart the operator already undid (config-design §3).
func TestRestartDrift(t *testing.T) {
	t.Run("no drift when the running config matches the environment", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "sqlite:///data/a.db")
		running, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if got := RestartDrift(running, nil, nil)(); len(got) != 0 {
			t.Errorf("drift = %v, want none — nothing changed", got)
		}
	})

	t.Run("names the specific key that changed", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "sqlite:///data/a.db")
		running, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		// The operator saves a new database target; this process is still on the old one.
		t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/loomarr")

		got := RestartDrift(running, nil, nil)()
		if len(got) != 1 || got[0] != "DATABASE_URL" {
			t.Errorf("drift = %v, want [DATABASE_URL] — the UI must name WHICH setting", got)
		}
	})

	t.Run("names any boot key, not only the database and listener", func(t *testing.T) {
		running, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("LOOMARR_PPROF", "true")
		t.Setenv("LOG_LEVEL", "debug")

		got := RestartDrift(running, nil, nil)()
		if strings.Join(got, ",") != "LOG_LEVEL,LOOMARR_PPROF" {
			t.Errorf("drift = %v, want [LOG_LEVEL LOOMARR_PPROF]", got)
		}
	})

	t.Run("clears when the change is reverted", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "sqlite:///data/a.db")
		running, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		drift := RestartDrift(running, nil, nil)

		t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/loomarr")
		if len(drift()) == 0 {
			t.Fatal("expected drift after the change")
		}
		// ⚠ The whole reason this is derived rather than a sticky boolean: undoing the
		// edit must stop the nagging, and a flag set at save time never would.
		t.Setenv("DATABASE_URL", "sqlite:///data/a.db")
		if got := drift(); len(got) != 0 {
			t.Errorf("drift = %v after reverting, want none", got)
		}
	})

	t.Run("a nil baseline reports no drift", func(t *testing.T) {
		// Safe direction: a false "restart required" points the operator at an action
		// that cannot help.
		if got := RestartDrift(nil, nil, nil)(); got != nil {
			t.Errorf("drift = %v with no baseline, want nil", got)
		}
	})

	t.Run("combines bootstrap and generation drift deterministically", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "sqlite:///data/a.db")
		running, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/loomarr")
		desired := map[string]string{
			"filler.dir":       "/clips/new",
			"filler.watch_dir": "/watch/new",
		}
		drift := RestartDrift(running, map[string]string{
			"filler.watch_dir": "/watch/old",
			"filler.dir":       "/clips/old",
		}, func(key string) string { return desired[key] })

		got := drift()
		want := []string{"DATABASE_URL", "filler.dir", "filler.watch_dir"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("drift = %v, want %v", got, want)
		}
	})

	t.Run("generation drift clears when desired values are reverted", func(t *testing.T) {
		desired := map[string]string{"filler.dir": "/clips/new"}
		drift := RestartDrift(nil, map[string]string{"filler.dir": "/clips/old"},
			func(key string) string { return desired[key] })
		if got := drift(); len(got) != 1 || got[0] != "filler.dir" {
			t.Fatalf("drift = %v, want [filler.dir]", got)
		}
		desired["filler.dir"] = "/clips/old"
		if got := drift(); len(got) != 0 {
			t.Errorf("drift after revert = %v, want none", got)
		}
	})
}
