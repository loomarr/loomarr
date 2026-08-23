package channels

import (
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

// staleProgramCount counts how many previously-scheduled programs the library can
// no longer supply (§9 slot revalidation → §17 slot-substitution count). It
// compares prev programs against the freshly-computed ELIGIBLE key set (every key
// the library can currently supply), NOT the aired slots — so a program rotated
// out by a curation rule / seasonal bench / rolling window is NOT drift, while a
// genuinely-missing library title still is (§6.5 drift fix). An empty prev (first
// reconcile) can't drift.
func TestStaleProgramCount(t *testing.T) {
	prog := func(k string) schedule.Slot {
		return schedule.Slot{Kind: schedule.SlotProgram, Key: provision.Key(k)}
	}
	eligible := func(keys ...string) map[provision.Key]bool {
		m := map[provision.Key]bool{}
		for _, k := range keys {
			m[provision.Key(k)] = true
		}
		return m
	}

	prev := []schedule.Slot{prog("movie:tmdb:1"), prog("movie:tmdb:2"), prog("movie:tmdb:3")}

	cases := []struct {
		name     string
		prev     []schedule.Slot
		eligible map[provision.Key]bool
		want     int
	}{
		// All prev keys still eligible ⇒ no drift, regardless of what's aired.
		{"none vanished", prev, eligible("movie:tmdb:1", "movie:tmdb:2", "movie:tmdb:3"), 0},
		// A key genuinely gone from the library (not in eligible) ⇒ drift.
		{"one vanished", prev, eligible("movie:tmdb:1", "movie:tmdb:2"), 1},
		{"two vanished", prev, eligible("movie:tmdb:1"), 2},
		// THE FIX: a rule / window / seasonal bench rotated keys out of the AIRED
		// deck, but they're still eligible (library has them) ⇒ NOT drift. Even though
		// only movie:tmdb:1 would air this window, all three remain eligible.
		{"rule/window rotation is not drift", prev, eligible("movie:tmdb:1", "movie:tmdb:2", "movie:tmdb:3"), 0},
		// First reconcile: empty prev can't drift.
		{"first reconcile can't drift", nil, eligible("movie:tmdb:1"), 0},
		// A nil eligible set (policy-free literal in a test / defensive) ⇒ treat prev as
		// still-eligible (no spurious drift); production always populates EligibleKeys.
		{"nil eligible is not drift", prev, nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := staleProgramCount(c.prev, c.eligible)
			if got != c.want {
				t.Errorf("staleProgramCount = %d, want %d", got, c.want)
			}
		})
	}
}
