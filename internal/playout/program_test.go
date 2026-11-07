package playout

import (
	"strings"
	"testing"
	"time"
)

// seconds must never emit exponent notation — ffmpeg would parse "1e-06" as a token rather
// than a duration.
func TestSeconds_NeverUsesExponentNotation(t *testing.T) {
	for _, d := range []time.Duration{
		time.Microsecond, time.Millisecond, time.Second,
		90*time.Minute + 500*time.Millisecond, 0,
	} {
		got := seconds(d)
		if strings.ContainsAny(got, "eE") {
			t.Errorf("seconds(%v) = %q, which ffmpeg cannot parse", d, got)
		}
	}
}
