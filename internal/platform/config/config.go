// Package config loads Loomarr's ENV-ONLY BOOTSTRAP configuration (config-design
// §1): the handful of keys needed before the database opens or that describe
// process topology. Everything else is an app-managed *setting* (the typed
// registry in internal/platform/settings), resolved env > database > default at runtime —
// NOT here. The classification rule (config-design §1): a key is env-only iff it
// is needed before the DB opens or describes process topology. Adding an
// app-managed knob here instead of the registry is the drift AGENTS.md forbids.
package config

import (
	"fmt"
	"net/url"
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

	// AgentDisableLiveTVTuner is the env-only flag scripts/dev-env.sh exports for an agent lane
	// (#1555): any non-blank value stops the lane adding or retiring a Live TV tuner on whatever
	// media server its library.* settings point at. Deliberately NOT a registry setting, so
	// nothing the Settings UI saves can turn it off. Read it through TunerPublishingDisabled.
	AgentDisableLiveTVTuner string `env:"LOOMARR_AGENT_DISABLE_LIVETV_TUNER"`

	// DataDir is the directory holding the bootstrap file, the generated installation key and
	// other boot-owned state. It is decided once (the SQLite database's directory, else /data)
	// and recorded in the bootstrap file by PersistDataDir; every later boot reads it back, so
	// a database cutover cannot move it. It has no env var of its own.
	DataDir string `env:"-"`

	// bootstrapDir is where the bootstrap file was found ("" when there was none), and
	// dataDirRecorded reports whether that file already carried DATA_DIR.
	bootstrapDir    string
	dataDirRecorded bool

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

	// Pprof mounts Go's /debug/pprof/* handlers — CPU, heap, goroutine and mutex
	// profiles (§7). Same reasoning and same tier as DevLogin: it is UNAUTHENTICATED
	// by nature (a profiler is not a browser and holds no session), it exposes stack
	// traces and memory contents, and a goroutine or CPU profile can be triggered
	// repeatedly to degrade a running server. So it is boot-time, default off, and
	// deliberately NOT a settings-registry key — a profiling surface that an admin
	// session could switch on at runtime is a worse hole than the one it opens.
	Pprof bool `env:"LOOMARR_PPROF" envDefault:"false"`

	// MetricsToken is the bearer credential Prometheus presents to GET /v1/metrics and /metrics
	// (§7, #1408). LOOMARR_METRICS_TOKEN or LOOMARR_METRICS_TOKEN_FILE (Docker-secrets idiom, both
	// set is an error) — resolved in Load. Empty ⇒ the endpoint refuses every request.
	//
	// Bootstrap tier, not the settings registry, for the LOOMARR_PPROF reason: it gates an
	// unauthenticated-by-nature surface, and a registry key is editable by any admin session.
	// It is deliberately NOT the API_TOKEN, so a scraper credential cannot act as an admin.
	MetricsToken string `env:"LOOMARR_METRICS_TOKEN"`

	// DemoUsername and DemoPassword turn on demo mode (#2268): the sign-in screen offers this
	// account to anyone, and the signed-in app shows the demo tour. Both deliberately public, so
	// only the demo site sets them, and only for an account it created with that password. Set
	// both or neither; Load refuses one alone.
	DemoUsername string `env:"LOOMARR_DEMO_USERNAME"`
	DemoPassword string `env:"LOOMARR_DEMO_PASSWORD"`

	// TMDBBaseURL points the TMDB client at a stand-in instead of the public API (#1941): the
	// demo library serves one over its invented catalogue, so a lane or demo backend can ground
	// suggestions offline without surfacing real titles. Bootstrap tier because the client's base
	// URL is fixed for its lifetime, and because redirecting every grounding lookup is not a
	// knob an admin session should be able to flip. Empty ⇒ https://api.themoviedb.org/3.
	TMDBBaseURL string `env:"LOOMARR_TMDB_BASE_URL"`
}

// validateTMDBBaseURL refuses a base URL every TMDB request would fail against, at boot rather
// than on the first suggestion.
func validateTMDBBaseURL(c *Config) error {
	c.TMDBBaseURL = strings.TrimRight(strings.TrimSpace(c.TMDBBaseURL), "/")
	if c.TMDBBaseURL == "" {
		return nil
	}
	u, err := url.Parse(c.TMDBBaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("LOOMARR_TMDB_BASE_URL must be an absolute http(s) URL, got %q", c.TMDBBaseURL)
	}
	return nil
}

// DemoMode reports whether this server is a demo site (#2268).
func (c *Config) DemoMode() bool { return c.DemoUsername != "" }

