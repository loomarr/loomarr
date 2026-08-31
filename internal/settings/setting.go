// Package settings is Loomarr's configuration subsystem (config-design.md): one
// typed registry declares every app-managed setting exactly once, and resolution
// (env > database > default), the Settings API, the wizard, feature gating, and
// the generated docs all derive from it. A setting that isn't in the registry
// does not exist (AGENTS.md do-nots; config-design §2).
//
// The split (config-design §1): env-only keys — those needed before the database
// opens, or that describe process topology (DATABASE_URL, AUTO_MIGRATE,
// LISTEN_ADDR, LOG_LEVEL, TZ) — stay in config.Config and are NOT registry
// settings. Everything else is app-managed, env-pinnable, and lives here.
package settings

import "fmt"

// Kind is a setting's value type. It drives parsing, validation, and how the UI
// renders the control (config-design §2).
type Kind string

const (
	KindString     Kind = "string"
	KindInt        Kind = "int"
	KindBool       Kind = "bool"
	KindDuration   Kind = "duration" // Go duration string, e.g. "168h"
	KindURL        Kind = "url"      // validated + normalized (scheme required, trailing slash stripped)
	KindEnum       Kind = "enum"     // one of Setting.Enum
	KindSecret     Kind = "secret"   // stored, masked on read, never echoed (§4)
	KindStringList Kind = "string_list"
	KindCron       Kind = "cron" // a 6-field seconds-leading cron expr (job schedules, §18.1)
)

// Presentation describes how a valid stored value should be edited and explained.
// Kind owns validation; Presentation owns the human control. Keeping those separate lets an
// integer remain an integer on the wire while the UI edits it as bytes or megabytes.
type Presentation string

const (
	PresentationDefault  Presentation = ""
	PresentationBytes    Presentation = "bytes"
	PresentationPath     Presentation = "path"
	PresentationLanguage Presentation = "language"
)

// ApplyTiming says when a persisted setting becomes the running value. The zero
// value is deliberately live: declarations have always hot-applied unless they
// explicitly opt into the much rarer generation boundary.
type ApplyTiming string

const (
	ApplyLive    ApplyTiming = ""
	ApplyRestart ApplyTiming = "restart"
)

// Group is a Settings-UI page (config-design §5). Every setting belongs to
// exactly one; the wizard renders one group's form per step (§6).
type Group string

const (
	GroupMediaServer   Group = "connections.media_server"
	GroupRequester     Group = "connections.requester"
	GroupTunarr        Group = "connections.tunarr"
	GroupTMDB          Group = "connections.tmdb"
	GroupAI            Group = "ai"
	GroupChannels      Group = "channels"
	GroupPlayout       Group = "playout"
	GroupFiller        Group = "filler"
	GroupBackup        Group = "backup"
	GroupNotifications Group = "notifications"
	GroupUsersSecurity Group = "users_security"
	// GroupSSO is its own group rather than more keys in users_security (§11, V8): the
	// Security page renders one block per group, and provider settings sitting unlabelled
	// among session TTLs would read as more session config. It also gives the block
	// somewhere to state what SSO does NOT do — §11's model is unusual enough that the
	// absence of "create people on first sign-in" needs saying.
	GroupSSO Group = "sso"
	// GroupImages is the image service (§22, V52) — where images live on disk, which formats
	// are produced, and the remote-fetch/retention knobs.
	//
	// Its own group for the same reason `backup` and `filler` have one: `images.dir` is an
	// operator's storage decision, and a storage path filed under "Advanced" beside job cron
	// expressions reads as something you should not touch. ⚠ There is no Settings → Images PAGE
	// yet — phase 3 declares the registry, and these keys are reachable via Settings → All
	// until a later phase gives them a form. `Groups()` is consumed only by the docs generator,
	// so a group with no page adds a docs section and nothing else.
	GroupImages   Group = "images"
	GroupAdvanced Group = "advanced"
)

// Feature is a capability gated on settings completeness (config-design §7). A
// setting's RequiredFor names the feature it's a prerequisite for; the computed
// feature set drives the API 409s, the tab empty states, and the checklist —
// one function, three consumers, no drift.
type Feature string

