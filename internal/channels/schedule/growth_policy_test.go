package schedule

import (
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
)

const (
	seedKey  provision.Key = "series:tvdb:90000801"
	grownKey provision.Key = "series:tvdb:90000802"
	otherKey provision.Key = "series:tvdb:90000803"
)

func growthLineup() []LineupEntry {
	return []LineupEntry{{Key: seedKey}, {Key: grownKey, Grown: "growth-2026-11-16"}, {Key: otherKey, Grown: "growth-2026-11-16"}}
}

// Taking a grown title off is the undo: it is declined so the next night leaves it out. Taking a
// seed off declines nothing, and an edit that keeps every title leaves the switches as they were.
func TestRemovingAGrownTitleDeclinesIt(t *testing.T) {
	before := growthLineup()

	p := ChannelPolicy{}.WithRemovedGrowthDeclined(before, []LineupEntry{{Key: otherKey}, {Key: seedKey}})
	if !p.Declined(grownKey) || p.Declined(seedKey) || p.Declined(otherKey) {
		t.Fatalf("declined %+v, want only the removed grown title", p.Growth)
	}

	if p := (ChannelPolicy{}).WithRemovedGrowthDeclined(before, []LineupEntry{{Key: grownKey}, {Key: otherKey}}); p.Growth != nil {
		t.Errorf("removing a seed declined %v", p.Growth.Declined)
	}

	keep := ChannelPolicy{OperatorPolicy: OperatorPolicy{Growth: &GrowthPolicy{Keep: true, Declined: []provision.Key{otherKey}}}}
	reordered := slices.Clone(before)
	slices.Reverse(reordered)
	if p := keep.WithRemovedGrowthDeclined(before, reordered); !p.Growth.Keep || len(p.Growth.Declined) != 1 {
		t.Errorf("a reorder changed the switches: %+v", p.Growth)
	}
	if p := keep.WithRemovedGrowthDeclined(before, before[:1]); !p.Growth.Keep || len(p.Growth.Declined) != 2 {
		t.Errorf("removing both grown titles: %+v, want Keep kept and one new refusal beside the old", p.Growth)
	}
	if len(keep.Growth.Declined) != 1 {
		t.Error("the caller's policy was changed in place")
	}
}

// A refusal list at its cap forgets its oldest refusal first.
func TestDeclinedForgetsTheOldestPastItsCap(t *testing.T) {
	full := make([]provision.Key, maxDeclined)
	for i := range full {
		full[i] = provision.Key("movie:tmdb:" + string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + string(rune('a'+i/676)))
	}
	p := ChannelPolicy{OperatorPolicy: OperatorPolicy{Growth: &GrowthPolicy{Declined: full}}}
	p = p.WithRemovedGrowthDeclined(growthLineup(), []LineupEntry{{Key: seedKey}, {Key: otherKey}})
	if len(p.Growth.Declined) != maxDeclined || p.Growth.Declined[0] != full[1] || !p.Declined(grownKey) {
		t.Errorf("len %d first %q; want the cap, the oldest dropped and the new refusal kept", len(p.Growth.Declined), p.Growth.Declined[0])
	}
}

// A hand edit replaces the lineup with entries that carry no batch; each title it keeps stays in
// the batch it joined with, so a later removal is still recognised as an undo.
func TestAHandEditKeepsEachTitlesBatch(t *testing.T) {
	out := ApplyLineup(growthLineup(), []LineupEntry{{Key: otherKey}, {Key: seedKey}, {Key: grownKey}}, LineupReplace,
		ApplyOpts{PreserveByKey: true, Now: time.Unix(1_800_000_000, 0)})
	got := map[provision.Key]string{}
	for _, e := range out {
		got[e.Key] = e.Grown
	}
	if got[seedKey] != "" || got[grownKey] != "growth-2026-11-16" || got[otherKey] != "growth-2026-11-16" {
		t.Errorf("batches after a reorder: %v", got)
	}
}

// A Christmas channel grows only by what fits Christmas, whatever its genres; an ordinary channel
// takes any comedy, and a channel for another holiday refuses the Christmas film.
func TestAHolidayChannelGrowsOnlyByItsHoliday(t *testing.T) {
	vocab := holidayvocab.Default()
	xmas := LineupEntry{Key: "movie:tmdb:90000901", Title: "A Puddleford Christmas", Genres: []string{"Comedy", "Family"}}
	plain := LineupEntry{Key: "movie:tmdb:90000902", Title: "The Puddleford Bake-Off", Genres: []string{"Comedy", "Family"}}
	christmas := ChannelPolicy{}
	christmas.Seasonal = SeasonalPolicy{Mode: SeasonalExclusive, Holidays: []string{"christmas"}}
	if !FitsHolidayChannel(xmas, christmas, vocab) || FitsHolidayChannel(plain, christmas, vocab) {
		t.Error("a Christmas channel must take the Christmas comedy and refuse the plain one")
	}
	if !FitsHolidayChannel(plain, ChannelPolicy{}, vocab) {
		t.Error("an ordinary channel refused a title on a holiday rule")
	}
	halloween := christmas
	halloween.Seasonal.Holidays = []string{"halloween"}
	if FitsHolidayChannel(xmas, halloween, vocab) {
		t.Error("a Halloween channel took a Christmas film")
	}
}

// A PATCH that says nothing of growth keeps the channel's switches and refusals.
func TestMergeFromOperator_OmittedGrowthKeepsTheSwitches(t *testing.T) {
	current := ChannelPolicy{OperatorPolicy: OperatorPolicy{Growth: &GrowthPolicy{Keep: true, Declined: []provision.Key{grownKey}}}}
	out := current.MergeFromOperator(ChannelPolicy{ProposalPolicy: ProposalPolicy{Ordering: OrderSequential}})
	if out.Growth == nil || !out.Growth.Keep || !out.Declined(grownKey) {
		t.Errorf("growth after an unrelated PATCH: %+v", out.Growth)
	}
}
