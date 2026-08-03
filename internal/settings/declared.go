package settings

import "fmt"

// declared is the canonical registry content: every app-managed setting, in the
// order it appears in design.md §15. This list IS the contract — design.md §15
// is its human mirror and `make config-docs` its generated reference. A key added
// here without a matching §15 row (or vice versa) is the drift CLAUDE.md forbids.
//
// Env-only bootstrap keys (DATABASE_URL, AUTO_MIGRATE, LISTEN_ADDR, LOG_LEVEL, TZ)
// are NOT here — they stay in config.Config (config-design §1 classification).
// Generated secrets (SESSION_SECRET, API_TOKEN, PLAYOUT_TOKEN) live in secrets.go
// (minted, not demanded — §4), not the app-managed registry.

// autoFileConfidenceRange bounds `filler.autofile.min_confidence` to 50–95 (§10 V38).
//
// ⚠ **The upper bound is load-bearing, not cosmetic.** `filler.MaxAutoFileConfidence` is 95 and
// an ungrounded era is capped strictly BELOW it, which is what guarantees no settable threshold
// can auto-file a fabricated era. Raising this ceiling without raising that cap silently breaks
// the guarantee §10 makes.
//
// ⚠ The number is repeated here rather than imported: `settings` must not depend on `filler`
// (the dependency runs the other way — the tagger reads settings). `filler`'s own test pins the
// relationship, so a divergence fails there rather than going unnoticed.
//
// The lower bound is a usability floor: below 50 the threshold admits clips whose tags did not
// fully verify, which makes Incoming an empty room and the catalog a surprise.
func autoFileConfidenceRange(v any) error {
	n, ok := v.(int)
	if !ok {
		return fmt.Errorf("want a whole number")
	}
	if n < 50 || n > 95 {
		return fmt.Errorf("want 50-95 (got %d) — below 50 files clips whose tags didn't verify; above 95 nothing would ever file", n)
	}
	return nil
}

