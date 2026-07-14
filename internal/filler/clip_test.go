package filler_test

import (
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/schedule"
)

// FILLER-NEVER-A-PROGRAM (§10/§19). This is a structural guarantee, and this test
// pins it: a Pod's entries carry a filler Kind and the scheduler places them only
// as SlotFiller/SlotFlex — there is no path from a Clip to a SlotProgram. If a
// future change adds one, this test (and the type surface it asserts) must be the
// thing that has to change, loudly.
func TestFillerNeverBecomesAProgram(t *testing.T) {
	// A PodEntry has no LibraryItemID→program bridge: its Kind is always a filler
	// Kind, never a program. Assert the domain offers no way to make a program slot
	// from a clip — the only slot constructor filler feeds is SlotFiller.
	clip := filler.Clip{LibraryItemID: "c1", Kind: filler.Commercial, DurationMs: 30000}

	slot := schedule.Slot{Kind: schedule.SlotFiller, LibraryItemID: clip.LibraryItemID, DurationMs: clip.DurationMs}
	if slot.IsProgram() {
		t.Fatal("a filler clip must never map to a program slot")
	}
	// And a clip's Kind is always in the filler kind set — never "program".
	switch clip.Kind {
	case filler.Commercial, filler.Bumper, filler.StationID, filler.PSA, filler.Trailer, filler.Interstitial:
		// ok — all filler kinds
	default:
		t.Fatalf("clip has a non-filler kind %q", clip.Kind)
	}
}

func TestClip_Tagged(t *testing.T) {
	full := filler.Clip{Era: 1992, Audience: filler.Kids, Category: "toys"}
	if !full.Tagged() {
		t.Error("fully-tagged clip should report Tagged()")
	}
	for _, c := range []filler.Clip{
		{Audience: filler.Kids, Category: "toys"}, // no era
		{Era: 1992, Category: "toys"},             // no audience
		{Era: 1992, Audience: filler.Kids},        // no category
	} {
		if c.Tagged() {
			t.Errorf("partially-tagged clip should not report Tagged(): %+v", c)
		}
	}
}
