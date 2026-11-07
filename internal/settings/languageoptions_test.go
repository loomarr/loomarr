package settings

import (
	"testing"

	"github.com/loomarr/loomarr/internal/langcode"
)

// The picker's codes are what a channel stores and what the detector writes, so each must already
// be the canonical spelling: a `he` that normalized to `iw` would match no clip.
func TestFillerLanguageOptionsAreCanonicalCodes(t *testing.T) {
	for _, o := range fillerLanguageOptions() {
		if !langcode.Valid(o.Value) || langcode.Normalize(o.Value) != o.Value {
			t.Errorf("language option %q: Valid=%v Normalize=%q", o.Value, langcode.Valid(o.Value), langcode.Normalize(o.Value))
		}
	}
}