// validateDemo refuses half a demo account: a sign-in button with no password would offer
// visitors a login that cannot succeed.
func validateDemo(c *Config) error {
	c.DemoUsername, c.DemoPassword = strings.TrimSpace(c.DemoUsername), strings.TrimSpace(c.DemoPassword)
	if (c.DemoUsername == "") != (c.DemoPassword == "") {
		return fmt.Errorf("LOOMARR_DEMO_USERNAME and LOOMARR_DEMO_PASSWORD must be set together")
	}
	return nil
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
	return LoadEnv(processEnvironment())
}

// LoadEnv is Load over an explicit environment map. The process environment is neither read
// nor written, so precedence is decided here and tests need no os.Setenv to steer it.
func LoadEnv(environ map[string]string) (*Config, error) {
	// Resolve the data directory the file lives in from the env-level DATABASE_URL
	// first: if an operator pinned a SQLite path, that is where the file belongs. This
	// is deliberately a chicken-and-egg the ENV side resolves — the file can relocate
	// the database, but never itself.
	var envOnly Config
	if err := env.ParseWithOptions(&envOnly, env.Options{Environment: environ}); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	fileValues, fileDir, err := loadBootstrapSearch(BootstrapDirsFor(envOnly.DatabaseURL))
	if err != nil {
		// Loud, not silent: this runs before the store opens, so a bad file must stop
		// the boot rather than let the process start with configuration the operator
		// believes is applied.
		return nil, err
	}

	// The file is DEFAULTS beneath the real environment: merge it into a copy of the map
	// for every key the environment does NOT already define, then parse once. That keeps
	// precedence in one obvious place instead of spread across per-field merges.
	merged := make(map[string]string, len(environ)+len(fileValues))
	for k, v := range fileValues {
		merged[k] = v
	}
	for k, v := range environ {
		merged[k] = v // env wins — the file never overrides a pin, even an empty one
	}

	var c Config
	if err := env.ParseWithOptions(&c, env.Options{Environment: merged}); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := resolveMetricsTokenFile(&c, environ); err != nil {
		return nil, err
	}
	if err := validateDemo(&c); err != nil {
		return nil, err
	}
	if err := validateTMDBBaseURL(&c); err != nil {
		return nil, err
	}
	c.bootstrapDir = fileDir
	if recorded := fileValues[dataDirKey]; recorded != "" {
		c.DataDir, c.dataDirRecorded = recorded, true
	} else {
		c.DataDir = DataDirFor(c.DatabaseURL)
	}
	return &c, nil
}

// TunerPublishingDisabled reports whether the agent-lane flag is set (any non-blank value).
func (c *Config) TunerPublishingDisabled() bool {
	return strings.TrimSpace(c.AgentDisableLiveTVTuner) != ""
}

// processEnvironment is the process environment as a map, the one place Load reads it.
func processEnvironment() map[string]string {
	environ := os.Environ()
	m := make(map[string]string, len(environ))
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			m[k] = v
		}
	}
	return m
}

// PersistDataDir records the decided DataDir in the bootstrap file so later boots read it
// back instead of re-deriving it. It is a no-op once recorded. The value is written into the
// file Load found, or a new one in DataDir itself on a first boot, preserving every other value.
func (c *Config) PersistDataDir() error {
	if c.dataDirRecorded || c.DataDir == "" {
		return nil
	}
	dir := c.bootstrapDir
	if dir == "" {
		dir = c.DataDir
	}
	if err := UpdateBootstrapFile(dir, map[string]string{dataDirKey: c.DataDir}); err != nil {
		return fmt.Errorf("record data directory: %w", err)
	}
	c.dataDirRecorded = true
	return nil
}

// resolveMetricsTokenFile applies the <VAR>_FILE half of the secret idiom (config-design §3).
// A non-empty <VAR> together with <VAR>_FILE is ambiguous and refused; an unreadable file stops
// the boot rather than leaving the operator believing a token is set. Errors name the variable
// and path, never the value.
func resolveMetricsTokenFile(c *Config, environ map[string]string) error {
	c.MetricsToken = strings.TrimSpace(c.MetricsToken) // a blank/whitespace pin means "unset", never a token
	path := environ["LOOMARR_METRICS_TOKEN_FILE"]
	if path == "" {
		return nil
	}
	if c.MetricsToken != "" {
		return fmt.Errorf("LOOMARR_METRICS_TOKEN and LOOMARR_METRICS_TOKEN_FILE are both set (ambiguous)")
	}
	token, err := ReadSecret(path)
	if err != nil {
		return fmt.Errorf("LOOMARR_METRICS_TOKEN_FILE: %w", err)
	}
	c.MetricsToken = token
	return nil
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
