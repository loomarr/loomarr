// Package settings is Loomarr's configuration subsystem (config-design.md): one
// typed registry declares every app-managed setting exactly once, and resolution
// (env > database > default), the Settings API, the wizard, feature gating, and
// the generated docs all derive from it. A setting that isn't in the registry
// does not exist (CLAUDE.md do-nots; config-design §2).
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
	GroupFiller        Group = "filler"
	GroupUsersSecurity Group = "users_security"
	GroupAdvanced      Group = "advanced"
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
)

// ValidateFunc checks a parsed value's shape (config-design §2, §9). URL
// normalization lives here. A validator must NEVER echo a secret value into its
// error string (§4 redaction). A nil ValidateFunc means "any value of the Kind".
type ValidateFunc func(any) error

// Setting declares one app-managed configuration key (config-design §2). Declared
// exactly once, in the registry; nothing constructs a Setting elsewhere.
type Setting struct {
	Key      string // canonical key, e.g. "library.url" — the DB + API identity
	EnvVar   string // the env pin, e.g. "LIBRARY_URL" (config-design §1)
	Group    Group
	Kind     Kind
	Default  any          // zero value of the Kind if a key has no default (e.g. a secret)
	Enum     []string     // Kind == KindEnum: the closed set
	Advanced bool         // hidden behind the per-page "Show advanced" toggle (§5)
	Required Feature      // RequiredFor: the feature this key gates (§7); FeatureNone = always-optional
	Validate ValidateFunc // shape validation; nil = Kind-default only
	Doc      string       // one-liner: UI help text + generated docs (§2)
}

// IsSecret reports whether the setting holds a secret (masked on read, never
// echoed — §4). Kept as a method so callers don't special-case KindSecret.
func (s Setting) IsSecret() bool { return s.Kind == KindSecret }

// validateEnum checks a value against a KindEnum's closed set. Registry helper so
// every enum setting gets the same fail-closed check without repeating it.
func (s Setting) validateEnum(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("%s: expected one of %v, got non-string", s.Key, s.Enum)
	}
	for _, e := range s.Enum {
		if str == e {
			return nil
		}
	}
	return fmt.Errorf("%s: %q is not one of %v", s.Key, str, s.Enum)
}
