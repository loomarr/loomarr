package settings

import (
	"github.com/loomarr/loomarr/internal/platform/buildinfo"
)

// declaredAI is the TMDB connection, the AI provider and models, and self-updating channels.
func declaredAI() []Setting {
	return []Setting{
		// --- Connections: TMDB (§15, Phase 11) ---
		{
			Key: "tmdb.api_key", EnvVar: "TMDB_API_KEY", Group: GroupTMDB,
			// The default is the key the release build ships with (decision 51); List reports
			// it as builtIn, never as set, and never previews it.
			Kind: KindSecret, Default: buildinfo.TMDBKey(), Required: FeatureSuggestions,
			Doc:    "Your own TMDB API key, which most households can leave empty.",
			Detail: "Your own TMDB API key, if you have one. Loomarr comes with its own, so most households can leave this empty. TMDB powers title search, channel icon suggestions, and grounding for AI channel suggestions.",
		},
		{
			// #2151 decision 21: one installation-wide metadata language and country, defaulting to
			// the media server's.
			Key: "metadata.language", Label: "Title language", EnvVar: "METADATA_LANGUAGE", Group: GroupTMDB,
			Kind: KindString, Default: "", Advanced: true, Validate: optionalMetadataLanguage,
			Doc:    "The language titles, overviews and trailers are shown in.",
			Detail: "As a language code (en, fr, pt-BR). Empty uses your media server's metadata language, or English. Changing it fetches every title's details again.",
		},
		{
			Key: "metadata.country", Label: "Ratings country", EnvVar: "METADATA_COUNTRY", Group: GroupTMDB,
			Kind: KindString, Default: "", Advanced: true, Validate: metadataCountry,
			Doc:    "The country whose content ratings Loomarr uses.",
			Detail: "Only US ratings are supported for now, so this can be empty or US.",
		},

		// --- AI (§15, §8.1; in-app selection persists to llm.* and overrides these env pins) ---
		{
			Key: "llm.provider", Label: "Lineup AI provider", EnvVar: "LLM_PROVIDER", Group: GroupAI,
			Kind: KindEnum, Enum: []EnumOption{opt("ollama", "Ollama"), opt("openai", "OpenAI-compatible")}, Default: "ollama", Required: FeatureSuggestions,
			Doc:    "Which AI to use: a local Ollama, or an OpenAI-compatible service.",
			Detail: "You can also pick a model in the AI settings.",
		},
		{
			// Both providers need an endpoint: the Ollama host for local AI, or the
			// OpenAI-compatible base URL for hosted AI. The default host is conventional, not
			// universal, so hiding this for Ollama made remote Ollama impossible to configure.
			Key: "llm.url", Label: "AI service address", EnvVar: "LLM_URL", Group: GroupAI,
			Kind: KindURL, Default: "",
			Doc:    "The address of your AI service.",
			Detail: "For Ollama, its host such as http://ollama:11434. For a hosted provider, the exact OpenAI-compatible API base; Loomarr fills this for OpenRouter, while Custom remains editable.",
		},
		{
			// The normal AI page uses the ranked picker for both provider kinds so two
			// controls never compete. This declaration remains for env compatibility and
			// the explicit All Settings escape hatch.
			Key: "llm.model", Label: "Lineup model", EnvVar: "LLM_MODEL", Group: GroupAI,
			Kind: KindString, Default: "", Required: FeatureSuggestions,
			Doc:      "The AI model that builds channel lineups.",
			Detail:   "The active model used to build channel lineups. Prefer the guided picker on the AI page; OpenRouter ids use provider/model (for example openai/gpt-4o-mini).",
			ShowWhen: map[string][]string{"llm.provider": {"openai"}},
		},
		{
			// Ollama is local and needs no key — this only applies to a hosted service.
			Key: "llm.api_key", Label: "Hosted AI API key", EnvVar: "LLM_API_KEY", Group: GroupAI,
			Kind: KindSecret, Default: "",
			Doc:      "The API key for your hosted AI service.",
			Detail:   "Never shown again after saving.",
			ShowWhen: map[string][]string{"llm.provider": {"openai"}},
		},
		{
			// Local-only (§8.2): a hosted service has no model to hold in memory, so this
			// is hidden for the openai provider rather than shown as an inert control.
			Key: "llm.keep_alive", Label: "Keep local model loaded", EnvVar: "LLM_KEEP_ALIVE", Group: GroupAI,
			Kind: KindDuration, Default: "2m", Advanced: true,
			Doc:      "How long the local AI model stays loaded between requests.",
			Detail:   "How long to keep the local AI model loaded in memory between requests. Loading it takes several seconds, so keeping it ready makes suggestions much faster — but the model shares GPU memory with channel playback, so the default is short (2m) to free that memory for streaming. Raise it if you rarely stream and want faster suggestions; set 0 to free memory as soon as each request finishes.",
			ShowWhen: map[string][]string{"llm.provider": {"ollama"}},
		},
		{
			Key: "asr.provider", Label: "Speech recognition", EnvVar: "ASR_PROVIDER", Group: GroupAI,
			Kind: KindEnum, Enum: []EnumOption{
				opt("whisper", "Built in on this device"), opt("hosted", "Connected speech service"),
			},
			Default: "whisper",
			Doc:     "How Loomarr understands speech in clips.",
			Detail:  "The built-in option stays on this device; a connected service is faster on low-power hardware and can use a separate address and key under Advanced.",
		},
		{
			Key: "asr.url", Label: "Speech service address", EnvVar: "ASR_URL", Group: GroupAI,
			Kind: KindURL, Default: "", Advanced: true,
			Doc:      "The address of a separate speech service, or blank to use your AI service.",
			Detail:   "OpenAI-compatible speech-to-text API base, such as http://ai-server:8083/v1. Leave blank to reuse the active hosted AI service.",
			ShowWhen: map[string][]string{"asr.provider": {"hosted"}},
		},
		{
			Key: "asr.api_key", Label: "Speech service API key", EnvVar: "ASR_API_KEY", Group: GroupAI,
			Kind: KindSecret, Default: "", Advanced: true,
			Doc:      "The API key for the separate speech service.",
			Detail:   "It is never shown again after saving and the main AI key is never sent to a custom speech address.",
			ShowWhen: map[string][]string{"asr.provider": {"hosted"}},
		},
		{
			Key: "asr.model", Label: "Speech model", EnvVar: "ASR_MODEL", Group: GroupAI,
			Kind: KindString, Default: "openai/whisper-large-v3", Advanced: true,
			Doc:      "The speech model the connected service uses.",
			Detail:   "Speech-to-text model used by the connected service. It is separate from the lineup model because it must return timed transcript segments and the detected language.",
			ShowWhen: map[string][]string{"asr.provider": {"hosted"}},
		},
		{
			Key: "suggest.max_acquisitions", Label: "Pending-download limit per person", EnvVar: "SUGGEST_MAX_ACQUISITIONS", Group: GroupAI,
			Kind: KindInt, Default: 10,
			Doc: "The most titles a single suggestion may download.",
		},

		// --- Self-updating channels / re-curation (programming-design §8.2) ---
		{
			Key: "job.recurate.schedule", EnvVar: "JOB_RECURATE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 0 4 * * 0", Advanced: true,
			Doc:    "How often auto-curate channels look again at what your library offers.",
			Detail: "How often auto-curate channels re-evaluate their intent against the library (cron). Weekly by default — this runs the AI, so keep it infrequent.",
		},
		{
			Key: "recurate.min_score_pct", Label: "Auto-curation request threshold", EnvVar: "RECURATE_MIN_SCORE_PCT", Group: GroupAI,
			Kind: KindInt, Unit: UnitPercent, Default: 60, Advanced: true,
			Doc:    "How good a match a title you don't have must be before auto-curate downloads it.",
			Detail: "Quality bar (0–100) a not-in-library title must clear for auto-curate to REQUEST it. In-library matches are added regardless. A per-channel override may be stricter or looser.",
		},
		{
			Key: "recurate.max_titles", Label: "Auto-curation channel limit", EnvVar: "RECURATE_MAX_TITLES", Group: GroupAI,
			Kind: KindInt, Default: 40, Advanced: true,
			Doc:    "The most titles an auto-curate channel may grow to.",
			Detail: "Re-curation won't request net-new titles past this cap. A per-channel override may be stricter or looser.",
		},
	}
}
