package settings

import (
	"fmt"
)

// Resolve returns a key's live value and provenance (config-design §3). Order:
// env > db > default. Env is pre-validated at boot (New), so it never self-heals;
// a bad DB value self-heals to default with a caution. Panics only on an unknown
// key (a programmer error — the caller used a key not in the registry).
func (s *Service) Resolve(key string) Resolved {
	set, ok := s.reg.Get(key)
	if !ok {
		panic("settings: Resolve on undeclared key " + key)
	}
	s.mu.RLock()
	dbRaw, hasDB := s.db[key]
	s.mu.RUnlock()
	return s.resolve(set, hasDB, dbRaw)
}

// resolve implements config-design §3's asymmetric resolution for one setting.
//
// Precedence is env > db > default. The asymmetry is in how a BAD value at each
// tier is handled (who made the mistake):
//   - env: pre-validated at boot, so a set env value here is trusted and wins.
//     (An invalid env value already failed the boot in New — it cannot reach here.)
//   - db: the app wrote it (or a migration drifted). A value that no longer parses
//     must NOT crash a running install: log a warning, fall through to the default,
//     and mark the result Caution so the UI can show a self-healed chip.
//   - default: the registry Default, always valid (registry_test enforces it).
//
// hasDB / dbRaw are the snapshot's stored override for this key (raw string).
func (s *Service) resolve(set Setting, hasDB bool, dbRaw string) Resolved {
	// env wins. It was validated at boot (New), so parse cannot fail here for a
	// value the environment supplied; an I/O error reading a *_FILE that appeared
	// after boot is the only surprise — treat it like a bad env and fall through
	// rather than panic a read path.
	if raw, ok, err := s.envValue(set); err == nil && ok {
		if v, perr := set.parse(raw); perr == nil {
			return Resolved{Setting: set, Value: v, Provenance: ProvenanceEnv}
		}
	}
	// db next. The app wrote it; if it no longer parses, self-heal to the default
	// and surface a caution — never crash a running install for the app's own drift.
	if hasDB {
		if v, err := set.parse(dbRaw); err == nil {
			return Resolved{Setting: set, Value: v, Provenance: ProvenanceDB}
		} else {
			// Redaction (§4): name the key and the shape error, never the value —
			// set.parse already builds messages from the key, not the raw string,
			// but for a secret we suppress even that to be safe.
			if set.IsSecret() {
				s.log.Warn("settings: stored value invalid, using default", "key", set.Key)
			} else {
				s.log.Warn("settings: stored value invalid, using default", "key", set.Key, "error", err)
			}
			return s.defaultResolved(set, true)
		}
	}
	// default.
	return s.defaultResolved(set, false)
}

// defaultResolved builds a Resolved from the registry default. The default MUST parse
// to the setting's typed Value (e.g. a KindDuration default "24h" → time.Duration), same
// as the env/db paths — a caller doing Resolve(key).Value.(time.Duration) must get a real
// Duration no matter which provenance won, or a typed accessor silently returns the zero
// value (the rolling-window horizon bug: an unparsed "24h" string failed the assertion in
// set.dur → window 0 → no truncation). The registry guarantees every default is valid
// (registry_test), so parse cannot fail; if it somehow does, fall back to the raw string
// rather than panic a read path.
func (s *Service) defaultResolved(set Setting, caution bool) Resolved {
	v := set.Default
	if raw, ok := set.Default.(string); ok {
		if parsed, err := set.parse(raw); err == nil {
			v = parsed
		}
	}
	return Resolved{
		Setting:    set,
		Value:      v,
		Provenance: ProvenanceDefault,
		Caution:    caution,
	}
}

// String returns a key's resolved value as a string (empty for unset optionals).
// Panics on a non-string/secret/enum/url Kind — callers wanting typed values use
// the typed accessors below.
func (s *Service) String(key string) string {
	r := s.Resolve(key)
	if r.Value == nil {
		return ""
	}
	str, ok := r.Value.(string)
	if !ok {
		panic(fmt.Sprintf("settings.String: %s is %T, not string", key, r.Value))
	}
	return str
}

// Provenance returns just the provenance for a key (UI lock-state; cheap).
func (s *Service) Provenance(key string) Provenance { return s.Resolve(key).Provenance }
