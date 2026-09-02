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
	unlocked := s.unlocked[key]
	s.mu.RUnlock()
	return s.resolve(set, hasDB, dbRaw, unlocked)
}

// ResolveMany returns several settings from one in-memory snapshot. It is the
// batch form for an adapter operation whose configuration must not mix values
// across a concurrent hot-apply. Unknown keys panic for the same reason Resolve
// does: callers name registry keys in code, so an unknown name is a programmer
// error rather than an operator condition.
func (s *Service) ResolveMany(keys ...string) map[string]Resolved {
	type input struct {
		set      Setting
		dbRaw    string
		hasDB    bool
		unlocked bool
	}

	inputs := make([]input, len(keys))
	for i, key := range keys {
		set, ok := s.reg.Get(key)
		if !ok {
			panic("settings: ResolveMany on undeclared key " + key)
		}
		inputs[i].set = set
	}

	s.mu.RLock()
	for i, key := range keys {
		inputs[i].dbRaw, inputs[i].hasDB = s.db[key]
		inputs[i].unlocked = s.unlocked[key]
	}
	s.mu.RUnlock()

	resolved := make(map[string]Resolved, len(keys))
	for i, key := range keys {
		in := inputs[i]
		resolved[key] = s.resolve(in.set, in.hasDB, in.dbRaw, in.unlocked)
	}
	return resolved
}

// ResolveMigrationOnly strictly resolves former settings for a one-time importer. Unlike ordinary
// runtime resolution, malformed legacy database values do not self-heal and malformed environment
// values do not fall through: importing a different value than the operator supplied would turn a
// compatibility path into a new source of configuration authority.
func (s *Service) ResolveMigrationOnly(keys ...string) (map[string]Resolved, error) {
	type input struct {
		set      Setting
		dbRaw    string
		hasDB    bool
		unlocked bool
	}
	inputs := make([]input, len(keys))
	for i, key := range keys {
		set, ok := s.reg.Get(key)
		if !ok || !set.MigrationOnly {
			return nil, fmt.Errorf("settings: %s is not a migration-only setting", key)
		}
		inputs[i].set = set
	}
	s.mu.RLock()
	for i, key := range keys {
		inputs[i].dbRaw, inputs[i].hasDB = s.db[key]
		inputs[i].unlocked = s.unlocked[key]
	}
	s.mu.RUnlock()

	resolved := make(map[string]Resolved, len(keys))
	for i, key := range keys {
		in := inputs[i]
		if !in.unlocked {
			raw, present, err := s.envValue(in.set)
			if err != nil {
				return nil, err
			}
			if present {
				value, err := in.set.parse(raw)
				if err != nil {
					return nil, fmt.Errorf("invalid legacy env %s: %w", in.set.EnvVar, err)
				}
				resolved[key] = Resolved{Setting: in.set, Value: value, Provenance: ProvenanceEnv}
				continue
			}
		}
		if in.hasDB {
			value, err := in.set.parse(in.dbRaw)
			if err != nil {
				return nil, fmt.Errorf("invalid legacy setting %s: %w", key, err)
			}
			resolved[key] = Resolved{
				Setting: in.set, Value: value, Provenance: ProvenanceDB, EnvOverride: in.unlocked,
			}
			continue
		}
		resolved[key] = s.defaultResolved(in.set, false, in.unlocked)
	}
	return resolved, nil
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
// unlocked is §3.1's claim: an admin has taken this key back from the environment.
func (s *Service) resolve(set Setting, hasDB bool, dbRaw string, unlocked bool) Resolved {
	// env wins — UNLESS an admin has explicitly taken this key back (§3.1). The claim is
	// durable (a settings column), so it holds across the restart that re-reads env; an
	// in-memory version of this check would let env silently reclaim the key on reboot and
	// discard what the operator saved.
	//
	// Skipping the branch entirely — rather than resolving env and preferring db — is what
	// makes the result honestly `db` rather than a third provenance. The lock STATE is
	// reported separately (Resolved.EnvOverride), because "the operator overrode
	// SEERR_URL" and "the environment never mentioned SEERR_URL" are different facts and a
	// field that conflates them cannot explain itself.
	//
	// It was validated at boot (New), so parse cannot fail here for a value the environment
	// supplied; an I/O error reading a *_FILE that appeared after boot is the only surprise
	// — treat it like a bad env and fall through rather than panic a read path.
	if !unlocked {
		if raw, ok, err := s.envValue(set); err == nil && ok {
			if v, perr := set.parse(raw); perr == nil {
				return Resolved{Setting: set, Value: v, Provenance: ProvenanceEnv}
			}
		}
	}
	// db next. The app wrote it; if it no longer parses, self-heal to the default
	// and surface a caution — never crash a running install for the app's own drift.
	if hasDB {
		if v, err := set.parse(dbRaw); err == nil {
			return Resolved{Setting: set, Value: v, Provenance: ProvenanceDB, EnvOverride: unlocked}
		} else {
			// Redaction (§4): name the key and the shape error, never the value —
			// set.parse already builds messages from the key, not the raw string,
			// but for a secret we suppress even that to be safe.
			if set.IsSecret() {
				s.log.Warn("settings: stored value invalid, using default", "key", set.Key)
			} else {
				s.log.Warn("settings: stored value invalid, using default", "key", set.Key, "error", err)
			}
			return s.defaultResolved(set, true, unlocked)
		}
	}
	// default. An unlocked key can legitimately land here — a secret never seeds on unlock
	// (§3.1), so it has no stored value until the operator enters one — and it is still
	// overriding, so the flag rides along or the field would claim the env var is unset.
	return s.defaultResolved(set, false, unlocked)
}

// defaultResolved builds a Resolved from the registry default. The default MUST parse
// to the setting's typed Value (e.g. a KindDuration default "24h" → time.Duration), same
// as the env/db paths — a caller doing Resolve(key).Value.(time.Duration) must get a real
// Duration no matter which provenance won, or a typed accessor silently returns the zero
// value (the rolling-window horizon bug: an unparsed "24h" string failed the assertion in
// set.dur → window 0 → no truncation). The registry guarantees every default is valid
// (registry_test), so parse cannot fail; if it somehow does, fall back to the raw string
// rather than panic a read path.
func (s *Service) defaultResolved(set Setting, caution, unlocked bool) Resolved {
	v := set.Default
	if raw, ok := set.Default.(string); ok {
		if parsed, err := set.parse(raw); err == nil {
			v = parsed
		}
	}
	return Resolved{
		Setting:     set,
		Value:       v,
		Provenance:  ProvenanceDefault,
		Caution:     caution,
		EnvOverride: unlocked,
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
