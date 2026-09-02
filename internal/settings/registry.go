package settings

import "fmt"

// Registry is the ordered, indexed set of every app-managed Setting plus explicitly marked
// one-time migration inputs. Runtime resolution reads both; API, docs, and write surfaces omit the
// migration-only entries. Env-only bootstrap keys (config-design §1) are deliberately absent —
// they live in config.Config and never resolve through here.
type Registry struct {
	ordered []Setting          // declaration order (stable docs/UI ordering)
	byKey   map[string]Setting // key → Setting
	byEnv   map[string]Setting // EnvVar → Setting (env-pin resolution)
}

// NewRegistry builds the registry from the declared settings, panicking on a
// duplicate key/env or a malformed declaration (a programmer error, caught at
// startup — never a runtime condition). The registry is the contract, so a
// broken declaration must fail loudly and immediately.
func NewRegistry() *Registry {
	return newRegistry(declared())
}

func newRegistry(list []Setting) *Registry {
	r := &Registry{
		byKey: make(map[string]Setting, len(list)),
		byEnv: make(map[string]Setting, len(list)),
	}
	for _, s := range list {
		if s.Key == "" || s.EnvVar == "" {
			panic(fmt.Sprintf("settings: declaration missing key/env: %+v", s))
		}
		if _, dup := r.byKey[s.Key]; dup {
			panic("settings: duplicate key " + s.Key)
		}
		if _, dup := r.byEnv[s.EnvVar]; dup {
			panic("settings: duplicate env var " + s.EnvVar)
		}
		if s.Kind == KindEnum && len(s.Enum) == 0 {
			panic("settings: enum setting " + s.Key + " has no Enum values")
		}
		switch s.Apply {
		case ApplyLive, ApplyRestart:
		default:
			panic("settings: invalid apply timing for " + s.Key + ": " + string(s.Apply))
		}
		r.byKey[s.Key] = s
		r.byEnv[s.EnvVar] = s
		r.ordered = append(r.ordered, s)
	}
	return r
}

// Get returns the Setting for a key, or false if the key is not declared.
func (r *Registry) Get(key string) (Setting, bool) { s, ok := r.byKey[key]; return s, ok }

// All returns every declaration in stable order, including migration-only inputs.
func (r *Registry) All() []Setting { return r.ordered }

// RestartKeys returns the app-managed settings that take effect at the next
// application generation. Declaration order is stable and therefore suitable
// for freezing the generation and reporting pending drift.
func (r *Registry) RestartKeys() []string {
	var keys []string
	for _, s := range r.ordered {
		if s.Apply == ApplyRestart {
			keys = append(keys, s.Key)
		}
	}
	return keys
}

// ByGroup returns public settings in one group, in declaration order.
func (r *Registry) ByGroup(g Group) []Setting {
	var out []Setting
	for _, s := range r.ordered {
		if s.Group == g && !s.MigrationOnly {
			out = append(out, s)
		}
	}
	return out
}

// Groups returns groups with public settings in a stable order (for the docs generator and the
// Settings IA). Declaration order across groups is preserved.
func (r *Registry) Groups() []Group {
	seen := map[Group]bool{}
	var out []Group
	for _, s := range r.ordered {
		if s.MigrationOnly {
			continue
		}
		if !seen[s.Group] {
			seen[s.Group] = true
			out = append(out, s.Group)
		}
	}
	return out
}
