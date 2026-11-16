package settings

// declaredChannelsAndFiller is channels and playback, then filler (commercials and ingest).
func declaredChannelsAndFiller() []Setting {
	return []Setting{
		// --- Channels & playback (§15, Phase 10; policy defaults = programming-design §2) ---
		{
			Key: "channel.reconcile_every", Label: "Channel rebuild cooldown", EnvVar: "CHANNEL_RECONCILE_EVERY", Group: GroupChannels,
			Kind: KindDuration, Default: "10m", Advanced: true,
			Doc:    "How long a channel rests after a rebuild before it can be rebuilt again.",
			Detail: "Minimum delay after a successful rebuild before that channel is eligible for another scheduled sweep. Change the sweep cadence under System → Tasks.",
		},
		{
			Key: "sched.window_hours", Label: "Schedule ahead", EnvVar: "SCHED_WINDOW_HOURS", Group: GroupChannels,
			Kind: KindDuration, Default: "24h",
			Doc:    "How far ahead each channel plans its schedule.",
			Detail: "How far ahead each channel schedules — the rolling window it materializes and rolls forward, instead of the whole series run (per-channel/-rule overridable; 0 = schedule everything).",
		},

		// --- Filler / commercials (§15, Phase 12; §10 redesign — Tunarr-owned) ---
		{
			Key: "filler.home_country", Label: "Country", EnvVar: "FILLER_HOME_COUNTRY", Group: GroupFiller,
			Kind: KindString, Default: "", Validate: optionalCountryCode,
			Doc:    "The country your channels are watched in, for holidays and filler.",
			Detail: "Where your channels are watched. Loomarr uses this country for channels and filler sources, and keeps its holidays on its dates (Canada's Thanksgiving in October, Britain's Mothering Sunday in spring). Empty, or a country Loomarr has no holidays for yet, keeps the US calendar.",
		},
		{
			// #2062: a picked place (an embedded location index id), never a typed name — a name
			// could not be measured against a broadcast station's community of license.
			Key: "filler.home_place", Label: "Home city", EnvVar: "FILLER_HOME_PLACE", Group: GroupFiller,
			Kind: KindString, Default: "", Validate: optionalPlaceID,
			Doc:    "Your city, so local station news and weather can air between programmes.",
			Detail: "Optional. Pick your city to get local TV material, such as station news and weather, from stations near you. Without it, only national filler airs.",
		},
		{
			Key: "filler.local_radius_km", Label: "Local station radius", EnvVar: "FILLER_LOCAL_RADIUS_KM", Group: GroupFiller,
			Kind: KindInt, Default: "100", Validate: localRadiusKM,
			Doc: "How far from your city a TV station can be and still count as local, in kilometres.",
		},
		{
			Key: "filler.region_fallback", Label: "Other countries", EnvVar: "FILLER_REGION_FALLBACK", Group: GroupFiller,
			Kind: KindStringList, Default: "", Validate: countryCodeList,
			Doc:    "Other countries whose commercials may fill in when your own run short.",
			Detail: "Optional, in order of preference: countries whose national commercials may air and whose sources may download after your own country's, such as GB, CA, AU. Your country's commercials always come first within an era; these fill in when its supply for that era runs thin.",
		},
		{
			// ⚠ Defaults inside /data, like database.url and backup.dir — the documented
			// volume carries it, so a zero-env `docker run` has a working drop-folder
			// instead of a Filler page that is one empty state telling the operator to go
			// and configure something. It was "" for no recorded reason while its two
			// neighbours both defaulted; that asymmetry made the whole feature opt-in by
			// accident. Still overridable to point at an existing library on another disk.
			Key: "filler.dir", Label: "Clip library", EnvVar: "FILLER_DIR", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Apply: ApplyRestart, DataSubdir: "filler", Required: FeatureFiller, Validate: storagePath(false),
			Doc:    "Where Loomarr stores clips.",
			Detail: "Each is filed under its content hash with its metadata beside it. Defaults to a directory beside the database (/data in the container) so its volume carries it; point it elsewhere to use an existing clip library.",
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
			Key: "filler.watch_dir", Label: "Drop folder", EnvVar: "FILLER_WATCH_DIR", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Apply: ApplyRestart, Default: "", Validate: storagePath(true),
			Doc:    "A folder where any clip you drop in is added to your library.",
			Detail: "Folder Loomarr watches for new clips. Anything dropped here is filed into your clip folder and then removed. Leave blank to use a '_watch' folder inside the clip folder.",
		},
		{
			Key: "filler.sync_every", Label: "Scan for dropped clips every", EnvVar: "FILLER_SYNC_EVERY", Group: GroupFiller,
			Kind: KindDuration, Default: "15m", Advanced: true,
			Doc: "How often Loomarr drains the drop folder and reconciles its own clip library.",
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
			Key: "filler.source.folder.enabled", Label: "Watch the drop folder", EnvVar: "FILLER_SOURCE_FOLDER_ENABLED", Group: GroupFiller,
			Kind: KindBool, Default: true,
			Doc:    "Pick up clips dropped into the drop folder.",
			Detail: "Scan the drop-folder for clips. Switching it off stops the catalog sync; clips already in the catalog stay.",
		},
		{
			// Selective speech details (§10 V44). Enabled by default because it only listens when a
			// source did not provide enough identity, and the local engine has no egress or usage cost.
			// Choosing a connected service remains the explicit egress/cost decision.
			Key: "filler.transcribe.enabled", Label: "Listen for missing clip details", EnvVar: "FILLER_TRANSCRIBE_ENABLED", Group: GroupFiller,
			Kind: KindBool, Default: true, Advanced: true,
			Doc:    "Listen to clips with little description for spoken brand and product names.",
			Detail: "When a source provides very little information, listen for spoken brand and product details. Loomarr skips this for clips that already have enough context. Choose the speech service under Settings → AI.",
		},
		{
			// Vision tagging (§10 V44). ⚠ OFF by default AND gated on a vision-capable LLM: the
			// hosted path spends multimodal tokens per clip and sends frames off the box, the local
			// path needs an Ollama vision model. Off, or with no vision model, the job is inert.
			Key: "filler.vision.enabled", Label: "Inspect unclear clips with vision AI", EnvVar: "FILLER_VISION_ENABLED", Group: GroupFiller,
			Kind: KindBool, Default: false, Advanced: true,
			Doc:    "Look at clips Loomarr still can't identify to read logos and on-screen text.",
			Detail: "Look at a few frames of clips Loomarr still can't identify — reading on-screen logos and text — to work out the brand, even for clips with no speech. Needs a vision-capable AI model.",
		},
		{
			// ⚠ Its OWN model knob, exactly like filler.language_model — and the live test that
			// added it found why: the tagging model (`llm.model`) is often a TEXT model with no
			// vision path (qwen3 in dev), while the box has a separate vision-capable one (gemma-4).
			// Tying vision to `llm.model` would force an operator to switch their whole LLM to a
			// vision model just to tag clips. Empty ⇒ fall back to `llm.model`, so an install whose
			// main model already sees images needs no second setting.
			Key: "filler.vision.model", Label: "Vision model", EnvVar: "FILLER_VISION_MODEL", Group: GroupFiller,
			Kind: KindString, Default: "", Advanced: true,
			Doc:    "Which AI model looks at clip pictures, or empty to use your main model.",
			Detail: "Which AI model reads clip frames (must be vision-capable). Leave empty to reuse your main model — set it only when that model can't see images.",
		},
		{
			// ⚠ **The model knob above was only half a separation, and the half it was missing is
			// the one that fails silently.** `filler.vision.model` let an operator name a vision
			// model, but the provider was still built from `llm.provider`/`llm.url`/`llm.api_key`
			// — so naming a LOCAL model while the main LLM was hosted sent an Ollama tag to the
			// hosted endpoint. Measured: `llava:7b` → `https://openrouter.ai/api/v1` → 401 on every
			// segment, which `ground` reports as zero looks and the gate refuses as "a segment
			// could not be classified". The model name is useless without the host that serves it.
			//
			// Empty ⇒ inherit the main provider.
			// ⚠ `inherit` is a REAL value, not an empty string: the registry invariant requires
			// every enum option to carry a value and a label, and an explicit word reads better in
			// the picker than a blank row. The resolver treats "" the same way, so an env var set
			// to empty means inherit rather than "no provider".
			Key: "filler.vision.provider", Label: "Vision service", EnvVar: "FILLER_VISION_PROVIDER", Group: GroupFiller,
			Kind: KindEnum, Default: "inherit", Advanced: true,
			Enum: []EnumOption{
				opt("inherit", "Same as your main AI"),
				opt("ollama", "Ollama"),
				opt("openai", "OpenAI-compatible"),
			},
			Doc:    "Which AI service looks at clip pictures.",
			Detail: "Leave as “same as your main AI” unless your vision model lives somewhere else — a local Ollama, say, while your main AI is a hosted service.",
		},
		{
			// Empty + `ollama` resolves to the conventional local host, the same rule `ollamaBase`
			// already applies to probes and pulls — so the common case (hosted text, local vision)
			// needs the provider knob alone.
			Key: "filler.vision.url", Label: "Vision service address", EnvVar: "FILLER_VISION_URL", Group: GroupFiller,
			Kind: KindURL, Default: "", Advanced: true,
			Doc:      "The address of that picture service.",
			Detail:   "Leave empty for a local Ollama on this machine.",
			ShowWhen: map[string][]string{"filler.vision.provider": {"ollama", "openai"}},
		},
		{
			// ⚠ **Never falls back to `llm.api_key`, and that is the point rather than an
			// omission.** Declaring a separate vision service means declaring its own credentials:
			// inheriting would send the operator's hosted key to whatever host they just named,
			// including `localhost`. A local Ollama needs no key, so the common case leaves this
			// empty and nothing is sent.
			Key: "filler.vision.api_key", Label: "Vision service API key", EnvVar: "FILLER_VISION_API_KEY", Group: GroupFiller,
			Kind: KindSecret, Default: "", Advanced: true,
			Doc:      "The API key for that picture service, if it needs one.",
			Detail:   "Your main AI's key is never reused here. Never shown again after saving.",
			ShowWhen: map[string][]string{"filler.vision.provider": {"openai"}},
		},
		{
			// The defect floor's judge (#2034) reuses the vision model above. Each look is reserved
			// at this ceiling before the call and settled at what it cost; an operator-run endpoint
			// reserves nothing. Whole cents, because a setting has no decimal kind.
			Key: "filler.defects.judge_look_cents", Label: "Most one picture-quality check may cost (US cents)", EnvVar: "FILLER_DEFECTS_JUDGE_LOOK_CENTS", Group: GroupFiller,
			Kind: KindInt, Default: 2, Advanced: true,
			Doc:    "The most one picture-quality check on a paid AI service may cost.",
			Detail: "Ads whose picture is close to the quality limit are shown to the vision model above for a ruling. This is the most one look may cost on a hosted model. 0 stops hosted looks, and those ads wait.",
		},
		{
			// ⚠ The ledger's day is shared: it sums every metered filler model call that day, not
			// only the judge's, so the doc says "paid filler AI" rather than "these checks".
			Key: "filler.defects.judge_daily_cents", Label: "Daily ceiling for picture-quality checks (US cents)", EnvVar: "FILLER_DEFECTS_JUDGE_DAILY_CENTS", Group: GroupFiller,
			Kind: KindInt, Default: 50, Advanced: true,
			Doc:    "The most picture-quality checks on a paid AI service may cost in one day.",
			Detail: "Picture-quality checks on a hosted model stop for the day (UTC) once Loomarr's paid filler AI spend that day would pass this. Ads past the limit wait for the next day.",
		},

		// Automatic compilation splitting (§10 V43). Detection ran only on a button press and
		// its result always required a human, which made compilations the most manual part of a
		// system whose claim is that it maintains itself. Cut acceptance still creates held
		// children; it does not grant any child permission to air.
		{
			// Splitting has no sweep cadence of its own: it is a rung every long recording reaches
			// as it is ingested, and detection is bounded by `filler.pipeline.max_splits` per pass.
			//
			// ⚠ **ON by default (V51b).** Cutting is destructive in a way tagging is not — a
			// mis-cut clip plays HALF AN ADVERT and the source is consumed either way — and that
			// risk has not changed; what makes ON safe is the evidence. The gate is
			// strict — after known duplicates and short fragments are removed, the remaining reel
			// qualifies as a whole or none of it does; an ungrounded era disqualifies at every
			// threshold, and any segment the detector admits it could not resolve sends the
			// remaining reel to a human — and the measured failure mode is the gate REFUSING good
			// reels, not admitting bad ones. Off by default meant every compilation waited for a
			// click that the design says should be unnecessary.
			Key: "filler.autosplit.enabled", Label: "Accept confident cuts automatically", EnvVar: "FILLER_AUTOSPLIT_ENABLED", Group: GroupFiller,
			Kind: KindBool, Default: true,
			Doc:    "Cut up recordings of several ads without asking when Loomarr is sure of every cut.",
			Detail: "Accept the cuts automatically when Loomarr is confident about every one of them. Anything less certain still waits for you under Filler → Incoming.",
		},
		{
			// This threshold governs cut acceptance only; it has no catalog-publication effect.
			Key: "filler.autosplit.min_confidence", Label: "Confidence required to accept cuts", EnvVar: "FILLER_AUTOSPLIT_MIN_CONFIDENCE",
			Group: GroupFiller, Kind: KindInt, Default: 85, Validate: autoSplitConfidenceRange,
			Doc:      "How sure Loomarr must be about every advert it found inside a recording before cutting it up without asking (50–95).",
			ShowWhen: map[string][]string{"filler.autosplit.enabled": {"true"}},
		},
		{
			// ⚠ ONE key doing two jobs on purpose. It selects which clips the split job even
			// looks at (longer than this ⇒ a compilation worth detecting) AND it is the ceiling
			// every segment must clear to auto-confirm. Two keys could disagree — a clip the job
			// considers too long to be an advert must not then auto-confirm as one.
			Key: "filler.autosplit.max_duration", Label: "Longest expected single clip", EnvVar: "FILLER_AUTOSPLIT_MAX_DURATION",
			Group: GroupFiller, Kind: KindDuration, Default: "120s",
			Doc:    "The longest a single advert is expected to be.",
			Detail: "Recordings longer than this are treated as compilations worth splitting, and any piece longer than this is one Loomarr will ask you about.",
		},
		{
			Key: "filler.structure_window_authority_path", Label: "Long-reel authority file", EnvVar: "FILLER_STRUCTURE_WINDOW_AUTHORITY_PATH", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Validate: storagePath(true), Advanced: true,
			Doc:      "A reviewed file that allows certain long recordings to be split, for advanced installs.",
			Detail:   "Optional absolute path to a separately reviewed long-reel materialization authority. Empty or invalid evidence enables no certified slice; a valid authority may create held children but cannot make them airable. A change applies to the next split; a split already running finishes under the files it started with.",
			ShowWhen: map[string][]string{"filler.autosplit.enabled": {"true"}},
		},
		{
			Key: "filler.structure_window_deployment_path", Label: "Long-reel deployment file", EnvVar: "FILLER_STRUCTURE_WINDOW_DEPLOYMENT_PATH", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Validate: storagePath(true), Advanced: true,
			Doc:      "A reviewed file naming the paid AI route long-recording splits may use, for advanced installs.",
			Detail:   "Optional absolute path to the reviewed authority's OpenRouter route and spend deployment. Empty, invalid, or mismatched evidence performs no structure inference and enables no certified slice. A change applies to the next split; a split already running finishes under the files it started with.",
			ShowWhen: map[string][]string{"filler.autosplit.enabled": {"true"}},
		},
		// The ingest pipeline's per-run budget (§10 V51b). Every one of these bounds ONE PASS, not
		// the catalog: a backlog drains over cycles, which is the property the per-job batch
		// constants they replace (LanguageBatch 25, TranscribeBatch 10, VisionBatch 5,
		// defaultSplitsPerRun 3) were chosen to defend. The numbers are carried forward unchanged.
		//
		// ⚠ **Zero means NONE, and that is a distinct state from the default.** It is the only way
		// an operator can say "never do this kind of work on this box" — the same three-state
		// encoding `filler.fetch.every` uses, and the reason these are integers rather than
		// booleans-plus-a-rate.
		{
			Key: "filler.pipeline.max_clips", Label: "Clips prepared per pass", EnvVar: "FILLER_PIPELINE_MAX_CLIPS", Group: GroupFiller,
			Kind: KindInt, Default: 25, Advanced: true,
			Doc:    "How many new clips Loomarr prepares at a time.",
			Detail: "How many clips Loomarr advances through preparation in one pass. A large import drains over several passes rather than occupying the machine in one.",
		},
		{
			// ⚠ THREE, the tightest budget here, because a transcode competes with playout for
			// the GPU and this is what makes the existing catalog backfill converge over a day
			// instead of pinning the box. Zero switches re-encoding off entirely — an escape
			// hatch that matters, because this is the one rung that rewrites the operator's file.
			Key: "filler.transcode.max_per_run", Label: "Clips re-encoded per pass", EnvVar: "FILLER_TRANSCODE_MAX_PER_RUN", Group: GroupFiller,
			Kind: KindInt, Default: 3, Advanced: true,
			Doc:    "How many clips Loomarr converts to its standard format at a time.",
			Detail: "How many clips Loomarr re-encodes to its standard format in one pass. Set to 0 to never re-encode — clips then play in whatever format they arrived in.",
		},
		{
			Key: "filler.pipeline.max_whisper", Label: "Clips transcribed per pass", EnvVar: "FILLER_PIPELINE_MAX_WHISPER", Group: GroupFiller,
			Kind: KindInt, Default: 10, Advanced: true,
			Doc:    "How many clips Loomarr listens to at a time.",
			Detail: "How many clips Loomarr listens to in one pass, for language and transcription together. Listening is slow — minutes per clip on some machines — so this keeps a pass from running away.",
		},
		{
			Key: "filler.pipeline.max_vision", Label: "Clips inspected visually per pass", EnvVar: "FILLER_PIPELINE_MAX_VISION", Group: GroupFiller,
			Kind: KindInt, Default: 5, Advanced: true,
			Doc:    "How many clips Loomarr looks at with AI at a time.",
			Detail: "How many clips Loomarr looks at with a vision model in one pass. The smallest budget, because on a hosted model each one is a charge.",
		},
		{
			// ⚠ Counted in SEGMENTS, not clips — the only budget here that is, and §10 V51g is
			// why it must exist. A rung's cost may not scale unboundedly with a clip's content:
			// the live catalog holds proposals of 235, 222, 142 and 133 segments, so an unbounded
			// per-segment pass is exactly the shape that burned 377s against a 120s budget.
			//
			// A reel with more segments than this continues on the next pass. The proposal persists
			// Looked per segment, so this is a resource budget rather than a review threshold.
			Key: "filler.pipeline.max_split_vision", Label: "Compilation segments inspected per pass", EnvVar: "FILLER_PIPELINE_MAX_SPLIT_VISION", Group: GroupFiller,
			Kind: KindInt, Default: 60, Advanced: true,
			Doc:    "How many pieces of one long recording Loomarr looks at a time.",
			Detail: "How many segments of one recording Loomarr looks at in a single pass. A longer recording is judged over several passes rather than made to wait for you — this bounds how much looking happens at once, not how big a recording can be.",
		},
		{
			Key: "filler.pipeline.max_splits", Label: "Compilations split per pass", EnvVar: "FILLER_PIPELINE_MAX_SPLITS", Group: GroupFiller,
			Kind: KindInt, Default: 3, Advanced: true,
			Doc:    "How many long recordings Loomarr looks inside at a time.",
			Detail: "How many long recordings Loomarr looks inside in one pass. Finding the adverts in one recording takes minutes.",
		},
		// Auto-fetch and its limits (§10 V38b). A registered source is polled on a schedule, which
		// supersedes §15's "there is no unattended crawler" — the superseded rule's concern
		// survives as these bounds rather than as a prohibition.
		//
		// ⚠ Every one of them fails toward doing LESS. An operator who never opens this page gets
		// a trickle they can live with; the failure mode being designed against is "add a source,
		// wake up to 8,000 files".
		{
			Key: "filler.fetch.every", Label: "Look for new clips", EnvVar: "FILLER_FETCH_EVERY", Group: GroupFiller,
			Kind: KindDuration, Presentation: PresentationFillerDownloadSchedule, Default: "6h", Validate: fillerFetchInterval,
			Presets: []Preset{
				preset("0s", "Never"), preset("6h", "Every 6 hours"), preset("12h", "Every 12 hours"),
				preset("24h", "Daily"), preset("168h", "Weekly"),
			},
			Doc:    "How often Loomarr checks enabled sources for new clips.",
			Detail: "Never stops sources that use this default; you can still check one yourself.",
		},
		{
			Key: "filler.fetch.max_per_run", Label: "Add up to", EnvVar: "FILLER_FETCH_MAX_PER_RUN", Group: GroupFiller,
			Kind: KindInt, Default: 10, Validate: positiveLimit,
			Doc: "The most clips each enabled source may add in one automatic check.",
		},
		{
			// ⚠ OFF until automatic splitting is certified (#952, #963; #1773). Every filler item
			// waiting on a person was a compilation reel whose cut points needed confirming, and
			// review should be a last resort. Off holds back only NEW reels from sources and
			// planned pulls (a known runtime over filler.autosplit.max_duration, the same signal
			// probe uses); nothing already downloaded is dropped, and the drop folder is exempt.
			Key: "filler.acquisition.compilations", Label: "Download compilation reels", EnvVar: "FILLER_ACQUISITION_COMPILATIONS", Group: GroupFiller,
			Kind: KindBool, Default: false,
			Doc:    "Also download recordings of several adverts in one video.",
			Detail: "Off until Loomarr can split them without asking you; sources then add single adverts only. Your drop folder is never affected.",
		},
		{
			// ⚠ Bounds the UNATTENDED path only. An admin queueing a clip or approving a pull is
			// a deliberate act and is not stopped by this — a ceiling on what happens while
			// nobody is looking is not a ceiling on what someone chooses to do.
			Key: "filler.fetch.max_catalog_clips", Label: "Automatic-download catalog limit", EnvVar: "FILLER_FETCH_MAX_CATALOG_CLIPS", Group: GroupFiller,
			Kind: KindInt, Default: 2000, Advanced: true, Validate: positiveLimit,
			Doc:    "Stop fetching automatically once your catalog reaches this many clips.",
			Detail: "You can still add more by hand.",
		},
		{
			Key: "filler.storage.library_budget_gb", Label: "Filler storage allowance", EnvVar: "FILLER_STORAGE_LIBRARY_BUDGET_GB", Group: GroupFiller,
			Kind: KindInt, Unit: UnitGiB, Default: 0, Validate: nonNegativeWholeNumber,
			Doc:    "How much disk space Loomarr may use for filler, or 0 to choose a safe amount.",
			Detail: "In GiB. Leave at 0 to choose a safe allowance automatically. Loomarr always keeps a protected amount of free space for the rest of the system.",
		},
		{
			Key: "filler.research.enabled", Label: "Find missing details automatically", EnvVar: "FILLER_RESEARCH_ENABLED", Group: GroupFiller,
			Kind: KindBool, Presentation: PresentationSwitch, Default: true,
			Doc:    "Look up a clip's likely time period and place in public sources.",
			Detail: "This does not affect whether the clip can play.",
		},
		{
			Key: "filler.research.wikidata_enabled", Label: "Wikidata", EnvVar: "FILLER_RESEARCH_WIKIDATA_ENABLED", Group: GroupFiller,
			Kind: KindBool, Presentation: PresentationSwitch, Default: true, Advanced: true,
			Doc: "Search Wikidata for public facts that may help identify a clip's era or location.",
		},
		{
			Key: "filler.research.wikipedia_enabled", Label: "Wikipedia", EnvVar: "FILLER_RESEARCH_WIKIPEDIA_ENABLED", Group: GroupFiller,
			Kind: KindBool, Presentation: PresentationSwitch, Default: true, Advanced: true,
			Doc: "Search Wikipedia for public background that may help identify a clip's era or location.",
		},
		{
			Key: "filler.research.archive_enabled", Label: "Archive.org", EnvVar: "FILLER_RESEARCH_ARCHIVE_ENABLED", Group: GroupFiller,
			Kind: KindBool, Presentation: PresentationSwitch, Default: true, Advanced: true,
			Doc:    "Search the Internet Archive's public catalog for related historical items.",
			Detail: "Searches Archive.org's public catalog for related historical items and metadata.",
		},
		{
			Key: "filler.research.loc_enabled", Label: "Library of Congress", EnvVar: "FILLER_RESEARCH_LOC_ENABLED", Group: GroupFiller,
			Kind: KindBool, Presentation: PresentationSwitch, Default: true, Advanced: true,
			Doc: "Search the Library of Congress public catalog for related historical records.",
		},
		{
			Key: "filler.research.web_provider", Label: "Web search provider", EnvVar: "FILLER_RESEARCH_WEB_PROVIDER", Group: GroupFiller,
			Kind: KindEnum, Default: "none", Advanced: true,
			Enum:   []EnumOption{opt("none", "Not configured"), opt("brave", "Brave Search"), opt("searxng", "SearXNG (self-hosted)")},
			Doc:    "A general web search Loomarr may try when public sources can't identify a clip.",
			Detail: "Optional last-resort search after trusted public sources cannot identify a clip. Search is automatic and never decides safety, readiness, or scheduling.",
		},
		{
			Key: "filler.research.brave_api_key", Label: "Brave Search API key", EnvVar: "FILLER_RESEARCH_BRAVE_API_KEY", Group: GroupFiller,
			Kind: KindSecret, Default: "", Advanced: true,
			ShowWhen: map[string][]string{"filler.research.web_provider": {"brave"}},
			Doc:      "Your Brave Search API key.",
			Detail:   "Installation-wide Brave Search key. Loomarr sends the public clip title and search terms, never the media file. The key is encrypted and never shown again.",
		},
		{
			Key: "filler.research.searxng_url", Label: "SearXNG address", EnvVar: "FILLER_RESEARCH_SEARXNG_URL", Group: GroupFiller,
			Kind: KindURL, Default: "", Advanced: true, Validate: credentialFreeHTTPSEndpoint,
			ShowWhen: map[string][]string{"filler.research.web_provider": {"searxng"}},
			Doc:      "The address of your SearXNG search server.",
			Detail:   "HTTPS address of your SearXNG server. Loomarr requests its bounded JSON search API and does not fetch result pages.",
		},
		{
			Key: "filler.research.monthly_limit", Label: "Monthly web searches", EnvVar: "FILLER_RESEARCH_MONTHLY_LIMIT", Group: GroupFiller,
			Kind: KindInt, Default: 100, Advanced: true, Validate: positiveWholeNumber,
			Doc:    "The most web searches Loomarr may make in a month.",
			Detail: "Most general-web searches Loomarr may make in one UTC calendar month. Trusted public-source lookups do not count toward this limit.",
		},
		{
			Key: "filler.incoming.ready_window", Label: "Keep ready clips in Incoming", EnvVar: "FILLER_INCOMING_READY_WINDOW", Group: GroupFiller,
			Kind: KindDuration, Default: "24h", Advanced: true, Validate: fillerIncomingReadyWindow,
			Doc:    "How long clips stay under Ready in Incoming.",
			Detail: "They remain in your Library after they leave this view.",
		},
		{
			Key: "filler.breaks_per_hour", Label: "Breaks per program hour", EnvVar: "FILLER_BREAKS_PER_HOUR", Group: GroupFiller,
			Kind: KindInt, Default: 4, Validate: nonNegativeWholeNumber,
			Doc:    "How many commercial breaks an hour of programmes gets, unless a channel sets its own.",
			Detail: "Default commercial-break frequency for channels that follow it. Set 0 to disable breaks by default; each channel can choose its own frequency.",
		},
		{
			Key: "filler.break_duration", Label: "Length of each break", EnvVar: "FILLER_BREAK_DURATION", Group: GroupFiller,
			Kind: KindDuration, Default: "2m", Validate: breakDuration,
			Doc:    "How long each commercial break lasts, unless a channel sets its own.",
			Detail: "How long each commercial break lasts by default, with the commercials rotated so recently shown ones wait their turn. Channels can choose their own length. Use breaks per program hour to turn breaks off.",
		},
		{
			Key: "filler.pod_max", Label: "Clips per break", EnvVar: "FILLER_POD_MAX", Group: GroupFiller,
			Kind: KindInt, Default: 4, Validate: positiveWholeNumber,
			Doc:    "How many clips a break usually holds.",
			Detail: "Preferred clip count per break. Loomarr automatically exceeds it when shorter clips need more slots to fill the requested break length.",
		},
		{
			Key: "filler.cooldown_seconds", Label: "Repeat spacing · same commercial", EnvVar: "FILLER_COOLDOWN_SECONDS", Group: GroupFiller,
			Kind: KindInt, Unit: UnitSeconds, Default: 14400, Advanced: true, Validate: nonNegativeWholeNumber,
			Doc:    "How long before a commercial you've seen can air again on any channel.",
			Detail: "Default seconds before a commercial seen on any channel airs again (14400 is 4 hours). Only watched airings count. Loomarr first reaches for commercials from a nearby era, and repeats one sooner only when that would otherwise leave a break short. Each channel can choose its own spacing.",
		},
		// ⚠ Default 0 = OFF, and that is the whole safety property of this knob (V17c).
		// `00014_clips_quality` shipped with quality as display-only and warned that a
		// well-meaning "prefer HD" would quietly starve the era-accurate 4:3 commercials the
		// feature exists to play. That warning holds — which is why an install that sets
		// nothing selects no quality floor at all, pinned by a test.
		//
		// Advanced: an operator who does not know what 240p looks like in a break should never
		// meet this, and one who does will go looking.
		{
			Key: "filler.min_quality", Label: "Minimum picture height (px)", EnvVar: "FILLER_MIN_QUALITY", Group: GroupFiller,
			Kind: KindInt, Default: 0, Advanced: true,
			Doc:    "The lowest picture size a commercial may have and still air, or 0 for any.",
			Detail: "Minimum clip height in pixels for a commercial to be eligible (480 excludes 240p rips). 0 disables the floor, which is the default — era accuracy beats resolution.",
		},
		{
			Key: "filler.weight", Label: "Selection weight", EnvVar: "FILLER_WEIGHT", Group: GroupFiller,
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
			Key: "filler.min_duration", Label: "Reject files shorter than", EnvVar: "FILLER_MIN_DURATION", Group: GroupFiller,
			Kind: KindDuration, Default: "10s", Advanced: true,
			Doc:    "Files shorter than this are turned away as broken downloads.",
			Detail: "Clips shorter than this are rejected on sight and never enter the catalog — a truncated download is not a short commercial. Set to 0s to accept anything with a readable duration.",
		},
		{
			// ⚠ **The only setting in Loomarr that deletes an operator's media**, which is why its
			// doc says so in the operator's own words rather than in ours. Everything else here
			// tombstones: "remove from catalog" keeps the file, disabling a source keeps its clips.
			//
			// It exists because partial confirm (§10 V54) leaves a residue BY DESIGN — every reel
			// files its confident cuts and keeps the doubtful ones back, so small proposals
			// accumulate and each one pins a 1–2 GB recording. Without an expiry the feature that
			// shrinks the operator's work grows their storage instead.
			//
			// ⚠ 0s is OFF, and that is the three-state encoding the rest of §10 uses: an operator
			// who has not chosen an expiry has not agreed to have recordings deleted. A reel that
			// produced NO clips is never eligible at any window — it is the only copy of that
			// content, and reaping it would destroy material Loomarr never managed to use.
			Key: "filler.split.review_window", Label: "Keep unreviewed cuts for", EnvVar: "FILLER_SPLIT_REVIEW_WINDOW", Group: GroupFiller,
			Kind: KindDuration, Default: "720h", Advanced: true,
			Doc:    "How long cuts you haven't reviewed wait before Loomarr gives up on them.",
			Detail: "When the time is up it drops the leftover cuts and DELETES the original recording to reclaim the space — but only for recordings that already produced clips, so nothing is lost that was never used. The clips themselves are never touched. Set to 0s to keep everything forever.",
		},
		{
			// ⚠ **ELIGIBILITY, not a reject — the pair to `filler.min_duration` above, and the
			// distinction is which side of the catalog boundary they sit on.** `min_duration`
			// refuses a file entry; these two decide whether a clip already IN the catalog may
			// fill a break. A clip outside them stays filed, searchable and pinnable; it simply
			// is not drawn automatically.
			//
			// ⚠ **`Policy.MinClipMs`/`MaxClipMs` existed for phases with no way to set them.**
			// They were assigned in tests and nowhere else — no key, no env var, no policy field —
			// so `durationEligible` always returned true and `PoolReport.Eligible`, which
			// `coverage.go` headlines as "the number that surprises operators", was arithmetically
			// identical to `Commercials` on every install. The pool strip showed one number twice
			// and called the pair a diagnosis. Wiring them is what makes that strip mean something.
			//
			// Both default to 0 = OFF, so no existing install changes behaviour on upgrade.
			Key: "filler.min_clip_duration", Label: "Shortest clip used automatically", EnvVar: "FILLER_MIN_CLIP_DURATION", Group: GroupFiller,
			Kind: KindDuration, Default: "0s", Advanced: true,
			Doc:    "Commercials shorter than this are not picked for breaks automatically.",
			Detail: "0s disables the floor. Unlike the minimum clip length above, this never rejects a clip — it stays in the catalog and can still be pinned to a channel.",
		},
		{
			Key: "filler.max_clip_duration", Label: "Longest clip used automatically", EnvVar: "FILLER_MAX_CLIP_DURATION", Group: GroupFiller,
			Kind: KindDuration, Default: "0s", Advanced: true,
			Doc:    "Commercials longer than this are not picked for breaks automatically.",
			Detail: "This is the guard against a three-minute infomercial filling a thirty-second gap. 0s disables the ceiling. Never rejects a clip; it stays in the catalog and can still be pinned.",
		},
		{
			// ⚠ Applied in the PLAYOUT chain, never written back to the file. The drop-folder
			// holds the operator's own files; in-place normalisation is destructive, unrepeatable,
			// and a re-scan cannot tell it already happened (§10 V40, §9.1).
			//
			// Measured spread across real fetched clips: -21.8 to -32.6 LUFS, ~11 dB of
			// clip-to-clip jump. -23 is the broadcast target.
			Key: "filler.target_lufs", Label: "Playback loudness target (LUFS)", EnvVar: "FILLER_TARGET_LUFS", Group: GroupFiller,
			Kind: KindString, Default: "-23", Advanced: true,
			Doc:    "How loud every filler clip plays, so breaks don't jump in volume.",
			Detail: "Loudness every filler clip is normalised to, in LUFS (-23 is the broadcast standard). Clips are measured at ingest; at playout a clip that misses the target gets a small constant gain (never a live filter, so no volume swell at clip starts). Empty disables it and clips play at whatever level they were recorded.",
		},
		{
			// ⚠ A clip with NO speech is always kept — a wordless visual spot has no language, and
			// those are often the best filler. Only confident non-target speech rejects (§10 V40).
			Key: "filler.language", Label: "Commercial language", EnvVar: "FILLER_LANGUAGE", Group: GroupFiller,
			Kind: KindEnum, Enum: fillerLanguageOptions(), Presentation: PresentationLanguage, Default: "en",
			Doc:    "The language commercials are in, unless a channel sets its own.",
			Detail: "Every channel's commercial language unless the channel sets its own. New clips are kept when their speech is in a language some channel uses and rejected when it is confidently in none of them; a clip with no speech at all is always kept. A clip whose language could not be determined airs only on channels that use this language.",
		},
		// --- Filler ingest (§10, §15) ---
		// ⚠ The vendored binaries ship in the SINGLE image (§16), so these paths are overrides
		// for an unusual layout, not an opt-in.
		// The two tool paths are what the `ingest` feature gate probes. They are
		// settings rather than hardcoded so an operator can point at a NEWER yt-dlp
		// than the image ships — yt-dlp releases fixes far faster than we cut images,
		// and a stale one silently stops extracting from YouTube.
		{
			Key: "ingest.ytdlp_path", Label: "yt-dlp executable", EnvVar: "INGEST_YTDLP_PATH", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Advanced: true,
			Doc:    "Where the yt-dlp download program lives.",
			Detail: "The Loomarr image sets this; empty means clip downloading is off.",
		},
		{
			Key: "ingest.ffmpeg_path", Label: "ffmpeg executable", EnvVar: "INGEST_FFMPEG_PATH", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Advanced: true,
			Doc: "Where the ffmpeg program lives (yt-dlp needs it to combine video and audio).",
		},
		{
			Key: "ingest.timeout", Label: "Download timeout", EnvVar: "INGEST_TIMEOUT", Group: GroupFiller,
			Kind: KindDuration, Default: "30m", Advanced: true,
			Doc: "How long one download may run before it's stopped, so a stuck fetch can't block others.",
		},
		// Compilation splitting (§10, V34). Empty/unrunnable ⇒ the transcript-rescue
		// step is unavailable and over-long segments surface as UNSPLITTABLE in the
		// review rather than being guessed at — coarse splitting needs only ffmpeg.
		{
			Key: "ingest.whisper_path", Label: "whisper executable", EnvVar: "INGEST_WHISPER_PATH", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Advanced: true,
			Doc:    "Where the whisper-cli speech program lives.",
			Detail: "The image sets this; empty means over-long compilation segments can't be transcribed for hidden ad breaks.",
		},
		{
			Key: "ingest.whisper_model", Label: "Transcription model file", EnvVar: "INGEST_WHISPER_MODEL", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Advanced: true,
			Doc:    "The speech model file used to transcribe long recordings.",
			Detail: "The whisper model file whisper-cli transcribes with. Size is a correctness property, not a quality preference — too small drops audio and the boundary detector then invents breaks.",
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
			Key: "filler.language_model", Label: "Language detection model file", EnvVar: "FILLER_LANGUAGE_MODEL", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Advanced: true,
			Doc:    "The model file used to work out what language a clip is in.",
			Detail: "Must be a MULTILINGUAL whisper model — an English-only one reports every clip as English, so the check would never reject anything. The image ships one; leave empty to turn local detection off.",
		},
	}
}
