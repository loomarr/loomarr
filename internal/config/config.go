// Package config loads Loomarr's ENV-ONLY BOOTSTRAP configuration (config-design
// §1): the handful of keys needed before the database opens or that describe
// process topology. Everything else is an app-managed *setting* (the typed
// registry in internal/settings), resolved env > database > default at runtime —
// NOT here. The classification rule (config-design §1): a key is env-only iff it
// is needed before the DB opens or describes process topology. Adding an
// app-managed knob here instead of the registry is the drift CLAUDE.md forbids.
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config is the env-only bootstrap surface (config-design §1). These keys are read
// once in main before the store opens; the settings registry cannot resolve them
// (it needs the DB) and they never appear in the Settings UI. Everything the app
// manages — connections, AI, channels, filler, timings, secrets — lives in the
// settings registry, not here.
type Config struct {
	// Core / harness — process topology, needed before anything else.
	ListenAddr string `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel   string `env:"LOG_LEVEL" envDefault:"info"`
	TZ         string `env:"TZ"`

	// Store — needed to OPEN the database, so necessarily pre-registry.
	// Defaults to the SQLite volume path so `docker run -v loomarr-data:/data
	// loomarr` boots into the wizard with zero required env (§15). Without the
	// default the promise held only via compose, which sets the same value.
	DatabaseURL string `env:"DATABASE_URL" envDefault:"sqlite:///data/loomarr.db"`
	AutoMigrate bool   `env:"AUTO_MIGRATE" envDefault:"true"`
}

// Load reads the bootstrap environment into a Config, applying §1 defaults. Only
// the process cannot start without these; every other knob is validated by the
// settings service at boot (an invalid env pin fails there, config-design §3).
func Load() (*Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &c, nil
}
