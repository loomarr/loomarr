package channels

import (
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
)

// staleProgramCount counts how many previously-scheduled programs vanished from
// the freshly-computed lineup (§9 slot revalidation → §17 slot-substitution
// count). It counts each vanished key, and an empty prev (first reconcile) can't
// drift.
func TestStaleProgramCount(t *testing.T) {
	prog := func(k string) schedule.Slot {
		return schedule.Slot{Kind: schedule.SlotProgram, Key: provision.Key(k)}
	}
	pending := schedule.Slot{Kind: schedule.SlotPending}

	prev := []schedule.Slot{prog("movie:tmdb:1"), prog("movie:tmdb:2"), prog("movie:tmdb:3")}

	cases := []struct {
		name string
		next []schedule.Slot
		want int
	}{
		{"none vanished", prev, 0},
		{"one vanished", []schedule.Slot{prog("movie:tmdb:1"), prog("movie:tmdb:2")}, 1},
		{"two vanished", []schedule.Slot{prog("movie:tmdb:1")}, 2},
		{"a program became a placeholder", []schedule.Slot{prog("movie:tmdb:1"), prog("movie:tmdb:2"), pending}, 1},
		{"first reconcile can't drift", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got int
			if c.name == "first reconcile can't drift" {
				got = staleProgramCount(nil, prev)
			} else {
				got = staleProgramCount(prev, c.next)
			}
			if got != c.want {
				t.Errorf("staleProgramCount = %d, want %d", got, c.want)
			}
		})
	}
}
