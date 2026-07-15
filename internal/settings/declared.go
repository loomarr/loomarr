package settings

// declared is the canonical registry content: every app-managed setting, in the
// order it appears in design.md §15. This list IS the contract — design.md §15
// is its human mirror and `make config-docs` its generated reference. A key added
// here without a matching §15 row (or vice versa) is the drift CLAUDE.md forbids.
//
// Env-only bootstrap keys (DATABASE_URL, AUTO_MIGRATE, LISTEN_ADDR, LOG_LEVEL, TZ)
// are NOT here — they stay in config.Config (config-design §1 classification).
// Generated secrets (SESSION_SECRET, API_TOKEN, WEBHOOK_SECRET) live in secrets.go
// (minted, not demanded — §4), not the app-managed registry.
func declared() []Setting {
	return []Setting{
		// --- Connections: media server (§15, Phase 5) ---
		{
			Key: "library.flavor", EnvVar: "LIBRARY_FLAVOR", Group: GroupMediaServer,
			Kind: KindEnum, Enum: []string{"emby", "jellyfin"}, Default: "",
			Doc: "Media server flavor. Selects the auth-header shape (Emby X-Emby-Token vs Jellyfin MediaBrowser).",
		},
		{
			Key: "library.url", EnvVar: "LIBRARY_URL", Group: GroupMediaServer,
			Kind: KindURL, Default: "",
			Doc: "Media server base URL, e.g. http://emby:8096.",
		},
		{
			Key: "library.token", EnvVar: "LIBRARY_TOKEN", Group: GroupMediaServer,
			Kind: KindSecret, Default: "",
			Doc: "Media server API token (Emby/Jellyfin). Grants read + Live-TV wiring.",
		},
		{
			Key: "season.precision", EnvVar: "SEASON_PRECISION", Group: GroupMediaServer,
			Kind: KindEnum, Enum: []string{"series", "seasons"}, Default: "series", Advanced: true,
			Doc: "Acquisition granularity for series: whole series (default) or per-season.",
		},

		// --- Connections: requester (§15, Phase 6) ---
		{
			Key: "seerr.url", EnvVar: "SEERR_URL", Group: GroupRequester,
			Kind: KindURL, Default: "", Required: FeatureAcquisition,
			Doc: "Seerr base URL, e.g. http://seerr:5055. Enables acquisitions (or use the direct Sonarr/Radarr pair).",
		},
		{
			Key: "seerr.api_key", EnvVar: "SEERR_API_KEY", Group: GroupRequester,
			Kind: KindSecret, Default: "",
			Doc: "Seerr API key (X-Api-Key header).",
		},
		{
			Key: "sonarr.url", EnvVar: "SONARR_URL", Group: GroupRequester,
			Kind: KindURL, Default: "", Advanced: true,
			Doc: "Direct Sonarr base URL (alternative to Seerr for TV acquisitions).",
		},
		{
			Key: "sonarr.api_key", EnvVar: "SONARR_API_KEY", Group: GroupRequester,
			Kind: KindSecret, Default: "", Advanced: true,
			Doc: "Direct Sonarr API key.",
		},
		{
			Key: "radarr.url", EnvVar: "RADARR_URL", Group: GroupRequester,
			Kind: KindURL, Default: "", Advanced: true,
			Doc: "Direct Radarr base URL (alternative to Seerr for movie acquisitions).",
		},
		{
			Key: "radarr.api_key", EnvVar: "RADARR_API_KEY", Group: GroupRequester,
			Kind: KindSecret, Default: "", Advanced: true,
			Doc: "Direct Radarr API key.",
		},

		// --- Connections: Tunarr (§15, Phase 10) ---
		{
			Key: "tunarr.url", EnvVar: "TUNARR_URL", Group: GroupTunarr,
			Kind: KindURL, Default: "",
			Doc: "Tunarr base URL, e.g. http://tunarr:8000. Where channels are pushed.",
		},
		{
			Key: "tunarr.api_key", EnvVar: "TUNARR_API_KEY", Group: GroupTunarr,
			Kind: KindSecret, Default: "",
			Doc: "Tunarr API key (optional on 1.3.8 — Phase-0 finding).",
		},
		{
			Key: "tunarr.transcode_config_id", EnvVar: "TUNARR_TRANSCODE_CONFIG_ID", Group: GroupTunarr,
			Kind: KindString, Default: "", Advanced: true,
			Doc: "Tunarr transcode-config uuid created channels reference (empty → resolve the instance Default, §9).",
		},

		// --- Connections: TMDB (§15, Phase 11) ---
		{
			Key: "tmdb.api_key", EnvVar: "TMDB_API_KEY", Group: GroupTMDB,
			Kind: KindSecret, Default: "", Required: FeatureSuggestions,
			Doc: "TMDB API key. Grounds suggestions — required when the suggester is enabled.",
		},

		// --- AI (§15, §8.1; in-app selection persists to llm.* and overrides these env pins) ---
		{
			Key: "llm.provider", EnvVar: "LLM_PROVIDER", Group: GroupAI,
			Kind: KindEnum, Enum: []string{"ollama", "openai"}, Default: "ollama", Required: FeatureSuggestions,
			Doc: "LLM provider. Selects the client (ollama vs OpenAI-compatible). An in-app pick (§8.1) overrides this.",
		},
		{
			Key: "llm.url", EnvVar: "LLM_URL", Group: GroupAI,
			Kind: KindURL, Default: "",
			Doc: "LLM base URL. For openai, the OpenAI-compatible base (…/v1); for ollama, its own host.",
		},
		{
			Key: "llm.model", EnvVar: "LLM_MODEL", Group: GroupAI,
			Kind: KindString, Default: "",
			Doc: "LLM model id (e.g. qwen3:8b). An in-app pick (§8.1) overrides this.",
		},
		{
			Key: "llm.api_key", EnvVar: "LLM_API_KEY", Group: GroupAI,
			Kind: KindSecret, Default: "",
			Doc: "LLM API key (read when provider=openai). In-app hosted picks store per-provider keys; never echoed by any API.",
		},
		{
			Key: "suggest.auto_approve", EnvVar: "SUGGEST_AUTO_APPROVE", Group: GroupAI,
			Kind: KindBool, Default: false, Advanced: true,
			Doc: "Auto-approve suggested acquisitions (bypasses the human approval gate — off by default, §8).",
		},
		{
			Key: "suggest.max_acquisitions", EnvVar: "SUGGEST_MAX_ACQUISITIONS", Group: GroupAI,
			Kind: KindInt, Default: 10,
			Doc: "Cap on acquisitions per suggestion (the §8 quota).",
		},

		// --- Channels & playback (§15, Phase 10; policy defaults = programming-design §2) ---
		{
			Key: "sched.default_strategy", EnvVar: "SCHED_DEFAULT_STRATEGY", Group: GroupChannels,
			Kind: KindEnum, Enum: []string{"sequential", "shuffle"}, Default: "shuffle",
			Doc: "Default per-channel scheduling strategy when a channel doesn't specify one.",
		},
		{
			Key: "sched.backfill", EnvVar: "SCHED_BACKFILL", Group: GroupChannels,
			Kind: KindEnum, Enum: []string{"stable", "reshuffle"}, Default: "stable",
			Doc: "How the sweep places newly-available acquisitions: stable (in place) or reshuffle.",
		},
		{
			Key: "channel.reconcile_every", EnvVar: "CHANNEL_RECONCILE_EVERY", Group: GroupChannels,
			Kind: KindDuration, Default: "10m",
			Doc: "Periodic channel reconcile sweep interval (§9).",
		},
		// Policy defaults (the middle tier of channel policy > registry default > built-in,
		// programming-design §2/§9). These close the ChannelPolicy registry-default deferral.
		{
			Key: "sched.episode_norepeat", EnvVar: "SCHED_EPISODE_NOREPEAT", Group: GroupChannels,
			Kind: KindDuration, Default: "168h",
			Doc: "Default no-repeat window for a series' episodes (per-channel overridable).",
		},
		{
			Key: "sched.movie_norepeat", EnvVar: "SCHED_MOVIE_NOREPEAT", Group: GroupChannels,
			Kind: KindDuration, Default: "720h",
			Doc: "Default no-repeat window for movies (per-channel overridable).",
		},
		{
			Key: "sched.series_min_gap", EnvVar: "SCHED_SERIES_MIN_GAP", Group: GroupChannels,
			Kind: KindDuration, Default: "2h",
			Doc: "Default minimum gap between two episodes of the same series (per-channel overridable).",
		},
		{
			Key: "sched.block_max", EnvVar: "SCHED_BLOCK_MAX", Group: GroupChannels,
			Kind: KindInt, Default: 2,
			Doc: "Default max consecutive programs from one series before another must air (per-channel overridable).",
		},
		{
			Key: "sched.ordering", EnvVar: "SCHED_ORDERING", Group: GroupChannels,
			Kind: KindEnum, Enum: []string{"sequential", "shuffle", "syndication"}, Default: "syndication",
			Doc: "Default policy ordering mode (per-channel overridable). Omitted per channel → the channel's Strategy.",
		},
		{
			Key: "seasonal.mode", EnvVar: "SEASONAL_MODE", Group: GroupChannels,
			Kind: KindEnum, Enum: []string{"off", "auto", "exclusive"}, Default: "auto",
			Doc: "Default seasonal handling (per-channel overridable): off, auto (boost/bench), or exclusive.",
		},

		// --- Filler / commercials (§15, Phase 12; §10 redesign — Tunarr-owned) ---
		{
			Key: "filler.dir", EnvVar: "FILLER_DIR", Group: GroupFiller,
			Kind: KindString, Default: "", Required: FeatureFiller,
			Doc: "Drop-folder Loomarr registers as a Tunarr 'local' source for commercials/bumpers.",
		},
		{
			Key: "filler.sync_every", EnvVar: "FILLER_SYNC_EVERY", Group: GroupFiller,
			Kind: KindDuration, Default: "15m",
			Doc: "How often to re-sync the filler catalog from Tunarr's local source.",
		},
		{
			Key: "filler.ai_tagging", EnvVar: "FILLER_AI_TAGGING", Group: GroupFiller,
			Kind: KindBool, Default: false,
			Doc: "Enable AI tagging of untagged commercials (era/audience/category).",
		},
		{
			Key: "filler.breaks_per_hour", EnvVar: "FILLER_BREAKS_PER_HOUR", Group: GroupFiller,
			Kind: KindInt, Default: 4,
			Doc: "Commercial-break density: breaks interleaved per program hour.",
		},
		{
			Key: "filler.pod_max", EnvVar: "FILLER_POD_MAX", Group: GroupFiller,
			Kind: KindInt, Default: 4,
			Doc: "Maximum clips per commercial pod.",
		},
		{
			Key: "filler.cooldown_seconds", EnvVar: "FILLER_COOLDOWN_SECONDS", Group: GroupFiller,
			Kind: KindInt, Default: 30, Advanced: true,
			Doc: "Min seconds before a filler clip may repeat (Tunarr filler-list attach).",
		},
		{
			Key: "filler.weight", EnvVar: "FILLER_WEIGHT", Group: GroupFiller,
			Kind: KindInt, Default: 1, Advanced: true,
			Doc: "Relative draw weight across multiple filler-lists.",
		},

		// --- Users & security (§15, Phase 9) ---
		{
			Key: "session.ttl", EnvVar: "SESSION_TTL", Group: GroupUsersSecurity,
			Kind: KindDuration, Default: "720h",
			Doc: "Session lifetime before re-login (§11).",
		},
		{
			Key: "cookie.secure", EnvVar: "COOKIE_SECURE", Group: GroupUsersSecurity,
			Kind: KindEnum, Enum: []string{"auto", "always", "never"}, Default: "auto",
			Doc: "Secure-cookie policy: auto (infer from request), always, or never (dev only).",
		},
		{
			Key: "user.sync_every", EnvVar: "USER_SYNC_EVERY", Group: GroupUsersSecurity,
			Kind: KindDuration, Default: "1h", Advanced: true,
			Doc: "How often to sync users from the media server (§11).",
		},

		// --- Advanced: TTLs, retention, workers, event webhook (§15) ---
		{
			Key: "request.ttl", EnvVar: "REQUEST_TTL", Group: GroupAdvanced,
			Kind: KindDuration, Default: "48h",
			Doc: "How long a 'wanted' title is retried before give-up (§4).",
		},
		{
			Key: "downloading.ttl", EnvVar: "DOWNLOADING_TTL", Group: GroupAdvanced,
			Kind: KindDuration, Default: "12h",
			Doc: "How long a 'downloading' title waits for the import webhook before give-up (§4).",
		},
		{
			Key: "reconcile.every", EnvVar: "RECONCILE_EVERY", Group: GroupAdvanced,
			Kind: KindDuration, Default: "5m",
			Doc: "Provisioning reconcile tick interval (§7).",
		},
		{
			Key: "job.workers", EnvVar: "JOB_WORKERS", Group: GroupAdvanced,
			Kind: KindInt, Default: 2,
			Doc: "Suggester worker-pool size (§8).",
		},
		{
			Key: "job.timeout", EnvVar: "JOB_TIMEOUT", Group: GroupAdvanced,
			Kind: KindDuration, Default: "10m",
			Doc: "Per-suggestion-job timeout (§8).",
		},
		{
			Key: "jobs.retention", EnvVar: "JOBS_RETENTION", Group: GroupAdvanced,
			Kind: KindDuration, Default: "720h",
			Doc: "How long completed jobs are kept before the janitor prunes them (§5).",
		},
		{
			Key: "proposals.retention", EnvVar: "PROPOSALS_RETENTION", Group: GroupAdvanced,
			Kind: KindDuration, Default: "2160h",
			Doc: "How long proposals are kept before pruning (§5).",
		},
		{
			Key: "event.webhook_url", EnvVar: "EVENT_WEBHOOK_URL", Group: GroupAdvanced,
			Kind: KindURL, Default: "",
			Doc: "Optional external event target (fired on terminal title transitions).",
		},
	}
}
