package settings

import (
	"strings"
	"sync"
	"time"
)

//go:generate go run ../../../cmd/settings-keys keys_gen.go

// Reader answers a setting's resolved value by key. *Service reads the live snapshot; the
// composition root's generation-frozen view also satisfies it. A nil answer means the reader
// cannot say (no store, or a key it does not hold), and every handle then returns the declared
// default — so a degraded boot keeps a default-on feature on instead of silently switching it off.
type Reader interface {
	Value(key string) any
}

// Value is the resolved value of key (env > db > default); nil on a nil service.
func (s *Service) Value(key string) any {
	if s == nil {
		return nil
	}
	return s.Resolve(key).Value
}

// The handle types are generated one per setting in keys_gen.go, typed by the setting's Kind.
// Callers read `settings.LibraryURL.Get(r)` rather than a string key, so a misspelt or retired
// key is a compile error and a kind mismatch cannot exist.
type (
	StringKey     struct{ key string } // string, enum, url, secret and cron settings
	IntKey        struct{ key string }
	BoolKey       struct{ key string }
	DurationKey   struct{ key string }
	StringListKey struct{ key string }
)

func (k StringKey) Key() string     { return k.key }
func (k IntKey) Key() string        { return k.key }
func (k BoolKey) Key() string       { return k.key }
func (k DurationKey) Key() string   { return k.key }
func (k StringListKey) Key() string { return k.key }

func (k StringKey) Get(r Reader) string          { return read[string](r, k.key) }
func (k IntKey) Get(r Reader) int                { return read[int](r, k.key) }
func (k BoolKey) Get(r Reader) bool              { return read[bool](r, k.key) }
func (k DurationKey) Get(r Reader) time.Duration { return read[time.Duration](r, k.key) }
func (k StringListKey) Get(r Reader) []string    { return read[[]string](r, k.key) }

// Joined is the list comma-joined, the spelling env pins and the settings API use.
func (k StringListKey) Joined(r Reader) string { return strings.Join(k.Get(r), ",") }

// declaredDefaults is built once: the registry is immutable, and a handle reads it only when the
// reader cannot answer. A default written as a string ("6h", "2048") is parsed by its Kind, as
// the service's own default resolution does.
var declaredDefaults = sync.OnceValue(func() map[string]any {
	defaults := map[string]any{}
	for _, s := range NewRegistry().All() {
		defaults[s.Key] = s.Default
		if raw, ok := s.Default.(string); ok {
			if parsed, err := s.parse(raw); err == nil {
				defaults[s.Key] = parsed
			}
		}
	}
	return defaults
})

func read[T any](r Reader, key string) T {
	if r != nil {
		if v, ok := r.Value(key).(T); ok {
			return v
		}
	}
	v, _ := fallback(key).(T)
	return v
}

func fallback(key string) any { return declaredDefaults()[key] }
