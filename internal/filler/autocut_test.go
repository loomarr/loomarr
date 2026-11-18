package filler

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
)

// One clean spot, judged so by the look, cut at corroborated edges: the only shape cut unattended.
func cleanCut() split.Segment {
	return split.Segment{
		StartMs: 0, EndMs: 30_000, Looked: true, BoundaryConfidence: 90,
		Whole: split.WholeOne, LookedRole: split.RoleCommercial,
	}
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
