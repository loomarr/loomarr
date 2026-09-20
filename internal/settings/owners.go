package settings

// ownerFor is the registry's one assignment point for editing tasks. Most groups have one
// owner; Filler intentionally has task-sized owners because its broad registry group spans
// the whole workflow. Returning the zero value is fail-closed: newRegistry rejects it.
func ownerFor(setting Setting) Owner {
	if owner, ok := fillerOwners[setting.Key]; ok {
		return owner
	}
	if setting.Key == "library.path_map" {
		return OwnerPlayback
	}
	if setting.Key == "access.public_url" {
		return OwnerSharing
	}
	if setting.Group == GroupAdvanced {
		switch setting.Key {
		case "diagnostics.dir", "diagnostics.retention", "diagnostics.max_storage_mb":
			return OwnerDiagnostics
		case "request.ttl", "downloading.ttl", "episodes.max_age", "job.workers", "job.timeout",
			"jobs.retention", "proposals.retention", "activity.retention", "setup.completed":
			return OwnerAdvanced
		default:
			return OwnerTasks
		}
	}

	switch setting.Group {
	case GroupMediaServer, GroupRequester, GroupTunarr, GroupTMDB:
		return OwnerConnections
	case GroupGeneral:
		return OwnerSharing
	case GroupAI:
		return OwnerAI
	case GroupChannels:
		return OwnerDefaults
	case GroupPlayout:
		return OwnerPlayback
	case GroupBackup:
		return OwnerBackup
	case GroupNotifications:
		return OwnerNotifications
	case GroupUsersSecurity, GroupSSO:
		return OwnerAccess
	case GroupImages:
		return OwnerStorage
	default:
		return ""
	}
}

var fillerOwners = map[string]Owner{
	"filler.home_country":    OwnerLocation,
	"filler.home_market":     OwnerLocation,
	"filler.breaks_per_hour": OwnerDefaults,

	"filler.dir":                   OwnerFillerFolders,
	"filler.watch_dir":             OwnerFillerFolders,
	"filler.sync_every":            OwnerFillerFolders,
	"filler.source.folder.enabled": OwnerFillerFolders,

	"filler.fetch.every":       OwnerFillerDownloads,
	"filler.fetch.max_per_run": OwnerFillerDownloads,

	"filler.fetch.max_catalog_clips":   OwnerFillerStorage,
	"filler.storage.library_budget_gb": OwnerFillerStorage,
	"filler.research.enabled":          OwnerFillerDetails,
	"filler.research.web_provider":     OwnerFillerDetails,
	"filler.research.brave_api_key":    OwnerFillerDetails,
	"filler.research.searxng_url":      OwnerFillerDetails,
	"filler.research.monthly_limit":    OwnerFillerDetails,
	"filler.incoming.ready_window":     OwnerFillerIncoming,
	"filler.break_duration":            OwnerFillerBreaks,
	"filler.pod_max":                   OwnerFillerBreaks,

	"filler.transcribe.enabled":               OwnerFillerReview,
	"filler.vision.enabled":                   OwnerFillerReview,
	"filler.conditioning.normalize_loudness":  OwnerFillerReview,
	"filler.autosplit.enabled":                OwnerFillerReview,
	"filler.autosplit.min_confidence":         OwnerFillerReview,
	"filler.autosplit.max_duration":           OwnerFillerReview,
	"filler.structure_window_authority_path":  OwnerFillerReview,
	"filler.structure_window_deployment_path": OwnerFillerReview,

	"filler.cooldown_seconds":    OwnerFillerPlayback,
	"filler.min_quality":         OwnerFillerPlayback,
	"filler.weight":              OwnerFillerPlayback,
	"filler.min_duration":        OwnerFillerPlayback,
	"filler.split.review_window": OwnerFillerPlayback,
	"filler.min_clip_duration":   OwnerFillerPlayback,
	"filler.max_clip_duration":   OwnerFillerPlayback,
	"filler.target_lufs":         OwnerFillerPlayback,
	"filler.language":            OwnerFillerPlayback,
	"filler.language_provider":   OwnerFillerPlayback,
	"filler.language_model":      OwnerFillerPlayback,

	"filler.pipeline.max_clips":        OwnerFillerLimits,
	"filler.transcode.max_per_run":     OwnerFillerLimits,
	"filler.pipeline.max_whisper":      OwnerFillerLimits,
	"filler.pipeline.max_vision":       OwnerFillerLimits,
	"filler.pipeline.max_split_vision": OwnerFillerLimits,
	"filler.pipeline.max_splits":       OwnerFillerLimits,

	"ingest.ytdlp_path":    OwnerFillerTools,
	"ingest.ffmpeg_path":   OwnerFillerTools,
	"ingest.timeout":       OwnerFillerTools,
	"ingest.whisper_path":  OwnerFillerTools,
	"ingest.whisper_model": OwnerFillerTools,

	"filler.transcribe.provider": OwnerAI,
	"filler.transcribe.model":    OwnerAI,
	"filler.vision.provider":     OwnerAI,
	"filler.vision.model":        OwnerAI,
	"filler.vision.url":          OwnerAI,
	"filler.vision.api_key":      OwnerAI,
}

var knownOwners = map[Owner]struct{}{
	OwnerConnections: {}, OwnerAI: {}, OwnerDefaults: {}, OwnerNotifications: {},
	OwnerLocation: {}, OwnerSharing: {}, OwnerAccess: {}, OwnerPlayback: {},
	OwnerStorage: {}, OwnerBackup: {}, OwnerTasks: {}, OwnerDiagnostics: {}, OwnerAdvanced: {},
	OwnerFillerFolders: {}, OwnerFillerDownloads: {}, OwnerFillerStorage: {}, OwnerFillerDetails: {},
	OwnerFillerIncoming: {}, OwnerFillerBreaks: {}, OwnerFillerReview: {},
	OwnerFillerPlayback: {}, OwnerFillerLimits: {}, OwnerFillerTools: {},
}
