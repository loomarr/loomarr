package settings

// declaredAccess is Users & security and SSO.
func declaredAccess() []Setting {
	return []Setting{
		// --- Users & security (§15, Phase 9) ---
		{
			Key: "session.ttl", Label: "Sign-in lifetime", EnvVar: "SESSION_TTL", Group: GroupUsersSecurity,
			Kind: KindDuration, Default: "720h",
			Doc: "How long you stay signed in before needing to log in again.",
		},
		{
			Key: "cookie.secure", Label: "Secure cookies", EnvVar: "COOKIE_SECURE", Group: GroupUsersSecurity,
			Kind: KindEnum, Enum: []EnumOption{opt("auto", "Auto (match the request)"), opt("always", "Always"), opt("never", "Never (local dev only)")}, Default: "auto", Advanced: true,
			Doc: "When to mark the login cookie secure: auto (match the request), always, or never (for local dev only).",
		},
		{
			Key: "security.trust_proxy", Label: "Trust reverse proxy", EnvVar: "TRUST_PROXY", Group: GroupUsersSecurity,
			Kind: KindBool, Default: "false", Advanced: true,
			Doc:    "Turn on only when a reverse proxy sits in front of Loomarr.",
			Detail: "Trust the X-Forwarded-For and X-Forwarded-Proto headers. Turn this on only if a reverse proxy sits in front of Loomarr and sets them. The proxy must reach Loomarr from a loopback, private, link-local or Tailscale (100.64.0.0/10) address. Off by default so a direct client can't forge them to bypass the login rate limit or downgrade its cookie.",
		},

		// --- SSO: a third CREDENTIAL path, never a provisioning one (§11, D-F, V8) ---
		//
		// ⚠ There is deliberately NO `auth.sso.auto_create` and NO `auth.sso.admin_group`,
		// though the v2 mock draws both. Auto-create is lazy self-provision, which is exactly
		// what §11's allowlist exists to prevent; group-derived roles would move a Loomarr
		// decision to someone else's directory. Adding either key later is a §11 conversation,
		// not a settings change.
		{
			Key: "auth.sso.enabled", Label: "Enable single sign-on", EnvVar: "AUTH_SSO_ENABLED", Group: GroupSSO,
			Kind: KindBool, Default: "false",
			Doc:    "Let people sign in with your identity provider.",
			Detail: "They still need an account here — signing in with your provider does not create one.",
		},
		{
			Key: "auth.sso.issuer", Label: "SSO issuer URL", EnvVar: "AUTH_SSO_ISSUER", Group: GroupSSO,
			Kind: KindURL, Default: "",
			Doc:      "Your identity provider's address.",
			Detail:   "For example https://auth.example.home. Loomarr reads its published configuration from there.",
			ShowWhen: map[string][]string{"auth.sso.enabled": {"true"}},
		},
		{
			Key: "auth.sso.client_id", Label: "SSO client ID", EnvVar: "AUTH_SSO_CLIENT_ID", Group: GroupSSO,
			Kind: KindString, Default: "",
			Doc:      "The client ID your provider issued for Loomarr.",
			ShowWhen: map[string][]string{"auth.sso.enabled": {"true"}},
		},
		{
			Key: "auth.sso.client_secret", Label: "SSO client secret", EnvVar: "AUTH_SSO_CLIENT_SECRET", Group: GroupSSO,
			Kind: KindSecret, Default: "",
			Doc:      "The client secret your provider issued for Loomarr.",
			ShowWhen: map[string][]string{"auth.sso.enabled": {"true"}},
		},
	}
}

