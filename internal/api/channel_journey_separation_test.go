package api_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit/channeljourney"
)

func TestReferenceChannelSeparation(t *testing.T) {
	tc := channeljourney.SeparatedNamedBlock()
	ch, preview, public := approveReferenceJourney(t, tc)
	if ch.Policy.Separation != tc.Proposal.Policy.Separation || public.Policy.Separation != ch.Policy.Separation {
		t.Fatal("ordinary approval or public response lost the explicit separation policy")
	}
	if len(ch.Policy.Applied) != 0 || len(public.Policy.Applied) != 0 {
		t.Fatalf("satisfiable pool reported relaxation: stored=%v public=%v", ch.Policy.Applied, public.Policy.Applied)
	}
	programs := referencePrograms(preview)
	if !slices.Equal(referencePrograms(ch.Desired), programs) {
		t.Fatal("persisted programme schedule differs from playback preview")
	}
	if err := referenceSeparationError(programs, tc.Want, ch.Policy.Separation); err != nil {
		t.Fatal(err)
	}

	// Challenge the assertions with defects in the actual emitted schedule. These
	// are observations only; no production policy, authority, or guard is changed.
	t.Run("missing_program", func(t *testing.T) {
		if err := referenceSeparationError(programs[1:], tc.Want, ch.Policy.Separation); err == nil {
			t.Fatal("missing expected episode passed")
		}
	})
	t.Run("repeated_program", func(t *testing.T) {
		bad := slices.Clone(programs)
		bad[1] = bad[0]
		if err := referenceSeparationError(bad, tc.Want, ch.Policy.Separation); err == nil {
			t.Fatal("duplicate episode passed")
		}
	})
	t.Run("adjacent_series", func(t *testing.T) {
		bad := slices.Clone(programs)
		for i := 2; i < len(bad); i++ {
			if bad[i].Key == bad[0].Key {
				bad[1], bad[i] = bad[i], bad[1]
				break
			}
		}
		if err := referenceSeparationError(bad, tc.Want, ch.Policy.Separation); err == nil {
			t.Fatal("adjacent same-series episodes passed")
		}
	})
	t.Run("gap_across_wrap", func(t *testing.T) {
		bad := slices.Clone(programs)
		bad[len(bad)-1].DurationMs = time.Minute.Milliseconds()
		policy := ch.Policy.Separation
		// Keep total runtime above this control's repeat window so only the
		// shortened last-to-first gap can fail. Interior gaps are unchanged.
		policy.EpisodeNoRepeat = schedule.Duration(time.Hour)
		err := referenceSeparationError(bad, tc.Want, policy)
		if err == nil || !strings.Contains(err.Error(), "cycle wrap") {
			t.Fatalf("wrap-only timing defect was not detected at the seam: %v", err)
		}
	})
}

func TestReferenceChannelSparseRepeatIsExplained(t *testing.T) {
	tc := channeljourney.SparseSingleShow()
	ch, preview, public := approveReferenceJourney(t, tc)
	if ch.Policy.Separation != tc.Proposal.Policy.Separation || public.Policy.Separation != ch.Policy.Separation {
		t.Fatal("relaxation overwrote the approved repeat policy")
	}
	want := []schedule.AppliedRelaxation{{Kind: "episodeNoRepeat", From: "48h0m0s", To: "24h0m0s"}}
	if !slices.Equal(ch.Policy.Applied, want) || !slices.Equal(public.Policy.Applied, want) {
		t.Fatalf("sparse-pool explanation: stored=%v public=%v want=%v", ch.Policy.Applied, public.Policy.Applied, want)
	}
	programs := referencePrograms(preview)
	if !slices.Equal(referencePrograms(ch.Desired), programs) {
		t.Fatal("sparse persisted schedule differs from preview")
	}
	var ids []string
	var runtime int64
	for _, slot := range programs {
		ids = append(ids, slot.LibraryItemID)
		runtime += slot.DurationMs
	}
	if !slices.Equal(ids, tc.Want) || runtime != (44*time.Minute).Milliseconds() {
		t.Fatalf("sparse cycle lost order, season exclusion or actual runway: %v, %d ms", ids, runtime)
	}
}

func TestReferenceChannelMovieRepeat(t *testing.T) {
	tc := channeljourney.SeparatedMovies()
	ch, preview, public := approveReferenceJourney(t, tc)
	if ch.Policy.Separation != tc.Proposal.Policy.Separation || public.Policy.Separation != ch.Policy.Separation {
		t.Fatal("approval or public response lost the movie repeat window")
	}
	if len(ch.Policy.Applied) != 0 || len(public.Policy.Applied) != 0 {
		t.Fatalf("satisfiable movie window was relaxed: stored=%v public=%v", ch.Policy.Applied, public.Policy.Applied)
	}
	programs := referencePrograms(preview)
	if !slices.Equal(referencePrograms(ch.Desired), programs) {
		t.Fatal("movie persisted schedule differs from preview")
	}
	var ids []string
	var runtime int64
	for _, slot := range programs {
		ids = append(ids, slot.LibraryItemID)
		runtime += slot.DurationMs
	}
	if !slices.Equal(ids, tc.Want) || runtime != ch.Policy.Separation.MovieNoRepeat.Std().Milliseconds() {
		t.Fatalf("movie identities/order or repeat interval differ: %v, %d ms", ids, runtime)
	}
}

func referencePrograms(slots []schedule.Slot) []schedule.Slot {
	var programs []schedule.Slot
	for _, slot := range slots {
		if slot.IsProgram() {
			programs = append(programs, slot)
		}
	}
	return programs
}

// Walk two observed cycles forward to check the public contract independently of
// the scheduler's backward-search placement/checker implementation.
func referenceSeparationError(programs []schedule.Slot, want []string, policy schedule.SeparationPolicy) error {
	if len(programs) != len(want) || len(programs) == 0 {
		return fmt.Errorf("programme count = %d, want %d", len(programs), len(want))
	}
	seen := make(map[string]bool)
	var cycleMs int64
	for _, slot := range programs {
		if !slices.Contains(want, slot.LibraryItemID) || seen[slot.LibraryItemID] {
			return fmt.Errorf("unexpected or repeated episode %q", slot.LibraryItemID)
		}
		seen[slot.LibraryItemID] = true
		cycleMs += slot.DurationMs
	}
	if time.Duration(cycleMs)*time.Millisecond < policy.EpisodeNoRepeat.Std() {
		return fmt.Errorf("cycle repeats before the approved episode window")
	}
	lastEnd := make(map[provision.Key]int64)
	var now int64
	var previous provision.Key
	run := 0
	for i := 0; i < 2*len(programs); i++ {
		slot := programs[i%len(programs)]
		where := "cycle interior"
		if i >= len(programs) {
			where = "cycle wrap"
		}
		if end, ok := lastEnd[slot.Key]; ok && now-end < policy.SeriesMinGap.Std().Milliseconds() {
			return fmt.Errorf("%s: series %s gap = %d ms", where, slot.Key, now-end)
		}
		if slot.Key == previous {
			run++
		} else {
			run = 1
		}
		if run > policy.BlockMax {
			return fmt.Errorf("%s: series block exceeds %d", where, policy.BlockMax)
		}
		now += slot.DurationMs
		lastEnd[slot.Key], previous = now, slot.Key
	}
	return nil
}
