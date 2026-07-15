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
