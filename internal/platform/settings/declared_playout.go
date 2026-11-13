package settings

// declaredPlayout is playout and the guide, then backup and the image service.
func declaredPlayout() []Setting {
	return []Setting{
		// --- Playout (§9.1, §15 — added by V4) ---
		// Loomarr serves its own channels. `playout.backend` is the ONLY key here with a
		// per-channel override: it rides `policy_json` as `policy.playout` (nil = inherit
		// this global), the same shape `rules`/`filler`/`window`/`autoCurate` already use,
		// so there is no migration. Inheriting channels resolve this value live, so changing
		// the default moves that fleet; a channel with an explicit policy remains pinned to
		// its selected backend.
		{
			Key: "playout.backend", Label: "Playback engine", EnvVar: "PLAYOUT_BACKEND", Group: GroupPlayout,
			Kind: KindEnum, Enum: []EnumOption{
				opt("internal", "Loomarr (internal)"),
				opt("tunarr", "Tunarr"),
			},
			Default: "internal",
			Doc:     "Whether Loomarr streams your channels itself or hands them to Tunarr.",
			Detail:  "Who streams a channel. Internal playout is the default: it supports mid-roll breaks, reports real transcode telemetry, and needs no extra service. Tunarr is an optional backend for an install that already runs Tunarr — not a requirement. An unrecognised value falls back to internal. Overridable per channel.",
		},
		{
			Key: "playout.encoder", Label: "Encoder override", EnvVar: "PLAYOUT_ENCODER", Group: GroupPlayout,
			Kind: KindString, Default: "", Advanced: true,
			Doc:    "Forces a particular video encoder instead of the one Loomarr picked.",
			Detail: "ffmpeg encoder for internal playout (e.g. libx264, h264_vaapi, h264_nvenc). Empty = pick the best one the transcode check found. Set it only to override that choice.",
		},
		{
			Key: "playout.audio_language", Label: "Preferred audio language", EnvVar: "PLAYOUT_AUDIO_LANGUAGE", Group: GroupPlayout,
			Kind: KindString, Presentation: PresentationLanguage, Presets: audioLanguagePresets(), Default: "en",
			Doc:    "Which audio language channels play when a film has more than one.",
			Detail: "Preferred audio language for internal playout. Any ISO 639 code saves (en, eng and fra all work). A preference, not a requirement: a film with no track in this language plays its first track rather than failing. Empty = play whichever track comes first in the file, which is how a foreign-language dub ends up playing instead of the original. A channel can override this on its Channel info page.",
		},
		{
			Key: "playout.quality_tier", Label: "Maximum playback quality", EnvVar: "PLAYOUT_QUALITY_TIER", Group: GroupPlayout,
			Kind: KindEnum, Enum: []EnumOption{
				opt("efficient", "Efficient — 720p, lowest bandwidth"),
				opt("balanced", "Balanced — 1080p"),
				opt("quality", "Quality — 1080p, best picture"),
			},
			Default: "balanced",
			Doc:     "The best picture quality channels aim for, traded against bandwidth.",
			Detail:  "The picture-versus-bandwidth target. Efficient is 720p and roughly half the bitrate — the right answer for a NAS running several channels, or for watching away from home. Balanced is 1080p and the default. Quality is 1080p at a higher frame rate and bitrate, which on grainy or dark film can be visibly cleaner but costs noticeably more bandwidth per channel. Whichever you pick, playout still steps down automatically as more channels start, so the choice is a ceiling rather than a promise.",
		},
		{
			// One curve on every family (G11, #1512); playout.ToneCurve maps it to each tone-mapper.
			// Read once when a stream starts and pinned for its life, so a change never alters the
			// look mid-stream. Costs are the household Arc's, per HDR stream at 1080p (#1516).
			Key: "playout.tone_curve", Label: "HDR tone curve", EnvVar: "PLAYOUT_TONE_CURVE", Group: GroupPlayout,
			Kind: KindEnum, Enum: []EnumOption{
				opt("hable", "Hable (filmic) — default"),
				opt("mobius", "Mobius"),
				opt("reinhard", "Reinhard"),
				opt("bt2390", "BT.2390 — more CPU on Intel"),
				opt("bt2446a", "BT.2446 Method A — more CPU on Intel"),
				opt("spline", "Spline — more CPU on Intel"),
			},
			Default: "hable",
			Doc:     "How HDR films are adjusted to look right on an ordinary screen.",
			Detail: "How an HDR film is turned into the SDR picture a channel broadcasts. Hable (the default) rolls bright highlights off gently, like film; Mobius and Reinhard keep more of the mid-tones and highlights. These three run on the GPU on Intel and NVIDIA, about 0.1 CPU cores per HDR stream on Intel. " +
				"BT.2390, BT.2446 Method A and Spline come from libplacebo. On Intel the picture makes a trip through system memory for them, which roughly doubles the CPU cost to about 0.25 cores per HDR stream. Where libplacebo cannot run (no Vulkan GPU, for example an NVIDIA container without NVIDIA's Vulkan driver), those three use the CPU tone-mapper's Mobius curve instead. " +
				"Without a GPU tone-mapper at all, every curve runs on the CPU after the GPU has scaled the picture down; the picture is correct but costs more CPU. A change applies to streams that start afterwards; a channel already playing keeps its curve until it restarts.",
		},
		{
			// The middle tier of channel policy.watermark.opacity > this > built-in (#1617); the
			// maintainer chose 40% from a real 1080p preview. Read per programme item, and the
			// rendered-bug cache is keyed by opacity, so a change re-renders the bugs.
			Key: "playout.watermark_opacity_pct", Label: "On-screen logo opacity", EnvVar: "PLAYOUT_WATERMARK_OPACITY_PCT", Group: GroupPlayout,
			Kind: KindInt, Unit: UnitPercent, Default: "40", Validate: watermarkOpacityRange,
			Doc:    "How solid the on-screen logo in the corner of every programme looks, from 10 (faint) to 100 (fully opaque).",
			Detail: "A channel with its own opacity keeps it. A change shows from each channel's next programme.",
		},
		{
			// The maintainer's four styles (#1617, picked from real-frame previews), Text by default,
			// then the approved design's six (#1659); the middle tier of channel
			// policy.watermark.look > this > built-in. Values match schedule.WatermarkLooks and
			// watermark.DesignStyles (pinned by an internal/app test). Read per programme item; the
			// rendered-bug cache is keyed by look, so a change re-renders.
			Key: "playout.watermark_look", Label: "On-screen logo style", EnvVar: "PLAYOUT_WATERMARK_LOOK", Group: GroupPlayout,
			Kind: KindEnum, Enum: []EnumOption{
				opt("plate", "Plate — the name cut out of a rounded badge"),
				opt("text", "Text — the name alone, with a soft shadow"),
				opt("outline", "Outline — the name as hollow letters"),
				opt("small-plate", "Small plate — a smaller badge with no shadow"),
				opt("tint", "Colour plate — the name cut out of a coloured badge"),
				opt("twotone", "Number + name — the channel number in a coloured tab beside the name"),
				opt("badge", "Number badge — the channel number in a coloured rounded square"),
				opt("lockup", "Number | name — number, thin line, name, in white"),
				opt("glass", "Glass — the name on a frosted see-through plate"),
				opt("stripe", "Era stripe — the name over a three-colour stripe"),
			},
			Default: "text",
			Doc:     "How the on-screen logo Loomarr draws from a channel's name looks.",
			Detail:  "Coloured styles use each channel's colour (by default, colours from the channel's decade). A channel with its own style keeps it, and an uploaded logo keeps its own shape. A change shows from each channel's next programme.",
		},
		{
			// ⚠ Separate from ingest.ffmpeg_path because these two callers fail differently.
			// Playout's ffmpeg is a runtime dependency of a channel that is ON AIR, while
			// ingest's is a build dependency of a download nobody is watching — so an operator pointing ingest at
			// a newer yt-dlp-compatible ffmpeg must not be able to break playout by doing it.
			Key: "playout.ffmpeg_path", Label: "FFmpeg path", EnvVar: "PLAYOUT_FFMPEG_PATH", Group: GroupPlayout,
			Kind: KindString, Presentation: PresentationPath, Default: "ffmpeg", Advanced: true,
			Doc:    "Where the ffmpeg program lives.",
			Detail: "The default works whenever ffmpeg is on the system PATH; set it only if yours is somewhere unusual.",
		},
		{
			// Where the in-app HLS player's segments are written (§9.1 Watch, V46). Disk-backed
			// beside the database by default, NOT the OS temp dir (#1512 spike requirement 8):
			// the channel packager keeps a DVR window per watched channel (~8 Mbit/s × 15 min,
			// about 1 GB), which a RAM-backed /tmp would hold in memory. Each process removes its
			// own scratch root when it stops and sweeps a crashed predecessor's at start.
			// Advanced: a wrong value degrades in-app playback, never the media-server streams
			// (those never touch this dir).
			Key: "playout.hls_dir", Label: "Browser playback cache", EnvVar: "PLAYOUT_HLS_DIR", Group: GroupPlayout,
			Kind: KindString, Presentation: PresentationPath, DataSubdir: "hls", Advanced: true,
			Doc:    "Where watching in the browser keeps a few minutes of temporary video.",
			Detail: "Directory where in-app browser playback writes its temporary HLS segments. Defaults to a directory beside the database (/data/hls in the container). Each channel being watched keeps up to 15 minutes of video here (about 1 GB), deleted when you stop watching, so use a disk, not a RAM-backed tmpfs like /dev/shm. Only affects in-app playback; your media server's streams never use it. Changing it takes effect after restart.",
		},
		{
			// Host measurements get their own small root: the capability evidence, the per-class cost
			// table and the synthetic probe clips. Read once at composition.
			Key: "playout.state_dir", Label: "Playout host measurements", EnvVar: "PLAYOUT_STATE_DIR", Group: GroupPlayout,
			Kind: KindString, Presentation: PresentationPath, DataSubdir: "playout-state", Advanced: true,
			Doc:    "Where Loomarr keeps what it learned about this server's video power.",
			Detail: "Where Loomarr keeps what it measured about this server's transcoding: the chosen encoder, the cost of each kind of stream and a few seconds of generated test video (about 20 MB). Defaults to a directory beside the database (/data in the container). Deleting it makes Loomarr measure again at the next start. Changing it takes effect after restart.",
		},
		{
			Key: "playout.max_channels", Label: "Live transcode safety cap", EnvVar: "PLAYOUT_MAX_CHANNELS", Group: GroupPlayout,
			Kind: KindInt, Default: "0", Validate: nonNegativeWholeNumber,
			Doc:    "The most channels Loomarr may stream at the same time, or 0 to decide for itself.",
			Detail: "Optional safety cap for simultaneous internal transcodes. Leave at 0 for Loomarr to use measured capacity automatically. A positive value can lower that measurement but cannot raise it.",
		},
		{
			// Host memory is a per-lease gate on hardware encodes, not a capacity term (§9.1): each
			// encoder's device context and pipeline hold ~0.5–1 GiB of host RAM. Hot-applied because
			// the ResourceBudget re-reads it on every admission.
			Key: "playout.memory_reserve_mb", Label: "Host memory reserve for encodes", EnvVar: "PLAYOUT_MEMORY_RESERVE_MB", Group: GroupPlayout,
			Kind: KindInt, Unit: UnitMiB, Default: "2048", Advanced: true, Validate: nonNegativeWholeNumber,
			Doc:    "Memory that channel streaming always leaves free for the rest of the machine.",
			Detail: "Host memory, in MiB, that hardware encoding must leave available. Loomarr does not start a hardware transcode that would take free memory below this. Set 0 to disable the host-memory check. Changes apply to the next encode without restart.",
		},
		{
			Key: "playout.encode_memory_mb", Label: "Host memory per encode", EnvVar: "PLAYOUT_ENCODE_MEMORY_MB", Group: GroupPlayout,
			Kind: KindInt, Unit: UnitMiB, Default: "0", Advanced: true, Validate: nonNegativeWholeNumber,
			Doc:    "How much memory one streaming channel is expected to use, or 0 to work it out.",
			Detail: "Host memory, in MiB, one hardware encode is expected to use. Leave at 0 to use 1024 MiB, or more if the encoder capability check measured more on this machine; set it when real encodes here use more, for example 4K or HDR tone mapping. Changes apply to the next encode without restart.",
		},
		{
			// The ResourceBudget's CPU term (#1512 G5): total playout CPU at full GPU capacity.
			// Hot-applied because the budget re-reads its facts on every admission.
			Key: "playout.gpu_cpu_millicores", Label: "Playout CPU on GPU hosts", EnvVar: "PLAYOUT_GPU_CPU_MILLICORES", Group: GroupPlayout,
			Kind: KindInt, Default: "1000", Advanced: true, Validate: positiveWholeNumber,
			Doc:    "How much processor time all channels together may use when a graphics card does the encoding.",
			Detail: "On a host that encodes on its GPU, the CPU all live channels together may use, in thousandths of a core (1000 = one core). Loomarr admits no more channels than fit this allowance at each channel's measured CPU cost. Never more than the container's CPU limit. Changes apply to the next channel start without restart.",
		},
		{
			Key: "playout.app_reserve_millicores", Label: "CPU kept for the app on software hosts", EnvVar: "PLAYOUT_APP_RESERVE_MILLICORES", Group: GroupPlayout,
			Kind: KindInt, Default: "1000", Advanced: true, Validate: nonNegativeWholeNumber,
			Doc:    "Processor time kept free for the rest of Loomarr when channels are encoded without a graphics card.",
			Detail: "On a host with no hardware encoder, the CPU kept free for the rest of Loomarr, in thousandths of a core (1000 = one core). Live channels may use the container's CPU limit (or the CPU count) minus this reserve. Changes apply to the next channel start without restart.",
		},

		{
			// The guide's display timezone (§12, V13b gap 7). The API always speaks absolute
			// epoch ms — a timezone is a RENDERING choice, and putting it on the wire would
			// invite a client to reinterpret instants it should merely format.
			//
			// Empty = the viewer's own browser timezone, which is right for the household
			// case. An operator sets it when the server and its viewers are elsewhere, or
			// when they want the guide to read in the channels' "broadcast" timezone.
			//
			// It is also the wall clock the rolling-window grid is laid on (#1675), so a daily
			// window turns at midnight in this zone. Empty there = the container's own zone.
			Key: "guide.timezone", Label: "Timezone", EnvVar: "GUIDE_TIMEZONE", Group: GroupPlayout,
			Kind: KindString, Presentation: PresentationTimezone, Presets: timezonePresets(), Default: "", Validate: ianaZone,
			Doc:    "The timezone the TV guide shows times in and each channel's day starts in.",
			Detail: "Which timezone the TV guide's times are shown in, as an IANA name like America/New_York, and where each channel's day starts: a daily schedule turns over at midnight in this timezone. Leave empty to show each viewer's own device timezone; the schedule then turns over at midnight in the server's timezone. Only the zones in the list are accepted: a name that is not one of them, \"Local\", and the old aliases such as US/Eastern or EST5EDT are refused, so a name means the same on every server.",
		},
		{
			// How far back the guide will look (§12, V13b gap 8).
			//
			// A real bound, not a cosmetic one: the past is recomputed from the channel's
			// CURRENT lineup, so a distant "as aired" view would be fiction — the lineup has
			// been reconciled since. A day is honest; a month would be invention presented as
			// history.
			Key: "guide.retention_hours", Label: "Past listings", EnvVar: "GUIDE_RETENTION_HOURS", Group: GroupPlayout,
			Kind: KindInt, Unit: UnitHours, Default: "24",
			Doc:    "How far back the TV guide lets you scroll.",
			Detail: "Past listings are recomputed from each channel's current lineup, so going too far back would show a schedule that never actually aired.",
		},

		// --- Backup (§16, §15 — added by V4) ---
		{
			Key: "backup.schedule", Label: "Automatic backup schedule", EnvVar: "BACKUP_SCHEDULE", Group: GroupBackup,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 30 3 * * *",
			Doc:    "When Loomarr backs up its settings, channels and people.",
			Detail: "When to write the nightly database backup. It contains settings, channels, people, encrypted secrets, and wrapped data keys, but not the external installation key. It does not contain filler, cached artwork, or operator image uploads.",
		},
		{
			Key: "backup.retain", Label: "Backups to keep", EnvVar: "BACKUP_RETAIN", Group: GroupBackup,
			Kind: KindInt, Default: "7",
			Doc: "How many backups to keep before pruning the oldest.",
		},
		{
			Key: "backup.dir", Label: "Backup location", EnvVar: "BACKUP_DIR", Group: GroupBackup,
			Kind: KindString, Presentation: PresentationPath, DataSubdir: "backups",
			Doc:    "Where backups are written.",
			Detail: "Defaults to a directory beside the database (/data in the container) so its volume carries them; point it elsewhere to keep backups off the same disk as the database.",
		},

		// --- Images (§15, §22, V52) ---
		{
			// ⚠ Defaults inside /data for the same reason `filler.dir` and `backup.dir` do — the
			// documented volume carries it — but with a consequence neither of those has: the
			// application backup is a DATABASE backup, and no image bytes are in the database
			// (§22). Everything here is regenerable or re-fetchable EXCEPT operator uploads, which
			// is why image maintenance counts unrecoverable-missing rows as a warning rather than
			// pretending it can repair them.
			Key: "images.dir", Label: "Image library location", EnvVar: "IMAGES_DIR", Group: GroupImages,
			Kind: KindString, Presentation: PresentationPath, DataSubdir: "images",
			Doc:    "Where Loomarr stores images, both originals and the resized copies it serves.",
			Detail: "Defaults to a directory beside the database (/data in the container) so its volume carries it. Not covered by the database backup: back up the volume.",
		},
		{
			Key: "images.max_upload_bytes", Label: "Maximum image upload", EnvVar: "IMAGES_MAX_UPLOAD_BYTES", Group: GroupImages,
			Kind: KindInt, Presentation: PresentationBytes, Default: "8388608",
			Doc:    "The largest image someone may upload.",
			Detail: "In bytes (8 MiB by default). Enforced while reading the upload, not from the size the client declares.",
		},
		{
			Key: "images.remote_fetch_enabled", Label: "Download remote artwork", EnvVar: "IMAGES_REMOTE_FETCH_ENABLED", Group: GroupImages,
			Kind: KindBool, Default: true,
			Doc:    "Whether Loomarr may download artwork from TMDB and your media server.",
			Detail: "Turn this off to keep to locally-produced images only — no outbound image requests are made.",
		},
		{
			Key: "images.cache_budget_mb", Label: "Resized-image cache", EnvVar: "IMAGES_CACHE_BUDGET_MB", Group: GroupImages,
			Kind: KindInt, Unit: UnitMiB, Default: "2048", Advanced: true,
			Doc:    "How much disk the resized copies of images may use.",
			Detail: "Past this, Loomarr starts removing the least recently used ones. They are always regenerable, so this costs a little latency, never an image.",
		},
	}
}
