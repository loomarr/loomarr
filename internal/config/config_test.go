package config

import "testing"

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
