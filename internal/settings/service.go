package settings

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/library"
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
	// EnvOverride: this key was taken back from the environment (§3.1), so the env var is
	// set but deliberately not winning. Reported ALONGSIDE Provenance rather than as a
	// fourth provenance value: such a key resolves honestly to `db`, and the UI needs both
	// facts to say "overriding SEERR_URL" instead of implying the variable is unset.
	EnvOverride bool
}

// Snapshot is one coherent durable settings generation. Values and environment-
// override ownership travel together because resolving them from separate reads
// could pair a new value with old authority during another replica's PATCH.
type Snapshot struct {
	Values       map[string]string
	EnvOverrides map[string]bool
}

// Loader reads one coherent persisted settings generation. The store implements
// it through one ListSettings query (accept-interfaces; settings does not import
// store), so refresh cost is one table read rather than one read per key.
type Loader interface {
	LoadSnapshot(ctx context.Context) (Snapshot, error)
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
	loader Loader // kept for post-write hot-apply and replica refresh

	// execProbe reports whether a path is a runnable executable. Injectable so the
	// `ingest` feature gate (the one environment-derived gate, config-design §7) can be
	// tested without touching the filesystem. nil ⇒ the real os.Stat probe.
	execProbe func(path string) bool

	// execLookPath resolves a bare tool name the way a shell would. Injectable for the same
	// reason as execProbe, and it has to be: faking only the PROBE left `exec.LookPath` running
	// for real, so `TestFeatures_UnsetToolPathsFallBackToPathLookup` passed on any machine with
	// ffmpeg installed and failed on every CI runner without it. A test that depends on the
	// host's PATH is not hermetic, and this one hid that for a whole release.
	// nil ⇒ the real exec.LookPath.
	execLookPath func(name string) (string, error)

	mu sync.RWMutex
	// refreshMu serializes durable reloads. Without it, a slow read that began before
	// a local PATCH committed could publish its old generation after the PATCH's own
	// reload and temporarily roll this process backward.
	refreshMu sync.Mutex
	db        map[string]string // persisted overrides (raw strings)
	// unlocked holds the keys taken back from the environment (§3.1). Only true entries
	// are present, so a missing key means "env still wins" — the default and the norm.
	unlocked map[string]bool
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
	snapshot, err := loader.LoadSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("load settings overrides: %w", err)
	}
	s.db = snapshot.Values
	s.unlocked = snapshot.EnvOverrides
	if s.db == nil {
		s.db = map[string]string{}
	}
	if s.unlocked == nil {
		s.unlocked = map[string]bool{}
	}
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

// IsUnlocked reports whether a key has been taken back from the environment (§3.1).
func (s *Service) IsUnlocked(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.unlocked[key]
}

// SetDB replaces the in-memory override snapshot after a write (hot-apply), and
// notifies watchers of changed keys. Called by the write path once the DB commit
// succeeds so the snapshot never leads the durable store.
func (s *Service) SetDB(next map[string]string) {
	s.publishSnapshot(Snapshot{Values: next})
}

// setSnapshot atomically publishes values and environment-override ownership from
// one durable generation. ResolveMany therefore sees either the complete old
// snapshot or the complete new one, never a tuple assembled across refreshes.
func (s *Service) setSnapshot(next Snapshot) {
	if next.Values == nil {
		next.Values = map[string]string{}
	}
	if next.EnvOverrides == nil {
		next.EnvOverrides = map[string]bool{}
	}
	s.publishSnapshot(next)
}

// publishSnapshot is the one publication path for values, authority, and watcher
// notifications. A nil EnvOverrides map means a values-only SetDB update and
// preserves current authority; durable snapshots are normalized by setSnapshot.
func (s *Service) publishSnapshot(next Snapshot) {
	if next.Values == nil {
		next.Values = map[string]string{}
	}
	s.mu.Lock()
	if next.EnvOverrides == nil {
		next.EnvOverrides = s.unlocked
	}
	changed := changedKeys(s.db, next.Values)
	changedSet := make(map[string]struct{}, len(changed))
	for _, key := range changed {
		changedSet[key] = struct{}{}
	}
	// Authority is part of the live resolution snapshot too. Re-locking a key can
	// move it from a stored value back to a different env value even though the raw
	// DB row did not change, so watchers must rebuild on ownership changes.
	for key, wasUnlocked := range s.unlocked {
		if wasUnlocked != next.EnvOverrides[key] {
			if _, exists := changedSet[key]; !exists {
				changed = append(changed, key)
				changedSet[key] = struct{}{}
			}
		}
	}
	for key, unlocked := range next.EnvOverrides {
		if unlocked != s.unlocked[key] {
			if _, exists := changedSet[key]; !exists {
				changed = append(changed, key)
			}
		}
	}
	s.db = next.Values
	s.unlocked = next.EnvOverrides
	var sends []struct {
		ch  chan Change
		key string
	}
	for _, key := range changed {
		for _, ch := range s.watchers[key] {
			sends = append(sends, struct {
				ch  chan Change
				key string
			}{ch: ch, key: key})
		}
	}
	s.mu.Unlock()
	for _, send := range sends {
		select {
		case send.ch <- Change{Key: send.key}:
		default:
		}
	}
}

// RefreshEvery keeps an ordinary Postgres replica's in-memory snapshot within the
// documented refresh bound. It blocks until ctx is cancelled; the composition root
// owns the one goroutine per application generation and does not call this for SQLite.
// A failed read keeps the last complete snapshot and is retried on the next tick.
func (s *Service) RefreshEvery(ctx context.Context, interval time.Duration, after func()) {
	if interval <= 0 {
		panic("settings: refresh interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.refreshOn(ctx, ticker.C, after)
}

// refreshOn is the deterministic clock seam for the periodic refresher's tests.
func (s *Service) refreshOn(ctx context.Context, ticks <-chan time.Time, after func()) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			if err := s.Refresh(ctx); err != nil {
				if ctx.Err() == nil {
					s.log.Warn("settings replica refresh failed; keeping prior snapshot", "err", err)
				}
				continue
			}
			if after != nil {
				after()
			}
		}
	}
}

// LibraryConnection projects one coherent settings snapshot into the canonical
// media-library connection used by runtime adapters and feature gating.
func (s *Service) LibraryConnection() library.Connection {
	values := s.ResolveMany("library.flavor", "library.url", "library.token")
	stringValue := func(key string) string {
		value, _ := values[key].Value.(string)
		return strings.TrimSpace(value)
	}
	flavor, err := library.ParseFlavor(stringValue("library.flavor"))
	if err != nil {
		return library.Connection{}
	}
	return library.Connection{
		Flavor: flavor, BaseURL: stringValue("library.url"), Token: stringValue("library.token"),
	}
}

// Watch returns a channel that receives a Change whenever any of the given keys'
// live resolution inputs change (config-design §3 hot-apply for long-lived clients).
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
