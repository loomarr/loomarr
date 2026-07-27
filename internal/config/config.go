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
	"os"
	"path/filepath"
	"strings"

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

	// DevLogin mounts POST /v1/auth/dev-login — a credential-free admin sign-in for
	// development (§11). It lives HERE, in the bootstrap tier, rather than in the
	// settings registry, and that placement is the security property: a registry key
	// is editable through the settings API by anyone who is already an admin, which
	// would let a compromised session install a permanent credential-free door. An
	// env var read once at boot cannot be switched on by a running process.
	// Default false — a shipped install has no such route at all.
	DevLogin bool `env:"LOOMARR_DEV_LOGIN" envDefault:"false"`
}

// Load reads the bootstrap configuration, resolving `env > file > default`
// (config-design §1). Only the process cannot start without these; every other knob is
// validated by the settings service at boot (an invalid env pin fails there, §3).
//
// The FILE tier exists for one reason: the wizard's Database step must persist which
// database to use, and it cannot write that answer into the database it is choosing.
// See bootstrapfile.go. Env still wins, so a GitOps pin is never overridden by
// something the wizard wrote.
func Load() (*Config, error) {
	// Resolve the data directory the file lives in from the env-level DATABASE_URL
	// first: if an operator pinned a SQLite path, that is where the file belongs. This
	// is deliberately a chicken-and-egg the ENV side resolves — the file can relocate
	// the database, but never itself.
	var envOnly Config
	if err := env.Parse(&envOnly); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	fileValues, err := loadBootstrapSearch(BootstrapDirsFor(envOnly.DatabaseURL))
	if err != nil {
		// Loud, not silent: this runs before the store opens, so a bad file must stop
		// the boot rather than let the process start with configuration the operator
		// believes is applied.
		return nil, err
	}

	// Apply the file as DEFAULTS beneath the real environment: env.ParseWithOptions'
	// DefaultValueTagName isn't a lookup hook, so the tier is expressed by setting only
	// the vars the environment does NOT already define, then parsing once. That keeps
	// precedence in one obvious place instead of spread across per-field merges.
	restore := map[string]bool{}
	for k, v := range fileValues {
		if _, pinned := os.LookupEnv(k); pinned {
			continue // env wins — the file never overrides a pin
		}
		if err := os.Setenv(k, v); err != nil {
			return nil, fmt.Errorf("apply %s from %s: %w", k, BootstrapFileName, err)
		}
		restore[k] = true
	}
	defer func() {
		// Leave the process environment as we found it: Load is called once in main,
		// but tests call it repeatedly and a leaked var would make them order-dependent.
		for k := range restore {
			_ = os.Unsetenv(k)
		}
	}()

	var c Config
	if err := env.Parse(&c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &c, nil
}

// DataDirFor returns the directory a SQLite database lives in — where the bootstrap
// file is kept. For a non-SQLite URL it returns the conventional /data, so a Postgres
// install still has one well-known place for the file.
func DataDirFor(databaseURL string) string {
	const sqlitePrefix = "sqlite://"
	if strings.HasPrefix(databaseURL, sqlitePrefix) {
		if p := strings.TrimPrefix(databaseURL, sqlitePrefix); p != "" {
			return filepath.Dir(p)
		}
	}
	return "/data"
}