// positiveLimit rejects a zero or negative cap.
//
// ⚠ Zero is refused rather than treated as "unlimited". A limit key silently meaning its own
// opposite is how an operator sets 0 expecting "no fetching" and gets an uncapped crawler.
// Turning auto-fetch OFF is `filler.fetch.every = 0`, which says what it does.
func positiveLimit(v any) error {
	n, ok := v.(int)
	if !ok {
		return fmt.Errorf("want a whole number")
	}
	if n < 1 {
		return fmt.Errorf("want 1 or more (got %d) — to stop fetching automatically, set filler.fetch.every to 0", n)
	}
	return nil
}

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
			Doc: "Where Loomarr stores clips. Each is filed under its content hash with its metadata beside it. Defaults inside /data so the documented volume carries it; point it elsewhere to use an existing clip library.",
		},
		{
			// The watch folder (§10 V38c, "Two folders, one pipeline"). Clips ARRIVE here —
			// downloads land here, operators drop files here — and every sync drains it into the
			// clip folder above.
			//
			// ⚠ **The default is EMPTY, resolved to `<filler.dir>/_watch` by the reader**, not a
			// literal `/data/filler/_watch`. A literal would silently stop tracking the moment an
			// operator pointed `filler.dir` at an existing library on another disk: arrivals would
			// keep landing under `/data` while the catalog looked elsewhere, and the drop-folder
			// would appear broken with both settings looking correct.
			//
			// ⚠ **Inside the clip folder rather than a sibling.** A sibling needs its own mounted
			// volume, and an unmounted watch folder loses anything not yet filed on the next
			// restart — silently, because an empty folder is also what success looks like. The
			// scan skips `_watch` by name so a waiting file is never catalogued from its arrival
			// path (which would then be pruned the moment intake moved it).
			Key: "filler.watch_dir", EnvVar: "FILLER_WATCH_DIR", Group: GroupFiller,
			Kind: KindString, Default: "",
			Doc: "Folder Loomarr watches for new clips. Anything dropped here is filed into your clip folder and then removed. Leave blank to use a '_watch' folder inside the clip folder.",
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
		// Auto-filing (§10 V38). ⚠ These two keys were REMOVED from §15 in V35's review as
		// declared-but-unconsumed — §15's own rule is that a setting not in the registry does not
		// exist, and the corollary is that one nothing READS does not either. They return here
		// with their consumer in the same PR: the tag job reads them, and a test proves a clip
		// below the threshold reaches Incoming instead of the catalog.
		//
		// ⚠ **ON by default** (maintainer, 2026-08-02), which means an existing install begins
		// filing clips without a human on its first tagging run after upgrade. What makes that
		// safe is not this number but §10's grounding CAP: an ungrounded era scores below every
		// reachable threshold, so the fabrication class stays with a person regardless of what
		// this is set to. `filler.Score` owns that property and a sabotage test pins it.
		{
			Key: "filler.autofile.enabled", EnvVar: "FILLER_AUTOFILE_ENABLED", Group: GroupFiller,
			Kind: KindBool, Default: true,
			Doc: "File confidently-tagged clips into the catalog automatically. Anything Loomarr is unsure about waits for you under Filler → Incoming.",
		},
		{
			// ⚠ Max is filler.MaxAutoFileConfidence (95), and the ceiling is load-bearing rather
			// than cosmetic: an ungrounded era is capped BELOW it, so no settable value can admit
			// a fabricated era. Raising this bound without raising that cap breaks the guarantee.
			Key: "filler.autofile.min_confidence", EnvVar: "FILLER_AUTOFILE_MIN_CONFIDENCE", Group: GroupFiller,
			Kind: KindInt, Default: 85, Validate: autoFileConfidenceRange,
			Doc: "How sure Loomarr must be before filing a clip without asking (50–95). Lower files more automatically; higher sends more to Incoming for you to check.",
		},
		// ⚠ `filler.autofile.normalize_loudness` is NOT declared here yet, deliberately. The
		// loudness pass (ffmpeg `loudnorm`) is real work that is not built, and §15's rule is that
		// a setting nothing READS does not exist — the exact defect that got the two keys above
		// removed in V35's review. Declaring the toggle first would repeat it inside the very
		// phase that records the lesson. It lands with its consumer.
		// Auto-fetch and its limits (§10 V38b). A registered source is polled on a schedule, which
		// supersedes §15's "there is no unattended crawler" — the superseded rule's concern
		// survives as these bounds rather than as a prohibition.
		//
		// ⚠ Every one of them fails toward doing LESS. An operator who never opens this page gets
		// a trickle they can live with; the failure mode being designed against is "add a source,
		// wake up to 8,000 files".
		{
			Key: "filler.fetch.every", EnvVar: "FILLER_FETCH_EVERY", Group: GroupFiller,
			Kind: KindDuration, Default: "6h",
			Doc: "How often Loomarr checks your sources for new clips. Set to 0 to stop fetching automatically — you can still queue clips yourself.",
		},
		{
			Key: "filler.fetch.max_per_run", EnvVar: "FILLER_FETCH_MAX_PER_RUN", Group: GroupFiller,
			Kind: KindInt, Default: 10, Validate: positiveLimit,
			Doc: "How many clips one source may download each time it's checked. Keeps a big collection trickling in instead of arriving all at once.",
		},
		{
			// ⚠ Bounds the UNATTENDED path only. An admin queueing a clip or approving a pull is
			// a deliberate act and is not stopped by this — a ceiling on what happens while
			// nobody is looking is not a ceiling on what someone chooses to do.
			Key: "filler.fetch.max_catalog_clips", EnvVar: "FILLER_FETCH_MAX_CATALOG_CLIPS", Group: GroupFiller,
			Kind: KindInt, Default: 2000, Validate: positiveLimit,
			Doc: "Stop fetching automatically once your catalog reaches this many clips. You can still add more by hand.",
		},
		{
			Key: "filler.fetch.max_disk_gb", EnvVar: "FILLER_FETCH_MAX_DISK_GB", Group: GroupFiller,
			Kind: KindInt, Default: 20, Validate: positiveLimit,
			Doc: "Stop fetching automatically once the filler folder reaches this size in GB.",
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
		{
			// ⚠ A REJECT, not a filter, and its default is ON — unlike filler.min_quality above,
			// which is opt-in eligibility over clips that already exist. A file shorter than this
			// never becomes a catalog row at all (§10 V40).
			//
			// It exists because `DurationMs <= 0` was the only guard, and a 2.9KB / 33ms truncated
			// download passed it and sat filed-and-airable in the dev catalog.
			Key: "filler.min_duration", EnvVar: "FILLER_MIN_DURATION", Group: GroupFiller,
			Kind: KindDuration, Default: "10s", Advanced: true,
			Doc: "Clips shorter than this are rejected on sight and never enter the catalog — a truncated download is not a short commercial. Set to 0s to accept anything with a readable duration.",
		},
		{
			// ⚠ Applied in the PLAYOUT chain, never written back to the file. The drop-folder
			// holds the operator's own files; in-place normalisation is destructive, unrepeatable,
			// and a re-scan cannot tell it already happened (§10 V40, §9.1).
			//
			// Measured spread across real fetched clips: -21.8 to -32.6 LUFS, ~11 dB of
			// clip-to-clip jump. -23 is the broadcast target.
			Key: "filler.target_lufs", EnvVar: "FILLER_TARGET_LUFS", Group: GroupFiller,
			Kind: KindString, Default: "-23", Advanced: true,
			Doc: "Loudness every filler clip is normalised to at playout, in LUFS (-23 is the broadcast standard). Empty disables normalisation and clips play at whatever level they were recorded.",
		},
		{
			// ⚠ A clip with NO speech is always kept — a wordless visual spot has no language, and
			// those are often the best filler. Only confident non-target speech rejects (§10 V40).
			Key: "filler.language", EnvVar: "FILLER_LANGUAGE", Group: GroupFiller,
			Kind: KindString, Default: "en", Advanced: true,
			Doc: "The language filler is expected to be in. A clip whose speech is confidently something else is rejected; a clip with no speech at all is always kept. Empty turns the language check off.",
		},
		{
			// Mirrors `llm.provider`'s local-vs-hosted split (§8.1), and for the same reason:
			// local is free and offline, hosted costs money and leaves the box.
			//
			// ⚠ **NOT Ollama.** "We already run a local LLM so we do not need whisper" is the
			// reasonable inference and it is wrong — Ollama has no audio input path at all
			// (probed 2026-08-03: completion/vision/tools/thinking, no `audio`; vision is images).
			// Local audio means whisper; hosted is what Ollama cannot be.
			//
			// ⚠ whisper is ~3s per clip natively but **~341s under QEMU**, which is why the job
			// runs in the background and why an arm64 install effectively needs the hosted path.
			Key: "filler.language_provider", EnvVar: "FILLER_LANGUAGE_PROVIDER", Group: GroupFiller,
			Kind: KindEnum, Enum: []EnumOption{
				opt("whisper", "Local (whisper)"), opt("hosted", "Hosted AI service"),
			},
			Default: "whisper", Advanced: true,
			Doc: "What works out a clip's language: the built-in local engine (free and offline, but slow on low-power hardware), or a hosted AI service (fast anywhere, costs a fraction of a cent per clip and sends a few seconds of audio off this machine).",
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
		{
			// ⚠ A SECOND model, and the reason is the `.en` suffix on the one above. An
			// English-only whisper build does NOT identify languages — it assumes English and
			// transcribes accordingly, so asked about a Spanish advert it answers "en" and the
			// language gate silently never rejects anything (§10 V40).
			//
			// `tiny` (multilingual, ~74MB) is adequate here in a way it was not for splitting:
			// language ID is CLASSIFICATION over the first seconds, not transcription, so the
			// "does it drop audible speech" gate that ruled out tiny.en never applies.
			//
			// Empty ⇒ local language detection is unavailable and the gate stays inert; the image
			// sets it, a source build does not.
			Key: "filler.language_model", EnvVar: "FILLER_LANGUAGE_MODEL", Group: GroupFiller,
			Kind: KindString, Default: "", Advanced: true,
			Doc: "The model file used to work out what language a clip is in. Must be a MULTILINGUAL whisper model — an English-only one reports every clip as English, so the check would never reject anything. The image ships one; leave empty to turn local detection off.",
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
			// ⚠ A scheduler Job's `ScheduleKey` MUST be declared here — `Resolve` panics on an
			// undeclared key, so a job registered without its row takes the whole app down at
			// boot. Caught by `make check`, which is the right place, but the coupling is easy
			// to miss when adding a job.
			//
			// Distinct from `filler.fetch.every`, which is the operator-facing "how often" in the
			// Filler group; this is the cron the scheduler actually runs on, in Advanced beside
			// its siblings. The two agree by default (6h).
			Key: "job.filler_fetch.schedule", EnvVar: "JOB_FILLER_FETCH_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Default: "0 0 */6 * * *",
			Doc: "How often Loomarr checks your filler sources for new clips (cron).",
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