// declaredAdvanced is the Advanced group: TTLs, retention, workers, the event webhook, and the
// image-service jobs.
func declaredAdvanced() []Setting {
	return []Setting{
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
		// The background-job scheduler's per-job CRON schedules (§18.1). Sonarr/Overseerr-style
		// 6-field seconds-leading cron; edited via the Tasks page's Modify Job modal (presets
		// + an advanced raw-cron field). These OVERRIDE each job's built-in default cron.
		{
			Key: "job.system_health.schedule", EnvVar: "JOB_SYSTEM_HEALTH_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "*/30 * * * * *",
			Doc:    "How often Loomarr checks its database and connections for Current Health.",
			Detail: "A cron schedule.",
		},
		{
			Key: "job.notification_delivery.schedule", EnvVar: "JOB_NOTIFICATION_DELIVERY_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "*/15 * * * * *",
			Doc:    "How often Loomarr sends waiting invitation and recovery messages.",
			Detail: "Claims queued messages for delivery, on a cron schedule.",
		},
		{
			Key: "job.reconcile.schedule", EnvVar: "JOB_RECONCILE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */5 * * * *",
			Doc:    "How often Loomarr checks on in-progress downloads.",
			Detail: "A cron schedule.",
		},
		{
			Key: "job.channel_maintenance.schedule", EnvVar: "JOB_CHANNEL_MAINTENANCE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */10 * * * *",
			Doc:    "How often Loomarr refreshes series episodes and brings live channels up to date.",
			Detail: "Also reconciles live channels with Tunarr, on a cron schedule.",
		},
		{
			Key: "job.filler_sync.schedule", EnvVar: "JOB_FILLER_SYNC_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */15 * * * *",
			Doc:    "How often Loomarr syncs the filler catalog.",
			Detail: "A cron schedule.",
		},
		{
			// ⚠ Daily and off-peak on purpose. The window this job enforces is measured in WEEKS,
			// so a faster cadence buys nothing and only widens the chance of a pass landing while
			// an operator is mid-review on a reel one hour past its expiry.
			Key: "job.filler_split_sweep.schedule", EnvVar: "JOB_FILLER_SPLIT_SWEEP_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 45 4 * * *",
			Doc:    "How often Loomarr checks for split suggestions you never reviewed.",
			Detail: "A cron schedule. What it does when it finds them is set by `filler.split.review_window`.",
		},
		{
			// Hourly at :20, clear of the :00 pipeline-heavy minute: a better copy that becomes
			// ready replaces the old one within the hour, and a pass only decodes clips it has
			// never fingerprinted (#2033).
			Key: "job.filler_identity.schedule", EnvVar: "JOB_FILLER_IDENTITY_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 20 * * * *",
			Doc:    "How often Loomarr looks for copies of the same ad and keeps the best one.",
			Detail: "It also links shorter or alternate versions, on a cron schedule.",
		},
		{
			// Every five minutes: a new clip waits at the defects rung for this pass's verdict
			// (#2036), so the interval is how long admission waits. A pass decodes within a bounded
			// budget, held clips first, and an idle pass reads the records and decodes nothing.
			Key: "job.filler_defects.schedule", EnvVar: "JOB_FILLER_DEFECTS_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */5 * * * *",
			Doc:    "How often Loomarr checks ads' picture and sound against the standard for their era.",
			Detail: "A cron schedule. A new ad waits for this check before it airs, and an ad that fails is removed and a better copy sought.",
		},
		{
			// ⚠ **Every scheduled job needs its ScheduleKey declared here** — `Resolve` PANICS on
			// an undeclared key, so a job registered without one takes the whole app down at
			// startup rather than degrading. Caught by the boot test, which is exactly its job.
			//
			// Hourly rather than the 15-minute sync or the 6-hourly fetch, because this is the
			// expensive one: on the local backend a batch of 25 clips is minutes natively and
			// hours under QEMU (~341s per clip). Hourly drains a catalog steadily without a pass
			// overlapping the next.
			// ⚠ **This one key replaced FOUR** (§10 V51b), all now retired-ok:
			// `job.filler_language.schedule`, `job.filler_split.schedule` (retired-ok),
			// `job.filler_transcribe.schedule` and `job.filler_vision.schedule` (retired-ok).
			// Those four existed to keep expensive sweeps off each
			// other's toes by PHASE-OFFSETTING them (:15, :30, :45, :50) — a scheduling discipline
			// that only works while nobody adds a fifth, and which the comments they carried
			// spelled out at length.
			//
			// The pipeline makes the whole arrangement unnecessary rather than tidier: it runs
			// ONE clip at a time through all the rungs in order, so two expensive stages cannot
			// share the runner by construction. There is nothing left to offset.
			//
			// ⚠ Every two minutes, far tighter than the hourly sweeps, and affordable for the
			// reason the sweeps were not: a pass is bounded by the per-run budget rather than by
			// the catalog size, so an idle install costs one indexed query. It has to be tight,
			// because this is the ONLY thing that advances a freshly downloaded clip.
			//
			// ⚠ Every job needs its schedule key declared or the settings service PANICS at
			// startup on `Resolve` of an undeclared key — so a new job and its key always land in
			// the same change.
			Key: "job.filler_pipeline.schedule", EnvVar: "JOB_FILLER_PIPELINE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */2 * * * *",
			Doc:    "How often Loomarr prepares new filler clips.",
			Detail: "A cron schedule. Preparation means measuring, re-encoding, splitting, listening to and identifying them.",
		},
		// --- The image service jobs (§22, V52) ---
		//
		// ⚠ Every job needs its schedule key declared or the settings service PANICS at startup on
		// `Resolve` of an undeclared key, so all four land with the jobs themselves.
		{
			// Every minute, because this is what stands between an adopted row and a visible
			// image: `Adopt` deliberately does NOT fetch (a page adopting fifty posters would put
			// TMDB's latency on Loomarr's own page load), so until this runs the surface shows a
			// placeholder. A pass is bounded by the concurrency cap and an indexed work-list
			// query, so an idle install costs one cheap SELECT.
			Key: "job.images_fetch.schedule", EnvVar: "JOB_IMAGES_FETCH_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 * * * * *",
			Doc:    "How often Loomarr downloads artwork it knows about but hasn't fetched yet.",
			Detail: "A cron schedule. Until this runs, those images show as placeholders.",
		},
		{
			// ⚠ Every five minutes: this is the step between a clip's artwork being RENDERED and
			// that artwork being visible through the image service, so a slow cadence reads as the
			// feature not working while an operator watches an import. Cheap to run often — the
			// work list selects only clips with artwork on disk and no image identity yet, so a
			// healthy install pays one indexed query.
			Key: "job.images_adopt_artwork.schedule", EnvVar: "JOB_IMAGES_ADOPT_ARTWORK_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */5 * * * *",
			Doc:    "How often Loomarr copies clip thumbnails and previews into the shared image library.",
			Detail: "Covers thumbnails and hover previews, on a cron schedule. Until a clip has been copied over, its older thumbnail is still what you see.",
		},
		{
			// At :20, clear of the filler media cluster (:15/:30/:45/:50) and of the two 04:xx
			// backup/retention jobs. AVIF encoding is CPU-intensive, so this is the image job that
			// genuinely contends for the box — §22 keeps it off request latency so a cold grid of
			// 50 posters cannot launch 50 encodes at once.
			Key: "job.images_avif.schedule", EnvVar: "JOB_IMAGES_AVIF_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 20 * * * *",
			Doc:    "How often Loomarr makes the smallest copies of images in the background.",
			Detail: "Encodes the AVIF copies of images that don't have them yet, on a cron schedule. AVIF is the smallest format and the most expensive to produce, so it is made in the background; until it exists browsers take WebP.",
		},
		{
			Key: "job.images_maintenance.schedule", EnvVar: "JOB_IMAGES_MAINTENANCE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 0 5 * * *",
			Doc: "When Loomarr restores recoverable artwork, enforces retention, and cleans up image storage.",
		},
		{
			Key: "job.library_scan.schedule", EnvVar: "JOB_LIBRARY_SCAN_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */5 * * * *",
			Doc:    "How often Loomarr looks for new titles on the media server.",
			Detail: "Scans for newly-added titles to mark requested items available, on a cron schedule.",
		},
		{
			Key: "job.library_full_scan.schedule", EnvVar: "JOB_LIBRARY_FULL_SCAN_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 0 3 * * *",
			Doc:    "How often Loomarr does a full media-server sweep to catch anything the quick scan missed.",
			Detail: "The quick scan is the incremental one above. A cron schedule.",
		},
		{
			Key: "job.library_scan.lookback", EnvVar: "JOB_LIBRARY_SCAN_LOOKBACK", Group: GroupAdvanced,
			Kind: KindDuration, Default: "1h",
			Doc: "How far back the incremental library scan looks for newly-added titles (should exceed the scan interval).",
		},
		{
			Key: "job.arr_queue_poll.schedule", EnvVar: "JOB_ARR_QUEUE_POLL_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 * * * * *",
			Doc:    "How often Loomarr checks download progress in Sonarr and Radarr.",
			Detail: "A cron schedule. Only used when titles are requested from Sonarr and Radarr directly.",
		},
		{
			Key: "job.seerr_queue_poll.schedule", EnvVar: "JOB_SEERR_QUEUE_POLL_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 * * * * *",
			Doc:    "How often Loomarr asks Seerr how downloads are going.",
			Detail: "Polls Seerr for coarse acquisition status, on a cron schedule. Only used when titles are requested through Seerr.",
		},
		{
			Key: "job.workers", EnvVar: "JOB_WORKERS", Group: GroupAdvanced,
			Kind: KindInt, Default: 1,
			Doc:    "How many channel suggestions can be worked on at once.",
			Detail: "One is the appliance-safe default; raise it deliberately for larger or hosted-model deployments.",
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
			Key: "diagnostics.dir", EnvVar: "DIAGNOSTICS_DIR", Group: GroupAdvanced,
			Kind: KindString, Presentation: PresentationPath,
			DataSubdir: "diagnostics", Validate: storagePath(false), Advanced: true,
			Doc:    "Where Loomarr keeps the logs of its video and streaming programs.",
			Detail: "Where Loomarr keeps bounded ffmpeg and streaming-process output. The directory must be persistent if logs should survive a restart. A change applies to the next process; output already written stays where it was.",
		},
		{
			Key: "diagnostics.retention", EnvVar: "DIAGNOSTICS_RETENTION", Group: GroupAdvanced,
			Kind: KindDuration, Default: "168h", Validate: positiveDuration,
			Doc:    "How long diagnostic events and finished process logs are kept.",
			Detail: "How long Diagnostic events and completed Process runs are kept. Active Process runs are never removed by age.",
		},
		{
			Key: "diagnostics.max_storage_mb", EnvVar: "DIAGNOSTICS_MAX_STORAGE_MB", Group: GroupAdvanced,
			Kind: KindInt, Unit: UnitMiB, Default: 512, Validate: positiveWholeNumber,
			Doc:    "How much disk diagnostic events and process logs may use.",
			Detail: "Soft global storage budget in MiB for retained Diagnostic events and bounded Process output. Active Process runs remain protected.",
		},
		{
			Key: "job.housekeeping.schedule", EnvVar: "JOB_HOUSEKEEPING_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 30 4 * * *",
			Doc: "When Loomarr removes expired sessions and operational records beyond their retention periods.",
		},
		{
			Key: "setup.completed", EnvVar: "SETUP_COMPLETED", Group: GroupAdvanced,
			Kind: KindBool, Default: false, Advanced: true,
			Doc:    "Whether first-run setup is done.",
			Detail: "Until it is, Loomarr opens the setup wizard.",
		},
	}
}
