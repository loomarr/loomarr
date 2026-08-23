package channels_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/schedule"
)

// The three states of a filler era (§10, V51f), and the reason the middle one had to exist.
//
// ⚠ **"Unset" and "any" were the same value, so one of them was unreachable.** The scope default
// keyed off `sel.Era == 0`, which is what a cleared field and a deliberate "draw from everything"
// both look like — so clearing the field re-inherited the channel's programming era on the very
// next derivation, and a channel with a scope era had no way to say "any". Presence is now the
// opt-in: an absent range inherits, a PRESENT one is the operator's answer even when it is empty.
func TestSelectionFrom_EraHasThreeStates(t *testing.T) {
	scope := &schedule.Range{From: 1990, To: 1999}

	for _, tc := range []struct {
		name string
		sel  *schedule.FillerSelection
		want filler.EraRange
	}{
		{
			name: "no filler selection at all inherits the channel's era",
			sel:  nil,
			want: filler.EraRange{From: 1990, To: 1999},
		},
		{
			name: "an unset era inherits the channel's era",
			sel:  &schedule.FillerSelection{Audience: "kids"},
			want: filler.EraRange{From: 1990, To: 1999},
		},
		{
			// The state that did not exist before V51f.
			name: "a present but empty range means ANY era, and does NOT re-inherit",
			sel:  &schedule.FillerSelection{Era: &schedule.Range{}},
			want: filler.EraRange{},
		},
		{
			name: "a set range wins over the channel's era",
			sel:  &schedule.FillerSelection{Era: &schedule.Range{From: 1975, To: 1985}},
			want: filler.EraRange{From: 1975, To: 1985},
		},
		{
			// ⚠ Both bounds, which is the whole defect: `To` was rendered, typed, canonicalised
			// and inverted-range-validated, then dropped on the floor by every consumer.
			name: "an open-ended range keeps its one bound",
			sel:  &schedule.FillerSelection{Era: &schedule.Range{From: 1980}},
			want: filler.EraRange{From: 1980},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := channels.SelectionFrom(tc.sel, scope).Era
			if got != tc.want {
				t.Errorf("era = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A channel with no programming era of its own has nothing to inherit, so an unset filler era is
// simply "any" — the additive default an empty policy has always promised.
func TestSelectionFrom_NoScopeEraLeavesTheRangeOpen(t *testing.T) {
	if got := channels.SelectionFrom(nil, nil).Era; !got.Any() {
		t.Errorf("era = %+v, want the open range", got)
	}
}