const (
	FeatureNone        Feature = ""
	FeatureAcquisition Feature = "acquisition" // needs a requester (Seerr or direct arr)
	FeatureSuggestions Feature = "suggestions" // needs an LLM + TMDB grounding
	FeatureFiller      Feature = "filler"      // needs a filler drop-folder
	FeatureUserSync    Feature = "user_sync"   // needs a media server to sync users FROM
	// ⚠ `ingest` is the OR of the two below — "some source can download". Anything reporting
	// which source is usable must read those instead (V38b).
	FeatureIngestArchive Feature = "ingest_archive" // archive.org: HTTP + ffmpeg, no yt-dlp
	FeatureIngestYouTube Feature = "ingest_youtube" // yt-dlp + ffmpeg
	FeatureIngest        Feature = "ingest"         // needs runnable yt-dlp + ffmpeg (now always in the image, §16 V3 — so this gate reports a BROKEN vendored binary, not an opt-out)
)

// ValidateFunc checks a parsed value's shape (config-design §2, §9). URL
// normalization lives here. A validator must NEVER echo a secret value into its
// error string (§4 redaction). A nil ValidateFunc means "any value of the Kind".
type ValidateFunc func(any) error

// EnumOption is one choice of a KindEnum setting: the stored VALUE (the closed-set
// member the BE persists and validates) plus its display LABEL (config-design §5).
// The label is a fact the registry owns — "openai" → "OpenAI", "emby" → "Emby" — so
// it lives next to the value rather than being re-derived (and drifting) on the FE.
type EnumOption struct {
	Value string // the stored/validated value, e.g. "ollama"
	Label string // the human label shown in the dropdown, e.g. "Ollama"
}

// opt is a terse constructor for the registry declarations: opt("ollama", "Ollama").
func opt(value, label string) EnumOption { return EnumOption{Value: value, Label: label} }

// Setting declares one app-managed configuration key (config-design §2). Declared
// exactly once, in the registry; nothing constructs a Setting elsewhere.
type Setting struct {
	Key          string // canonical key, e.g. "library.url" — the DB + API identity
	Label        string // human label for workflow forms; raw-key views still show Key
	EnvVar       string // the env pin, e.g. "LIBRARY_URL" (config-design §1)
	Group        Group
	Kind         Kind
	Presentation Presentation // optional richer editor semantics beyond Kind
	Apply        ApplyTiming  // empty/live = next read; restart = next app generation
	Default      any          // zero value of the Kind if a key has no default (e.g. a secret)
	Enum         []EnumOption // Kind == KindEnum: the closed set, each with a display label
	Advanced     bool         // hidden behind the per-page "Show advanced" toggle (§5)
	Required     Feature      // RequiredFor: the feature this key gates (§7); FeatureNone = always-optional
	Validate     ValidateFunc // shape validation; nil = Kind-default only
	Doc          string       // one-liner: UI help text + generated docs (§2)
	// ShowWhen makes a field CONDITIONAL (config-design §5): it is shown only when the
	// current value of a named key is one of the listed values. Empty/nil = always shown.
	// e.g. llm.api_key: {"llm.provider": {"openai"}} hides the key when Ollama is picked.
	ShowWhen map[string][]string
}

// IsSecret reports whether the setting holds a secret (masked on read, never
// echoed — §4). Kept as a method so callers don't special-case KindSecret.
func (s Setting) IsSecret() bool { return s.Kind == KindSecret }

// EnumValues returns just the stored values of an enum's options — for the
// value-only consumers (validation, generated docs) that don't care about labels.
func (s Setting) EnumValues() []string {
	vals := make([]string, len(s.Enum))
	for i, o := range s.Enum {
		vals[i] = o.Value
	}
	return vals
}

// validateEnum checks a value against a KindEnum's closed set. Registry helper so
// every enum setting gets the same fail-closed check without repeating it.
func (s Setting) validateEnum(v any) error {
	vals := s.EnumValues()
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("%s: expected one of %v, got non-string", s.Key, vals)
	}
	for _, e := range vals {
		if str == e {
			return nil
		}
	}
	return fmt.Errorf("%s: %q is not one of %v", s.Key, str, vals)
}
