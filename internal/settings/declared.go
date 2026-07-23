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
			Kind: KindEnum, Enum: []EnumOption{opt("emby", "Emby"), opt("jellyfin", "Jellyfin")}, Default: "",
			Doc: "Emby or Jellyfin. They sign in differently, so Loomarr needs to know which one you run.",
		},
		{
			Key: "library.url", EnvVar: "LIBRARY_URL", Group: GroupMediaServer,
			Kind: KindURL, Default: "",
			Doc: "Media server base URL, e.g. http://emby:8096.",
		},
		{
			Key: "library.token", EnvVar: "LIBRARY_TOKEN", Group: GroupMediaServer,
			Kind: KindSecret, Default: "",
			Doc: "An API key from your media server. Lets Loomarr read your library and set up the TV guide.",
		},
		{
			Key: "season.precision", EnvVar: "SEASON_PRECISION", Group: GroupMediaServer,
			Kind: KindEnum, Enum: []EnumOption{opt("series", "Whole series"), opt("seasons", "Requested seasons")}, Default: "series", Advanced: true,
			Doc: "When adding a series, get the whole show (default) or just the seasons you asked for.",
		},

		// --- Connections: requester (§15, Phase 6) ---
		// How Loomarr acquires missing titles: through Seerr (default), or Sonarr + Radarr
		// directly. The provider gates which fields show (ShowWhen), mirroring llm.provider.
		{
			Key: "requester.provider", EnvVar: "REQUESTER_PROVIDER", Group: GroupRequester,
			Kind: KindEnum, Enum: []EnumOption{opt("seerr", "Seerr / Jellyseerr"), opt("arr", "Sonarr + Radarr (direct)")}, Default: "seerr",
			Doc: "How Loomarr downloads missing titles: through Seerr, or Sonarr and Radarr directly.",
		},
		{
			Key: "seerr.url", EnvVar: "SEERR_URL", Group: GroupRequester,
			Kind: KindURL, Default: "", Required: FeatureAcquisition,
			Doc:      "Your Seerr address, e.g. http://seerr:5055. This is how Loomarr downloads missing titles.",
			ShowWhen: map[string][]string{"requester.provider": {"seerr"}},
		},
		{
			Key: "seerr.api_key", EnvVar: "SEERR_API_KEY", Group: GroupRequester,
			Kind: KindSecret, Default: "",
			Doc:      "Your Seerr API key.",
			ShowWhen: map[string][]string{"requester.provider": {"seerr"}},
		},
		{
			Key: "sonarr.url", EnvVar: "SONARR_URL", Group: GroupRequester,
			Kind: KindURL, Default: "",
			Doc:      "Sonarr address (for TV), e.g. http://sonarr:8989.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "sonarr.api_key", EnvVar: "SONARR_API_KEY", Group: GroupRequester,
			Kind: KindSecret, Default: "",
			Doc:      "Your Sonarr API key (Settings → General in Sonarr).",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "sonarr.quality_profile", EnvVar: "SONARR_QUALITY_PROFILE", Group: GroupRequester,
			Kind: KindString, Default: "", Advanced: true,
			Doc:      "Optional Sonarr quality profile (name or id). Blank = Sonarr's first profile.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "sonarr.root_folder", EnvVar: "SONARR_ROOT_FOLDER", Group: GroupRequester,
			Kind: KindString, Default: "", Advanced: true,
			Doc:      "Optional Sonarr root folder path. Blank = Sonarr's first root folder.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "radarr.url", EnvVar: "RADARR_URL", Group: GroupRequester,
			Kind: KindURL, Default: "",
			Doc:      "Radarr address (for movies), e.g. http://radarr:7878.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "radarr.api_key", EnvVar: "RADARR_API_KEY", Group: GroupRequester,
			Kind: KindSecret, Default: "",
			Doc:      "Your Radarr API key (Settings → General in Radarr).",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "radarr.quality_profile", EnvVar: "RADARR_QUALITY_PROFILE", Group: GroupRequester,
			Kind: KindString, Default: "", Advanced: true,
			Doc:      "Optional Radarr quality profile (name or id). Blank = Radarr's first profile.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "radarr.root_folder", EnvVar: "RADARR_ROOT_FOLDER", Group: GroupRequester,
			Kind: KindString, Default: "", Advanced: true,
			Doc:      "Optional Radarr root folder path. Blank = Radarr's first root folder.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},

		// --- Connections: Tunarr (§15, Phase 10) ---
		{
			Key: "tunarr.url", EnvVar: "TUNARR_URL", Group: GroupTunarr,
			Kind: KindURL, Default: "",
			Doc: "Your Tunarr address, e.g. http://tunarr:8000. This is where Loomarr builds your channels.",
		},
		{
			Key: "tunarr.transcode_config_id", EnvVar: "TUNARR_TRANSCODE_CONFIG_ID", Group: GroupTunarr,
			Kind: KindString, Default: "", Advanced: true,
			Doc: "Which Tunarr transcode profile new channels use. Leave empty to use Tunarr's default.",
		},

		// --- Connections: TMDB (§15, Phase 11) ---
		{
			Key: "tmdb.api_key", EnvVar: "TMDB_API_KEY", Group: GroupTMDB,
			Kind: KindSecret, Default: "", Required: FeatureSuggestions,
			Doc: "A free TMDB API key. Needed for AI channel suggestions.",
		},

		// --- AI (§15, §8.1; in-app selection persists to llm.* and overrides these env pins) ---
		{
			Key: "llm.provider", EnvVar: "LLM_PROVIDER", Group: GroupAI,
			Kind: KindEnum, Enum: []EnumOption{opt("ollama", "Ollama"), opt("openai", "OpenAI-compatible")}, Default: "ollama", Required: FeatureSuggestions,
			Doc: "Which AI to use: a local Ollama, or an OpenAI-compatible service. You can also pick a model in the AI settings.",
		},
		{
			// A hosted (OpenAI-compatible) service needs its base URL; a local Ollama is
			// reached at its own host, chosen in the model picker — so this is hidden for Ollama.
			Key: "llm.url", EnvVar: "LLM_URL", Group: GroupAI,
			Kind: KindURL, Default: "",
			Doc:      "The base URL of your OpenAI-compatible service, ending in /v1.",
			ShowWhen: map[string][]string{"llm.provider": {"openai"}},
		},
		{
			// For a hosted service you type the model name; for Ollama the ranked model
			// picker below is how you choose, so this free-text field is hidden there to
			// avoid two controls setting the same thing.
			Key: "llm.model", EnvVar: "LLM_MODEL", Group: GroupAI,
			Kind: KindString, Default: "",
			Doc:      "The model name for your hosted AI service (e.g. gpt-4o-mini).",
			ShowWhen: map[string][]string{"llm.provider": {"openai"}},
		},
		{
			// Ollama is local and needs no key — this only applies to a hosted service.
			Key: "llm.api_key", EnvVar: "LLM_API_KEY", Group: GroupAI,
			Kind: KindSecret, Default: "",
			Doc:      "API key for your hosted AI service. Never shown again after saving.",
			ShowWhen: map[string][]string{"llm.provider": {"openai"}},
		},
		{
			Key: "suggest.auto_approve", EnvVar: "SUGGEST_AUTO_APPROVE", Group: GroupAI,
			Kind: KindBool, Default: false, Advanced: true,
			Doc: "Automatically approve suggested downloads, with no review step. Off by default.",
		},
		{
			Key: "suggest.max_acquisitions", EnvVar: "SUGGEST_MAX_ACQUISITIONS", Group: GroupAI,
			Kind: KindInt, Default: 10,
			Doc: "The most titles a single suggestion may download.",
		},

		// --- Channels & playback (§15, Phase 10; policy defaults = programming-design §2) ---
		{
			Key: "sched.default_strategy", EnvVar: "SCHED_DEFAULT_STRATEGY", Group: GroupChannels,
			Kind: KindEnum, Enum: []EnumOption{opt("sequential", "Sequential"), opt("shuffle", "Shuffle")}, Default: "shuffle",
			Doc: "How channels order their programs by default, unless a channel sets its own.",
		},
		{
			Key: "sched.backfill", EnvVar: "SCHED_BACKFILL", Group: GroupChannels,
			Kind: KindEnum, Enum: []EnumOption{opt("stable", "Stable"), opt("reshuffle", "Reshuffle")}, Default: "stable",
			Doc: "When new titles arrive, keep the lineup order (stable) or reshuffle it.",
		},
		{
			Key: "channel.reconcile_every", EnvVar: "CHANNEL_RECONCILE_EVERY", Group: GroupChannels,
			Kind: KindDuration, Default: "10m",
			Doc: "How often Loomarr rebuilds channels to pick up newly-available content.",
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
			Kind: KindEnum, Enum: []EnumOption{opt("sequential", "Sequential"), opt("shuffle", "Shuffle"), opt("syndication", "Syndication")}, Default: "syndication",
			Doc: "Default program order (per-channel overridable). If a channel sets none, it uses its own strategy.",
		},
		{
			Key: "seasonal.mode", EnvVar: "SEASONAL_MODE", Group: GroupChannels,
			Kind: KindEnum, Enum: []EnumOption{opt("off", "Off"), opt("auto", "Auto (favor in-season)"), opt("exclusive", "In-season only")}, Default: "auto",
			Doc: "How channels handle seasonal content (per-channel overridable): off, auto (favor in-season), or only in-season.",
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
			Doc: "Seconds before the same commercial can play again.",
		},
		{
			Key: "filler.weight", EnvVar: "FILLER_WEIGHT", Group: GroupFiller,
			Kind: KindInt, Default: 1, Advanced: true,
			Doc: "How heavily this commercial set is drawn from, relative to others.",
		},

		// --- Filler ingest (§10, §15; loomarr:filler image variant only) ---
		// The two tool paths are what the `ingest` feature gate probes. They are
		// settings rather than hardcoded so an operator can point at a NEWER yt-dlp
		// than the image ships — yt-dlp releases fixes far faster than we cut images,
		// and a stale one silently stops extracting from YouTube.
		{
			Key: "ingest.ytdlp_path", EnvVar: "INGEST_YTDLP_PATH", Group: GroupFiller,
			Kind: KindString, Default: "", Advanced: true,
			Doc: "Where the yt-dlp program lives. The loomarr:filler image sets this; empty means clip downloading is off.",
		},
		{
			Key: "ingest.ffmpeg_path", EnvVar: "INGEST_FFMPEG_PATH", Group: GroupFiller,
			Kind: KindString, Default: "", Advanced: true,
			Doc: "Where the ffmpeg program lives (yt-dlp needs it to combine video and audio).",
		},
		{
			Key: "ingest.max_concurrent", EnvVar: "INGEST_MAX_CONCURRENT", Group: GroupFiller,
			Kind: KindInt, Default: 2, Advanced: true,
			Doc: "Maximum ingest sources downloaded in parallel.",
		},
		{
			Key: "ingest.timeout", EnvVar: "INGEST_TIMEOUT", Group: GroupFiller,
			Kind: KindDuration, Default: "30m", Advanced: true,
			Doc: "How long one download may run before it's stopped, so a stuck fetch can't block others.",
		},

		// --- Users & security (§15, Phase 9) ---
		{
			Key: "session.ttl", EnvVar: "SESSION_TTL", Group: GroupUsersSecurity,
			Kind: KindDuration, Default: "720h",
			Doc: "How long you stay signed in before needing to log in again.",
		},
		{
			Key: "cookie.secure", EnvVar: "COOKIE_SECURE", Group: GroupUsersSecurity,
			Kind: KindEnum, Enum: []EnumOption{opt("auto", "Auto (match the request)"), opt("always", "Always"), opt("never", "Never (local dev only)")}, Default: "auto",
			Doc: "When to mark the login cookie secure: auto (match the request), always, or never (for local dev only).",
		},
		{
			Key: "user.sync_every", EnvVar: "USER_SYNC_EVERY", Group: GroupUsersSecurity,
			Kind: KindDuration, Default: "1h", Advanced: true,
			Doc: "How often Loomarr refreshes imported users from your media server.",
		},

		// --- Advanced: TTLs, retention, workers, event webhook (§15) ---
		{
			Key: "request.ttl", EnvVar: "REQUEST_TTL", Group: GroupAdvanced,
			Kind: KindDuration, Default: "48h",
			Doc: "How long Loomarr keeps trying to request a title before giving up.",
		},
		{
			Key: "downloading.ttl", EnvVar: "DOWNLOADING_TTL", Group: GroupAdvanced,
			Kind: KindDuration, Default: "12h",
			Doc: "How long a downloading title waits to finish before Loomarr gives up on it.",
		},
		{
			Key: "reconcile.every", EnvVar: "RECONCILE_EVERY", Group: GroupAdvanced,
			Kind: KindDuration, Default: "5m",
			Doc: "How often Loomarr checks on in-progress downloads.",
		},
		// The background-job scheduler's per-job CRON schedules (§18.1). Sonarr/Overseerr-style
		// 6-field seconds-leading cron; edited via the Tasks page's Modify Job modal (presets
		// + an advanced raw-cron field). These OVERRIDE each job's built-in default cron.
		{
			Key: "job.reconcile.schedule", EnvVar: "JOB_RECONCILE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 */5 * * * *",
			Doc: "How often Loomarr checks on in-progress downloads (cron).",
		},
		{
			Key: "job.channel_sweep.schedule", EnvVar: "JOB_CHANNEL_SWEEP_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 */10 * * * *",
			Doc: "How often Loomarr reconciles channels with Tunarr (cron).",
		},
		{
			Key: "job.filler_sync.schedule", EnvVar: "JOB_FILLER_SYNC_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 */15 * * * *",
			Doc: "How often Loomarr syncs the filler catalog (cron).",
		},
		{
			Key: "job.session_sweep.schedule", EnvVar: "JOB_SESSION_SWEEP_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 0 * * * *",
			Doc: "How often Loomarr clears out expired sign-in sessions (cron).",
		},
		{
			Key: "job.library_scan.schedule", EnvVar: "JOB_LIBRARY_SCAN_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 */5 * * * *",
			Doc: "How often Loomarr scans the media server for newly-added titles to mark requested items available (cron).",
		},
		{
			Key: "job.library_full_scan.schedule", EnvVar: "JOB_LIBRARY_FULL_SCAN_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 0 3 * * *",
			Doc: "How often Loomarr does a full media-server sweep to catch anything the incremental scan missed (cron).",
		},
		{
			Key: "job.library_scan.lookback", EnvVar: "JOB_LIBRARY_SCAN_LOOKBACK", Group: GroupAdvanced,
			Kind: KindDuration, Default: "1h",
			Doc: "How far back the incremental library scan looks for newly-added titles (should exceed the scan interval).",
		},
		{
			Key: "job.arr_queue_poll.schedule", EnvVar: "JOB_ARR_QUEUE_POLL_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 * * * * *",
			Doc: "How often Loomarr polls Sonarr/Radarr download progress (cron; direct requester only).",
		},
		{
			Key: "job.workers", EnvVar: "JOB_WORKERS", Group: GroupAdvanced,
			Kind: KindInt, Default: 2,
			Doc: "How many channel suggestions can be worked on at once.",
		},
		{
			Key: "job.timeout", EnvVar: "JOB_TIMEOUT", Group: GroupAdvanced,
			Kind: KindDuration, Default: "10m",
			Doc: "How long one channel suggestion may run before it's stopped.",
		},
		{
			Key: "jobs.retention", EnvVar: "JOBS_RETENTION", Group: GroupAdvanced,
			Kind: KindDuration, Default: "720h",
			Doc: "How long finished suggestion jobs are kept before they're cleaned up.",
		},
		{
			Key: "proposals.retention", EnvVar: "PROPOSALS_RETENTION", Group: GroupAdvanced,
			Kind: KindDuration, Default: "2160h",
			Doc: "How long suggested lineups are kept before they're cleaned up.",
		},
		{
			Key: "event.webhook_url", EnvVar: "EVENT_WEBHOOK_URL", Group: GroupAdvanced,
			Kind: KindURL, Default: "",
			Doc: "Optional webhook Loomarr calls when a title finishes (or gives up). Leave empty to skip.",
		},
		{
			Key: "setup.completed", EnvVar: "SETUP_COMPLETED", Group: GroupAdvanced,
			Kind: KindBool, Default: false, Advanced: true,
			Doc: "Whether first-run setup is done. Until it is, Loomarr opens the setup wizard.",
		},
	}
}
