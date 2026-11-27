package filler

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
)

// One clean spot, judged so by the look and by its sound, cut at corroborated edges: the only
// shape cut unattended.
func cleanCut() split.Segment {
	return split.Segment{
		StartMs: 0, EndMs: 30_000, Looked: true, BoundaryConfidence: 90,
		Whole: split.WholeOne, LookedRole: split.RoleCommercial, Sound: cleanSound(),
	}
}

// cleanSound is a cut that opens and closes on quiet with sound throughout.
func cleanSound() *split.CutSound {
	return &split.CutSound{MedianDb: -22, StartDb: -40, EndDb: -40, InsideDb: -6}
}

func TestAutoCuttable_EachRefusalDropsOnlyItsCut(t *testing.T) {
	pol := &AutoSplitPolicy{
		Enabled:       func() bool { return true },
		MinConfidence: func() int { return 85 },
		MaxDuration:   func() time.Duration { return 2 * time.Minute },
	}
	cases := []struct {
		name string
		edit func(*split.Segment)
		want AutoSplitReject
	}{
		{"clean", func(*split.Segment) {}, AutoSplitOK},
		{"unsplittable", func(s *split.Segment) { s.Unsplittable = true }, RejectUnsplittable},
		{"duplicate", func(s *split.Segment) { s.DupOf = "old/ad.mp4" }, RejectDuplicate},
		{"dedup unchecked", func(s *split.Segment) { s.DedupUnchecked = true }, RejectDedupUnchecked},
		{"too long", func(s *split.Segment) { s.EndMs = 121_000 }, RejectTooLong},
		{"too short", func(s *split.Segment) { s.EndMs = 9_000 }, RejectTooShort},
		{"one detector", func(s *split.Segment) { s.BoundaryConfidence = 65 }, RejectBoundaryUncertain},
		{"sound unmeasured", func(s *split.Segment) { s.Sound = nil }, RejectSoundNotClean},
		{"sound runs through an edge", func(s *split.Segment) { s.Sound.EndDb = -3 }, RejectSoundNotClean},
		{"quiet inside", func(s *split.Segment) { s.Sound.InsideDb = -30 }, RejectSoundNotClean},
		{"no sound", func(s *split.Segment) { s.Sound.MedianDb = split.SoundSilentDb }, RejectSoundNotClean},
		// The look still decides: clean sound never makes a cut the look did not see as one spot.
		{"clean sound, look saw part", func(s *split.Segment) { s.Whole = split.WholePart }, RejectNotOneSpot},
		{"programme", func(s *split.Segment) { s.LookedRole = split.RoleProgrammeFragment }, RejectNotFiller},
		{"never looked", func(s *split.Segment) { s.LookedRole, s.Whole = "", "" }, RejectNotFiller},
		{"part", func(s *split.Segment) { s.Whole = split.WholePart }, RejectNotOneSpot},
		{"several", func(s *split.Segment) { s.Whole = split.WholeSeveral }, RejectNotOneSpot},
		{"unsure", func(s *split.Segment) { s.Whole = split.WholeUnsure }, RejectNotOneSpot},
		{"legacy look", func(s *split.Segment) { s.Whole = "" }, RejectNotOneSpot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := cleanCut()
			tc.edit(&bad)
			good := cleanCut()
			good.StartMs, good.EndMs = 40_000, 70_000
			part := AutoCuttable(SplitProposal{Segments: []split.Segment{bad, good}}, pol, 10*time.Second)
			if part.Reject != AutoSplitOK {
				t.Fatalf("whole-proposal reject %q", part.Reject)
			}
			if tc.want == AutoSplitOK {
				if len(part.Confirm) != 2 {
					t.Fatalf("partition = %+v, want both cut", part)
				}
				return
			}
			if len(part.Confirm) != 1 || part.Confirm[0].StartMs != 40_000 {
				t.Fatalf("partition = %+v, want only the clean cut made", part)
			}
			if len(part.Hold) != 1 || part.Hold[0].HoldReason != string(tc.want) {
				t.Fatalf("hold = %+v, want %q", part.Hold, tc.want)
			}
		})
	}
}

func TestAutoCuttable_OffHoldsEverythingForThePerson(t *testing.T) {
	pol := &AutoSplitPolicy{Enabled: func() bool { return false }}
	part := AutoCuttable(SplitProposal{Segments: []split.Segment{cleanCut()}}, pol, 0)
	if part.Reject != RejectDisabled || len(part.Confirm) != 0 || len(part.Hold) != 1 {
		t.Fatalf("partition = %+v", part)
	}
}

// A stored proposal can still carry a confirmed complete-timeline decision. It authorizes no cut:
// the gate decides each cut from the look, and segments nobody looked at are held, however clean
// their edges and sound.
func TestAutoCuttable_AConfirmedStructureDecisionCutsNothingUnlooked(t *testing.T) {
	proposal := decidedStructureProposal(t)
	if proposal.StructureDecision == nil || proposal.Structure == nil {
		t.Fatal("fixture carries no confirmed structure decision")
	}
	for i := range proposal.Segments {
		proposal.Segments[i].Sound = cleanSound()
		if proposal.Segments[i].Looked || proposal.Segments[i].LookedRole != "" {
			t.Fatalf("fixture segment %d was looked at: %+v", i, proposal.Segments[i])
		}
	}
	pol := &AutoSplitPolicy{
		Enabled:       func() bool { return true },
		MinConfidence: func() int { return 85 },
		MaxDuration:   func() time.Duration { return 2 * time.Minute },
	}
	part := AutoCuttable(proposal, pol, 10*time.Second)
	if part.Reject != AutoSplitOK || len(part.Confirm) != 0 || len(part.Hold) != len(proposal.Segments) {
		t.Fatalf("partition = %+v, want every segment held", part)
	}
	for _, held := range part.Hold {
		if held.HoldReason != string(RejectNotFiller) {
			t.Errorf("segment at %d ms held as %q, want %q", held.StartMs, held.HoldReason, RejectNotFiller)
		}
	}
}

func TestParseWholeCutTreatsAnythingUnknownAsUnsure(t *testing.T) {
	for in, want := range map[string]split.WholeCut{
		"one": split.WholeOne, " One ": split.WholeOne, "part": split.WholePart,
		"several": split.WholeSeveral, "": split.WholeUnsure, "two": split.WholeUnsure, "yes": split.WholeUnsure,
	} {
		if got := split.ParseWholeCut(in); got != want {
			t.Errorf("ParseWholeCut(%q) = %q, want %q", in, got, want)
		}
	}
}
