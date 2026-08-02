package settings

// declared is the canonical registry content: every app-managed setting, in the
// order it appears in design.md §15. This list IS the contract — design.md §15
// is its human mirror and `make config-docs` its generated reference. A key added
// here without a matching §15 row (or vice versa) is the drift CLAUDE.md forbids.
//
// Env-only bootstrap keys (DATABASE_URL, AUTO_MIGRATE, LISTEN_ADDR, LOG_LEVEL, TZ)
// are NOT here — they stay in config.Config (config-design §1 classification).
// Generated secrets (SESSION_SECRET, API_TOKEN, PLAYOUT_TOKEN) live in secrets.go
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
		{
			// RE-SCOPED by V4 (§9.1), not duplicated. This was a Tunarr-group, Advanced
			// knob documented as "only needed for uploaded channel icons". Internal
			// playout makes it the base every SEGMENT request resolves against, so it
			// moves to Playout and stops being Advanced — "get it wrong and channels
			// appear in the guide but never play" is not an advanced failure mode.
			//
			// Deliberately ONE key, not a second `playout.public_url`: it is genuinely
			// the server's own public address, and both callers (icon fetch, segment
			// fetch) need the same value. Two keys could drift, and an operator would
			// have to know which one Live TV reads.
			Key: "server.public_url", EnvVar: "SERVER_PUBLIC_URL", Group: GroupPlayout,
			Kind: KindURL, Default: "",
			Doc: "Loomarr's own address as your media server and Tunarr can reach it, e.g. http://loomarr:8080. Internal playout serves every stream segment from this base, so a wrong value means channels appear in the guide and never play. Also used for uploaded channel icons.",
		},

		// --- Playout (§9.1, §15 — added by V4) ---
		// Loomarr serves its own channels. `playout.backend` is the ONLY key here with a
		// per-channel override: it rides `policy_json` as `policy.playout` (nil = inherit
		// this global), the same shape `rules`/`filler`/`window`/`autoCurate` already use,
		// so there is no migration. That is what makes "changing this affects new channels
		// only — the ones already on the other backend keep playing" true rather than
		// aspirational: switching the default never touches an existing channel's policy.
		{
			Key: "playout.backend", EnvVar: "PLAYOUT_BACKEND", Group: GroupPlayout,
			Kind: KindEnum, Enum: []EnumOption{
				opt("internal", "Loomarr (internal)"),
				opt("tunarr", "Tunarr"),
			},
			Default: "internal",
			Doc:     "Who streams a channel. Internal playout is required for mid-roll breaks (§10) and reports real transcode telemetry. Tunarr remains fully supported — the right answer for hardware that cannot transcode, or an install that already works. Overridable per channel.",
		},
		{
			Key: "playout.transport", EnvVar: "PLAYOUT_TRANSPORT", Group: GroupPlayout,
			Kind: KindEnum, Enum: []EnumOption{
				opt("both", "HLS and MPEG-TS"),
				opt("hls", "HLS only"),
				opt("mpegts", "MPEG-TS only"),
			},
			Default: "both",
			Doc:     "Which stream formats internal playout offers. Media servers differ in what they accept, so both is the default: MPEG-TS matches Tunarr's existing shape and keeps latency low, HLS survives proxies.",
		},
		{
			Key: "playout.encoder", EnvVar: "PLAYOUT_ENCODER", Group: GroupPlayout,
			Kind: KindString, Default: "",
			Doc: "ffmpeg encoder for internal playout (e.g. libx264, h264_vaapi, h264_nvenc). Empty = pick the best one the transcode check found. Set it only to override that choice.",
		},
		{
			Key: "playout.audio_language", EnvVar: "PLAYOUT_AUDIO_LANGUAGE", Group: GroupPlayout,
			Kind: KindString, Default: "eng",
			Doc: "Preferred audio language for internal playout, as an ISO 639-2 code (eng, fra, spa, jpn). A preference, not a requirement: a film with no track in this language plays its first track rather than failing. Empty = play whichever track comes first in the file, which is how a foreign-language dub ends up playing instead of the original.",
		},
		{
			Key: "playout.quality_tier", EnvVar: "PLAYOUT_QUALITY_TIER", Group: GroupPlayout,
			Kind: KindEnum, Enum: []EnumOption{
				opt("efficient", "Efficient — 720p, lowest bandwidth"),
				opt("balanced", "Balanced — 1080p"),
				opt("quality", "Quality — 1080p, best picture"),
			},
			Default: "balanced",
			Doc:     "The picture-versus-bandwidth target. Efficient is 720p and roughly half the bitrate — the right answer for a NAS running several channels, or for watching away from home. Balanced is 1080p and the default. Quality is 1080p at a higher frame rate and bitrate, which on grainy or dark film can be visibly cleaner but costs noticeably more bandwidth per channel. Whichever you pick, playout still steps down automatically as more channels start, so the choice is a ceiling rather than a promise.",
		},
		{
			// ⚠ Still separate from ingest.ffmpeg_path, but NOT for the reason this comment
			// used to give ("the filler sidecar bundles its own ffmpeg in a different image").
			// There is one image now (§16), so that rationale died with the sidecar and the
			// two-tag split.
			//
			// The live reason: these two callers fail differently. Playout's ffmpeg is a
			// runtime dependency of a channel that is ON AIR, while ingest's is a build
			// dependency of a download nobody is watching — so an operator pointing ingest at
			// a newer yt-dlp-compatible ffmpeg must not be able to break playout by doing it.
			Key: "playout.ffmpeg_path", EnvVar: "PLAYOUT_FFMPEG_PATH", Group: GroupPlayout,
			Kind: KindString, Default: "ffmpeg", Advanced: true,
			Doc: "Where the ffmpeg program lives. The default works whenever ffmpeg is on the system PATH; set it only if yours is somewhere unusual.",
		},
		{
			Key: "playout.max_channels", EnvVar: "PLAYOUT_MAX_CHANNELS", Group: GroupPlayout,
			Kind: KindInt, Default: "4",
			Doc: "How many channels internal playout will encode at once. Defaults conservatively; the wizard's transcode check measures a realistic number for your hardware. A test pattern is cheaper to encode than film grain, so treat any measured value as a starting estimate.",
		},

		{
			// The guide's display timezone (§12, V13b gap 7). The API always speaks absolute
			// epoch ms — a timezone is a RENDERING choice, and putting it on the wire would
			// invite a client to reinterpret instants it should merely format.
			//
			// Empty = the viewer's own browser timezone, which is right for the household
			// case. An operator sets it when the server and its viewers are elsewhere, or
			// when they want the guide to read in the channels' "broadcast" timezone.
			Key: "guide.timezone", EnvVar: "GUIDE_TIMEZONE", Group: GroupPlayout,
			Kind: KindString, Default: "", Advanced: true,
			Doc: "Which timezone the TV guide's times are shown in, as an IANA name like America/New_York. Leave empty to use each viewer's own device timezone.",
		},
		{
			// How far back the guide will look (§12, V13b gap 8).
			//
			// A real bound, not a cosmetic one: the past is recomputed from the channel's
			// CURRENT lineup, so a distant "as aired" view would be fiction — the lineup has
			// been reconciled since. A day is honest; a month would be invention presented as
			// history.
			Key: "guide.retention_hours", EnvVar: "GUIDE_RETENTION_HOURS", Group: GroupPlayout,
			Kind: KindInt, Default: "24", Advanced: true,
			Doc: "How far back the TV guide lets you scroll, in hours. Past listings are recomputed from each channel's current lineup, so going too far back would show a schedule that never actually aired.",
		},

		// --- Backup (§16, §15 — added by V4) ---
		{
			Key: "backup.schedule", EnvVar: "BACKUP_SCHEDULE", Group: GroupBackup,
			Kind: KindCron, Default: "0 30 3 * * *",
			Doc: "When to write the nightly instance backup. A backup is the whole instance — settings, channels, people, and the generated secrets — so treat the file as a credential.",
		},
		{
			Key: "backup.retain", EnvVar: "BACKUP_RETAIN", Group: GroupBackup,
			Kind: KindInt, Default: "7",
			Doc: "How many backups to keep before pruning the oldest.",
		},
		{
			Key: "backup.dir", EnvVar: "BACKUP_DIR", Group: GroupBackup,
			Kind: KindString, Default: "/data/backups",
			Doc: "Where backups are written. Defaults inside /data so the documented volume carries them; point it elsewhere to keep backups off the same disk as the database.",
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
			// Local-only (§8.2): a hosted service has no model to hold in memory, so this
			// is hidden for the openai provider rather than shown as an inert control.
			Key: "llm.keep_alive", EnvVar: "LLM_KEEP_ALIVE", Group: GroupAI,
			Kind: KindDuration, Default: "30m", Advanced: true,
			Doc:      "How long to keep the local AI model loaded in memory between requests. Loading it takes several seconds, so keeping it ready makes suggestions much faster. Set to 0 to free the memory as soon as each request finishes.",
			ShowWhen: map[string][]string{"llm.provider": {"ollama"}},
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

		// --- Self-updating channels / re-curation (programming-design §8.2) ---
		{
			Key: "job.recurate.schedule", EnvVar: "JOB_RECURATE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 0 4 * * 0", Advanced: true,
			Doc: "How often auto-curate channels re-evaluate their intent against the library (cron). Weekly by default — this runs the AI, so keep it infrequent.",
		},
		{
			Key: "recurate.min_score_pct", EnvVar: "RECURATE_MIN_SCORE_PCT", Group: GroupAI,
			Kind: KindInt, Default: 60, Advanced: true,
			Doc: "Quality bar (0–100) a not-in-library title must clear for auto-curate to REQUEST it. In-library matches are added regardless. A per-channel override may be stricter or looser.",
		},
		{
			Key: "recurate.max_titles", EnvVar: "RECURATE_MAX_TITLES", Group: GroupAI,
			Kind: KindInt, Default: 40, Advanced: true,
			Doc: "The most titles an auto-curate channel may grow to. Re-curation won't request net-new titles past this cap. A per-channel override may be stricter or looser.",
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
			Key: "sched.window_hours", EnvVar: "SCHED_WINDOW_HOURS", Group: GroupChannels,
			Kind: KindDuration, Default: "24h",
			Doc: "How far ahead each channel schedules — the rolling window it materializes and rolls forward, instead of the whole series run (per-channel/-rule overridable; 0 = schedule everything).",
		},
		{
			Key: "seasonal.mode", EnvVar: "SEASONAL_MODE", Group: GroupChannels,
			Kind: KindEnum, Enum: []EnumOption{opt("off", "Off"), opt("auto", "Auto (favor in-season)"), opt("exclusive", "In-season only")}, Default: "auto",
			Doc: "How channels handle seasonal content (per-channel overridable): off, auto (favor in-season), or only in-season.",
		},

		// --- Filler / commercials (§15, Phase 12; §10 redesign — Tunarr-owned) ---
		{
			// ⚠ Defaults inside /data, like database.url and backup.dir — the documented
			// volume carries it, so a zero-env `docker run` has a working drop-folder
			// instead of a Filler page that is one empty state telling the operator to go
			// and configure something. It was "" for no recorded reason while its two
			// neighbours both defaulted; that asymmetry made the whole feature opt-in by
			// accident. Still overridable to point at an existing library on another disk.
			Key: "filler.dir", EnvVar: "FILLER_DIR", Group: GroupFiller,
			Kind: KindString, Default: "/data/filler", Required: FeatureFiller,
			Doc: "Drop-folder Loomarr registers as a Tunarr 'local' source for commercials/bumpers. Defaults inside /data so the documented volume carries it; point it elsewhere to use an existing clip library.",
		},
		{
			Key: "filler.sync_every", EnvVar: "FILLER_SYNC_EVERY", Group: GroupFiller,
			Kind: KindDuration, Default: "15m",
			Doc: "How often to re-sync the filler catalog from Tunarr's local source.",
		},
		{
			// The drop-folder's on/off switch on the Sources tab (§10 V35). A setting rather
			// than a row because the folder is DERIVED from `filler.dir` — a remote
			// collection's switch is a column on its own row, so a row and a setting never
			// describe the same source.
			//
			// ⚠ Default TRUE, and it must stay that way: an install that has never seen this
			// key has a working drop-folder, and defaulting to false would silently stop the
			// scan on upgrade.
			//
			// ⚠ There is deliberately no `filler.source.library.enabled`. Nothing scans a
			// media-server library for clips (§10 took the media server out of the filler
			// path), so that key would gate no work — a control that dims a row and changes
			// nothing. The Sources tab renders that row as provenance, without a switch.
			Key: "filler.source.folder.enabled", EnvVar: "FILLER_SOURCE_FOLDER_ENABLED", Group: GroupFiller,
			Kind: KindBool, Default: true,
			Doc: "Scan the drop-folder for clips. Switching it off stops the catalog sync; clips already in the catalog stay.",
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
		// ⚠ Default 0 = OFF, and that is the whole safety property of this knob (V17c).
		// `00014_clips_quality` shipped with quality as display-only and warned that a
		// well-meaning "prefer HD" would quietly starve the era-accurate 4:3 commercials the
		// feature exists to play. That warning still holds — which is why an install that sets
		// nothing behaves exactly as it did before this key existed, pinned by a test.
		//
		// Advanced: an operator who does not know what 240p looks like in a break should never
		// meet this, and one who does will go looking.
		{
			Key: "filler.min_quality", EnvVar: "FILLER_MIN_QUALITY", Group: GroupFiller,
			Kind: KindInt, Default: 0, Advanced: true,
			Doc: "Minimum clip height in pixels for a commercial to be eligible (480 excludes 240p rips). 0 disables the floor, which is the default — era accuracy beats resolution.",
		},
		{
			Key: "filler.weight", EnvVar: "FILLER_WEIGHT", Group: GroupFiller,
			Kind: KindInt, Default: 1, Advanced: true,
			Doc: "How heavily this commercial set is drawn from, relative to others.",
		},

		// --- Filler ingest (§10, §15) ---
		// ⚠ The vendored binaries ship in the SINGLE image (§16). This block used to be
		// labelled "loomarr:filler image variant only" — that variant no longer exists, so
		// these paths are overrides for an unusual layout, not an opt-in.
		// The two tool paths are what the `ingest` feature gate probes. They are
		// settings rather than hardcoded so an operator can point at a NEWER yt-dlp
		// than the image ships — yt-dlp releases fixes far faster than we cut images,
		// and a stale one silently stops extracting from YouTube.
		{
			Key: "ingest.ytdlp_path", EnvVar: "INGEST_YTDLP_PATH", Group: GroupFiller,
			Kind: KindString, Default: "", Advanced: true,
			Doc: "Where the yt-dlp program lives. The Loomarr image sets this; empty means clip downloading is off.",
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
		// Compilation splitting (§10, V34). Empty/unrunnable ⇒ the transcript-rescue
		// step is unavailable and over-long segments surface as UNSPLITTABLE in the
		// review rather than being guessed at — coarse splitting needs only ffmpeg.
		{
			Key: "ingest.whisper_path", EnvVar: "INGEST_WHISPER_PATH", Group: GroupFiller,
			Kind: KindString, Default: "", Advanced: true,
			Doc: "Where the whisper-cli program lives. The image sets this; empty means over-long compilation segments can't be transcribed for hidden ad breaks.",
		},
		{
			Key: "ingest.whisper_model", EnvVar: "INGEST_WHISPER_MODEL", Group: GroupFiller,
			Kind: KindString, Default: "", Advanced: true,
			Doc: "The whisper model file whisper-cli transcribes with. Size is a correctness property, not a quality preference — too small drops audio and the boundary detector then invents breaks.",
		},
		// The starter pack (§10, V17d). A DEFAULT, not a hardcoded truth: an operator can
		// point it at their own collection, and emptying it turns the pack off. Listing
		// only — nothing downloads until the operator keeps a row.
		{
			Key: "filler.starter_collection", EnvVar: "FILLER_STARTER_COLLECTION", Group: GroupFiller,
			Kind: KindString, Default: "classic_tv_commercials",
			Doc: "The archive.org collection suggested as a starter pack when your clip catalog is empty. Nothing downloads until you pick from it. Leave empty to turn the suggestion off.",
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

		// --- SSO: a third CREDENTIAL path, never a provisioning one (§11, D-F, V8) ---
		//
		// ⚠ There is deliberately NO `auth.sso.auto_create` and NO `auth.sso.admin_group`,
		// though the v2 mock draws both. Auto-create is lazy self-provision, which is exactly
		// what §11's allowlist exists to prevent; group-derived roles would move a Loomarr
		// decision to someone else's directory. Adding either key later is a §11 conversation,
		// not a settings change.
		{
			Key: "auth.sso.enabled", EnvVar: "AUTH_SSO_ENABLED", Group: GroupSSO,
			Kind: KindBool, Default: "false",
			Doc: "Let people sign in with your identity provider. They still need an account here — signing in with your provider does not create one.",
		},
		{
			Key: "auth.sso.issuer", EnvVar: "AUTH_SSO_ISSUER", Group: GroupSSO,
			Kind: KindURL, Default: "",
			Doc: "Your identity provider's address, e.g. https://auth.example.home. Loomarr reads its published configuration from there.",
		},
		{
			Key: "auth.sso.client_id", EnvVar: "AUTH_SSO_CLIENT_ID", Group: GroupSSO,
			Kind: KindString, Default: "",
			Doc: "The client ID your provider issued for Loomarr.",
		},
		{
			Key: "auth.sso.client_secret", EnvVar: "AUTH_SSO_CLIENT_SECRET", Group: GroupSSO,
			Kind: KindSecret, Default: "",
			Doc: "The client secret your provider issued for Loomarr.",
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
			Key: "job.series_episode_refresh.schedule", EnvVar: "JOB_SERIES_EPISODE_REFRESH_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 0 * * * *",
			Doc: "How often Loomarr re-reads the episode lists of shows used by channels, so the guide doesn't have to ask the media server on every load (cron).",
		},
		{
			Key: "episodes.max_age", EnvVar: "EPISODES_MAX_AGE", Group: GroupAdvanced,
			Kind: KindDuration, Default: "24h",
			Doc: "How stale a cached series episode list may be before it is re-read from the media server. A missing or expired entry still falls back to a live read, so this bounds freshness, never correctness.",
		},
		{
			Key: "job.arr_queue_poll.schedule", EnvVar: "JOB_ARR_QUEUE_POLL_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 * * * * *",
			Doc: "How often Loomarr polls Sonarr/Radarr download progress (cron; direct requester only).",
		},
		{
			Key: "job.seerr_queue_poll.schedule", EnvVar: "JOB_SEERR_QUEUE_POLL_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 * * * * *",
			Doc: "How often Loomarr polls Seerr for coarse acquisition status (cron; Seerr requester only).",
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
			Key: "activity.retention", EnvVar: "ACTIVITY_RETENTION", Group: GroupAdvanced,
			Kind: KindDuration, Default: "720h",
			Doc: "How long the Dashboard's recent-activity entries are kept before they're cleaned up.",
		},
		{
			Key: "job.retention_purge.schedule", EnvVar: "JOB_RETENTION_PURGE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 30 4 * * *",
			Doc: "When to clean up finished suggestion jobs and declined requests.",
		},
		{
			Key: "job.activity_purge.schedule", EnvVar: "JOB_ACTIVITY_PURGE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 15 4 * * *",
			Doc: "When to clean up old recent-activity entries.",
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
