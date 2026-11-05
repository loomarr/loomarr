package api_test

import "github.com/loomarr/loomarr/internal/settings"

// mapSettings is a settings.Reader answering from a fixed map; a key it does not hold is
// unanswered (nil), so a handle reads that setting's declared default.
type mapSettings map[string]any

func (m mapSettings) Value(key string) any { return m[key] }

// stringSettings adapts a string-valued stub to a settings.Reader for tests that only configure
// string settings. A nil function is a nil reader, the unit-test convention for "settings unwired".
func stringSettings(fn func(key string) string) settings.Reader {
	if fn == nil {
		return nil
	}
	return stringReader(fn)
}

type stringReader func(key string) string

func (f stringReader) Value(key string) any { return f(key) }
