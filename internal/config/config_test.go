package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bootstrap surface (config-design §1) is env-only: process topology + the
// keys needed to open the DB. App-managed settings moved to internal/settings
// (their defaults are tested there, via the registry).
func TestLoadDefaults(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ListenAddr != ":8080" {
		t.Errorf("ListenAddr default = %q, want :8080", c.ListenAddr)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel default = %q, want info", c.LogLevel)
	}
	if !c.AutoMigrate {
		t.Error("AutoMigrate default = false, want true")
	}
	// The zero-env promise (§15): `docker run -v loomarr-data:/data loomarr`
	// boots into the wizard. Without a default the process comes up store-less
	// and never-ready, and the promise held only via compose — which sets this
	// exact value.
	if c.DatabaseURL != "sqlite:///data/loomarr.db" {
		t.Errorf("DatabaseURL default = %q, want sqlite:///data/loomarr.db", c.DatabaseURL)
	}
}

// A store-less boot is what the missing default produced, so assert the
// resolved value is usable rather than merely non-empty.
func TestLoadDatabaseURLOverride(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@db:5432/loomarr")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DatabaseURL != "postgres://u:p@db:5432/loomarr" {
		t.Errorf("DatabaseURL = %q, want the env pin to win over the default", c.DatabaseURL)
	}
}

func TestLoadOverride(t *testing.T) {
	t.Setenv("LISTEN_ADDR", ":9999")
	t.Setenv("AUTO_MIGRATE", "false")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ListenAddr != ":9999" {
		t.Errorf("ListenAddr = %q, want :9999", c.ListenAddr)
	}
	if c.AutoMigrate {
		t.Error("AUTO_MIGRATE=false should disable auto-migrate")
	}
}

// LOOMARR_METRICS_TOKEN / _FILE (§7, §15): the Prometheus scrape credential follows the
// <VAR> | <VAR>_FILE secret idiom. Unset is legal (metrics then refuse, fail closed).
func TestLoadMetricsToken(t *testing.T) {
	file := func(t *testing.T, content string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "metrics-token")
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	t.Run("unset", func(t *testing.T) {
		c, err := Load()
		if err != nil || c.MetricsToken != "" {
			t.Fatalf("Load = %q, %v; want empty token, nil", c.MetricsToken, err)
		}
	})
	t.Run("direct", func(t *testing.T) {
		t.Setenv("LOOMARR_METRICS_TOKEN", "abc")
		if c, err := Load(); err != nil || c.MetricsToken != "abc" {
			t.Fatalf("Load = %q, %v; want abc", c.MetricsToken, err)
		}
	})
	t.Run("file trims the trailing newline", func(t *testing.T) {
		t.Setenv("LOOMARR_METRICS_TOKEN_FILE", file(t, "from-file\n"))
		if c, err := Load(); err != nil || c.MetricsToken != "from-file" {
			t.Fatalf("Load = %q, %v; want from-file", c.MetricsToken, err)
		}
	})
	t.Run("both set is an error", func(t *testing.T) {
		t.Setenv("LOOMARR_METRICS_TOKEN", "abc")
		t.Setenv("LOOMARR_METRICS_TOKEN_FILE", file(t, "def"))
		if _, err := Load(); err == nil {
			t.Fatal("Load with both set = nil error, want ambiguity error")
		}
	})
	t.Run("unreadable file is an error, not an open endpoint", func(t *testing.T) {
		t.Setenv("LOOMARR_METRICS_TOKEN_FILE", filepath.Join(t.TempDir(), "missing"))
		if _, err := Load(); err == nil {
			t.Fatal("Load with an unreadable token file = nil error, want failure")
		}
	})
	t.Run("error never echoes the token", func(t *testing.T) {
		t.Setenv("LOOMARR_METRICS_TOKEN", "super-secret-value")
		t.Setenv("LOOMARR_METRICS_TOKEN_FILE", file(t, "x"))
		_, err := Load()
		if err == nil || strings.Contains(err.Error(), "super-secret-value") {
			t.Fatalf("err = %v; must exist and not contain the token", err)
		}
	})
}
