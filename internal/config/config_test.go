package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// With nothing set, §15 defaults must apply. (t.Setenv-free: env.Parse reads
	// the process env; the test env has none of these set.)
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
	if c.SeasonPrecision != "series" {
		t.Errorf("SeasonPrecision default = %q, want series", c.SeasonPrecision)
	}
	if !c.AutoMigrate {
		t.Error("AutoMigrate default = false, want true")
	}
	if c.RequestTTL != 48*time.Hour {
		t.Errorf("RequestTTL default = %v, want 48h", c.RequestTTL)
	}
	if c.SessionTTL != 720*time.Hour {
		t.Errorf("SessionTTL default = %v, want 720h", c.SessionTTL)
	}
}

func TestLoadOverride(t *testing.T) {
	t.Setenv("LISTEN_ADDR", ":9999")
	t.Setenv("RECONCILE_EVERY", "30s")
	t.Setenv("JOB_WORKERS", "4")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ListenAddr != ":9999" {
		t.Errorf("ListenAddr = %q, want :9999", c.ListenAddr)
	}
	if c.ReconcileEvery != 30*time.Second {
		t.Errorf("ReconcileEvery = %v, want 30s", c.ReconcileEvery)
	}
	if c.JobWorkers != 4 {
		t.Errorf("JobWorkers = %d, want 4", c.JobWorkers)
	}
}
