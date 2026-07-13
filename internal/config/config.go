// Package config loads Loomarr's 12-factor configuration (design §15) from the
// environment. Config is parsed once in main and passed explicitly to subsystems;
// nothing here reads os.Getenv at call time, which keeps subsystems testable.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config is the full §15 environment surface. Fields are grouped by subsystem.
// Only the always-required knobs are validated at startup (see Load); the rest
// are validated by the subsystem that consumes them, so a single missing/broken
// dependency degrades one feature rather than refusing to boot (§6).
type Config struct {
	// Core / harness (Phase 1)
	ListenAddr string `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel   string `env:"LOG_LEVEL" envDefault:"info"`
	TZ         string `env:"TZ"`

	// Store (Phase 3/4)
	DatabaseURL string `env:"DATABASE_URL"`
	AutoMigrate bool   `env:"AUTO_MIGRATE" envDefault:"true"`

	// Library — Emby/Jellyfin (Phase 5)
	LibraryFlavor   string `env:"LIBRARY_FLAVOR"`
	LibraryURL      string `env:"LIBRARY_URL"`
	LibraryToken    string `env:"LIBRARY_TOKEN"`
	SeasonPrecision string `env:"SEASON_PRECISION" envDefault:"series"`

	// Requester — Seerr (or direct arr) (Phase 6)
	SeerrURL    string `env:"SEERR_URL"`
	SeerrAPIKey string `env:"SEERR_API_KEY"`
	SonarrURL   string `env:"SONARR_URL"`
	SonarrKey   string `env:"SONARR_API_KEY"`
	RadarrURL   string `env:"RADARR_URL"`
	RadarrKey   string `env:"RADARR_API_KEY"`

	// Ingest webhooks (Phase 6)
	WebhookSecret   string `env:"WEBHOOK_SECRET"`
	EventWebhookURL string `env:"EVENT_WEBHOOK_URL"`

	// Programmer — Tunarr (Phase 10)
	TunarrURL         string `env:"TUNARR_URL"`
	TunarrAPIKey      string `env:"TUNARR_API_KEY"`
	TunarrTranscodeID string `env:"TUNARR_TRANSCODE_CONFIG_ID"` // channel-create transcodeConfigId (§9, §15)

	// Provisioning / scheduling timing (Phases 7, 10)
	RequestTTL            time.Duration `env:"REQUEST_TTL" envDefault:"48h"`
	DownloadingTTL        time.Duration `env:"DOWNLOADING_TTL" envDefault:"12h"`
	ReconcileEvery        time.Duration `env:"RECONCILE_EVERY" envDefault:"5m"`
	ChannelReconcileEvery time.Duration `env:"CHANNEL_RECONCILE_EVERY" envDefault:"10m"`
	SchedDefaultStrategy  string        `env:"SCHED_DEFAULT_STRATEGY" envDefault:"shuffle"`
	SchedBackfill         string        `env:"SCHED_BACKFILL" envDefault:"stable"`

	// Users, auth, sessions (Phase 9)
	SessionSecret string        `env:"SESSION_SECRET"`
	SessionTTL    time.Duration `env:"SESSION_TTL" envDefault:"720h"`
	CookieSecure  string        `env:"COOKIE_SECURE" envDefault:"auto"`
	APIToken      string        `env:"API_TOKEN"`
	UserSyncEvery time.Duration `env:"USER_SYNC_EVERY" envDefault:"1h"`

	// Jobs / suggester (Phase 11)
	JobWorkers         int           `env:"JOB_WORKERS" envDefault:"2"`
	JobTimeout         time.Duration `env:"JOB_TIMEOUT" envDefault:"10m"`
	JobsRetention      time.Duration `env:"JOBS_RETENTION" envDefault:"720h"`
	ProposalsRetention time.Duration `env:"PROPOSALS_RETENTION" envDefault:"2160h"`
	LLMProvider        string        `env:"LLM_PROVIDER" envDefault:"ollama"`
	LLMURL             string        `env:"LLM_URL"`
	LLMModel           string        `env:"LLM_MODEL"`
	LLMAPIKey          string        `env:"LLM_API_KEY"`
	TMDBAPIKey         string        `env:"TMDB_API_KEY"`
	SuggestAutoApprove bool          `env:"SUGGEST_AUTO_APPROVE" envDefault:"false"`
	SuggestMaxAcquire  int           `env:"SUGGEST_MAX_ACQUISITIONS" envDefault:"10"`

	// Filler / commercials (Phase 12)
	FillerLibrary     string        `env:"FILLER_LIBRARY"`
	FillerSyncEvery   time.Duration `env:"FILLER_SYNC_EVERY" envDefault:"15m"`
	FillerAITagging   bool          `env:"FILLER_AI_TAGGING" envDefault:"false"`
	FillerBreaksPerHr int           `env:"FILLER_BREAKS_PER_HOUR" envDefault:"4"`
	FillerPodMax      int           `env:"FILLER_POD_MAX" envDefault:"4"`
}

// Load reads the environment into a Config, applying §15 defaults. It validates
// only the knobs the process cannot start without; subsystem-specific config is
// checked lazily by each subsystem (§6 degrade-don't-wedge).
func Load() (*Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &c, nil
}
