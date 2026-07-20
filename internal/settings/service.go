package settings

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Provenance is where a resolved value came from (config-design §3). It drives the
// UI: env → the field is locked ("set via environment"); db → editable, with an
// audit line; default → editable, no override yet. A self-healed db value (bad db,
// fell through to default) is reported as ProvenanceDefault with Caution set.
type Provenance string

const (
	ProvenanceEnv     Provenance = "env"
	ProvenanceDB      Provenance = "db"
	ProvenanceDefault Provenance = "default"
)

// Resolved is one setting's live value plus how it was resolved. Value is the
// typed value (per Kind); for secrets, callers must mask it before display (§4).
type Resolved struct {
	Setting    Setting
	Value      any
	Provenance Provenance
	Caution    bool // a db value failed to parse and self-healed to default (§3)
}

// Loader reads persisted overrides. The store implements it (accept-interfaces
// idiom — settings does not import store). Returns the raw string and whether the
// key had a stored value.
type Loader interface {
	Load(ctx context.Context, key string) (value string, ok bool, err error)
	LoadAll(ctx context.Context) (map[string]string, error)
}

// Change is emitted on Watch channels when a watched key's value changes, so
// long-lived constructions (the LLM client, §8.1) can rebuild (config-design §3).
type Change struct {
	Key string
}

// Service is the settings runtime (config-design §3): it holds an in-memory
// snapshot resolved env > db > default, refreshed on write, and answers reads
// without touching the DB on the hot path. Safe for concurrent use.
type Service struct {
	reg    *Registry
	env    func(string) (string, bool) // injectable for tests; defaults to os.LookupEnv
	log    *slog.Logger
	loader Loader // kept for the post-write snapshot refresh (Patch hot-apply)

	// execProbe reports whether a path is a runnable executable. Injectable so the
	// `ingest` feature gate (the one environment-derived gate, config-design §7) can be
	// tested without touching the filesystem. nil ⇒ the real os.Stat probe.
	execProbe func(path string) bool

	mu       sync.RWMutex
	db       map[string]string // persisted overrides (raw strings)
	watchers map[string][]chan Change
}

// New builds a Service over the registry, loading the current db overrides and
// validating every env pin up front (config-design §3: an invalid env value fails
// the boot, loudly). Returns an error meant to abort startup.
func New(ctx context.Context, reg *Registry, loader Loader, log *slog.Logger) (*Service, error) {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{
		reg:      reg,
		env:      os.LookupEnv,
		log:      log,
		loader:   loader,
		watchers: make(map[string][]chan Change),
	}
	// Validate every env pin at boot — an operator typo must fail here, not lurk.
	for _, set := range reg.All() {
		raw, present, err := s.envRaw(set)
		if err != nil {
			return nil, err
		}
		if !present {
			continue
		}
		// An empty pin is ignored (config-design §3), but never silently: an operator
		// who *meant* to blank the setting must not have to infer that we disregarded
		// them from the absence of any effect.
		if raw == "" {
			log.Warn("ignoring empty env pin — falling through to the database, then the default",
				"var", set.EnvVar, "key", set.Key)
			continue
		}
		if _, perr := set.parse(raw); perr != nil {
			return nil, fmt.Errorf("invalid env %s: %w", set.EnvVar, perr)
		}
	}
	db, err := loader.LoadAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load settings overrides: %w", err)
	}
	s.db = db
	return s, nil
}

// envValue reads a setting's env pin and applies the empty-is-unset rule
// (config-design §3), so an unfilled `.env` template line cannot shadow a value the
// operator saved through the UI. Returns (value, isSet, error).
func (s *Service) envValue(set Setting) (string, bool, error) {
	raw, present, err := s.envRaw(set)
	if err != nil || !present || raw == "" {
		return "", false, err
	}
	return raw, true, nil
}

// envRaw resolves the pin *before* the empty-is-unset rule, honoring the
// Docker-secrets idiom (config-design §3): <VAR> or <VAR>_FILE supplies the value.
// Split out from envValue so boot can tell "no pin" from "a pin we deliberately
// ignored" and warn about the latter — silently dropping it is what made the
// original bug invisible.
//
// An empty <VAR> does not count for the ambiguity check either: `LIBRARY_TOKEN=`
// left in a template alongside a real LIBRARY_TOKEN_FILE from Docker secrets is a
// plausible combination, and failing the boot over it would punish the same blank
// line this rule exists to forgive.
func (s *Service) envRaw(set Setting) (string, bool, error) {
	direct, hasDirect := s.env(set.EnvVar)
	filePath, hasFile := s.env(set.EnvVar + "_FILE")
	switch {
	// Only a NON-EMPTY <VAR> conflicts with <VAR>_FILE; an empty one falls through to
	// the file below. Presence is otherwise reported verbatim — applying the
	// empty-is-unset rule here would hide the empty pin from the boot warning too.
	case hasDirect && direct != "" && hasFile:
		return "", false, fmt.Errorf("%s and %s_FILE both set (ambiguous)", set.EnvVar, set.EnvVar)
	case hasFile:
		b, err := os.ReadFile(filePath) //nolint:gosec // path is operator-supplied config, by design
		if err != nil {
			return "", false, fmt.Errorf("read %s_FILE (%s): %w", set.EnvVar, filePath, err)
		}
		return strings.TrimRight(string(b), "\n"), true, nil
	case hasDirect:
		return direct, true, nil
	default:
		return "", false, nil
	}
}

// SetDB replaces the in-memory override snapshot after a write (hot-apply), and
// notifies watchers of changed keys. Called by the write path once the DB commit
// succeeds so the snapshot never leads the durable store.
func (s *Service) SetDB(next map[string]string) {
	s.mu.Lock()
	changed := changedKeys(s.db, next)
	s.db = next
	// Snapshot watcher channels under the lock; send after releasing it.
	var sends []struct {
		ch  chan Change
		key string
	}
	for _, k := range changed {
		for _, ch := range s.watchers[k] {
			sends = append(sends, struct {
				ch  chan Change
				key string
			}{ch, k})
		}
	}
	s.mu.Unlock()
	for _, snd := range sends {
		select {
		case snd.ch <- Change{Key: snd.key}:
		default: // non-blocking: a slow watcher never stalls a save
		}
	}
}

// Watch returns a channel that receives a Change whenever any of the given keys'
// stored values change (config-design §3 hot-apply for long-lived clients).
func (s *Service) Watch(keys ...string) <-chan Change {
	ch := make(chan Change, 8)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range keys {
		s.watchers[k] = append(s.watchers[k], ch)
	}
	return ch
}

// LoadRaw returns the raw stored value for a key from the snapshot, whether or
// not it is a declared registry setting. Used for namespaced keys the registry
// doesn't declare (e.g. the per-provider llm.api_key.<provider>, §8.1). Returns
// ("", ErrNoValue) when unset. The env tier does NOT apply to raw keys — they are
// pure db values.
func (s *Service) LoadRaw(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.db[key]; ok {
		return v, nil
	}
	return "", ErrNoValue
}

// ErrNoValue is returned by LoadRaw when a raw key has no stored value.
var ErrNoValue = fmt.Errorf("settings: no stored value")

func changedKeys(prev, next map[string]string) []string {
	var out []string
	for k, v := range next {
		if prev[k] != v {
			out = append(out, k)
		}
	}
	for k := range prev {
		if _, ok := next[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}
