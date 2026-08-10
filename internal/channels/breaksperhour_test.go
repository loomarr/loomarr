package channels_test

import (
	"testing"

	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/schedule"
)

func ptr(n int) *int { return &n }

// The three states of a per-channel break density (§10 V51f), plus the rule that outranks all
// three.
//
// ⚠ **`0` is the switch that did not exist.** Turning filler off for ONE channel had no
// expression: the only lever was emptying the catalog, which affects every channel. A plain int
// cannot carry it either — "inherit" and "none" are both zero — which is why this is a pointer,
// the same shape `FetchEverySeconds` uses for the same reason.
func TestBreaksPerHourFor(t *testing.T) {
	const global = 4

	for _, tc := range []struct {
		name    string
		policy  *int
		hasPool bool
		want    int
	}{
		{"unset inherits the global", nil, true, global},
		{"a set value overrides the global", ptr(2), true, 2},
		{"zero means NO breaks on this channel", ptr(0), true, 0},

		// ⚠ The dead-air rule wins over every one of them. Break gaps with nothing to fill them
		// leave empty flex that Tunarr renders as large channel-named blocks — a promise of
		// commercials Loomarr cannot keep. The override lowers density; it cannot conjure clips.
		{"no pool beats an inherited density", nil, false, 0},
		{"no pool beats an explicit density", ptr(6), false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pol := schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{BreaksPerHour: tc.policy}}
			if got := channels.BreaksPerHourFor(pol, tc.hasPool, global); got != tc.want {
				t.Errorf("breaks/hour = %d, want %d", got, tc.want)
			}
		})
	}
}

// ⚠ A negative value is treated as "none" rather than passed through. It cannot arrive from the
// UI, but a hand-edited policy_json can carry one, and a negative density reaching the scheduler
// is an arithmetic question nobody has an answer for.
func TestBreaksPerHourFor_NegativeIsNone(t *testing.T) {
	pol := schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{BreaksPerHour: ptr(-3)}}
	if got := channels.BreaksPerHourFor(pol, true, 4); got != 0 {
		t.Errorf("breaks/hour = %d, want 0 for a negative override", got)
	}
}
