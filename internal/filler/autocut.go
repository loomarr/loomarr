package filler

import (
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
)

// The unattended cut (#4409, #3838 decisions 10, 11 and 19). A compilation splits itself: a cut
// that is one clean piece of filler is cut and then meets the same bar as a single downloaded ad,
// and every other cut is dropped. Nobody reviews.
//
// The bar for the cut itself is about where it was cut and what the whole cut is, never about what
// it advertises: era, audience and category are judged on the cut-out clip by the same rungs a
// single ad runs, after the cut.

const (
	// RejectNotFiller is a cut whose look says it is not break material: programme, other
	// non-filler, ambiguous or unusable.
	RejectNotFiller AutoSplitReject = "the cut is not break material"
	// RejectNotOneSpot is a cut whose look did not see one complete spot: part of one, several, or
	// could not tell. Black and silence cannot see a hard cut between ads, nor tell a dip inside an
	// ad from a gap between two (the split corpus), so this look is what keeps both off the air.
	RejectNotOneSpot AutoSplitReject = "the cut is not one whole spot"
	// RejectSoundNotClean is a cut whose sound does not say one whole spot: it does not open and
	// close on quiet, goes quiet inside, has no sound, or was not measured. Measured, the vision
	// look alone called 16 of 40 cuts that were not one spot `one`; this is the check that holds.
	RejectSoundNotClean AutoSplitReject = "the cut's sound is not one whole spot"
)

// AutoCuttable partitions a proposal into the cuts made unattended (Confirm) and the cuts dropped
// (Hold, each with its reason). It refuses the whole proposal only when splitting is switched off
// or nothing was detected.
func AutoCuttable(p SplitProposal, pol *AutoSplitPolicy, minClipDuration time.Duration) SplitPartition {
	if pol == nil || pol.Enabled == nil || !pol.Enabled() {
		return SplitPartition{Reject: RejectDisabled, Hold: p.Segments}
	}
	if len(p.Segments) == 0 {
		return SplitPartition{Reject: RejectNoSegments}
	}
	maxDur := 120 * time.Second
	if pol.MaxDuration != nil {
		if d := pol.MaxDuration(); d > 0 {
			maxDur = d
		}
	}
	var out SplitPartition
	for _, s := range p.Segments {
		if why := cutVerdict(s, pol, minClipDuration, maxDur); why != AutoSplitOK {
			s.HoldReason = string(why)
			out.Hold = append(out.Hold, s)
			continue
		}
		out.Confirm = append(out.Confirm, s)
	}
	return out
}

// cutVerdict is why one cut may or may not be made unattended. The detector's own refusals and the
// length come first, then where it was cut, then what the whole cut is. Each is a refusal: nothing
// here is a score that could stand in for another.
func cutVerdict(s split.Segment, pol *AutoSplitPolicy, minClipDuration, maxDur time.Duration) AutoSplitReject {
	switch {
	case s.Unsplittable:
		return RejectUnsplittable
	case s.DupOf != "":
		return RejectDuplicate
	case s.DedupUnchecked:
		return RejectDedupUnchecked
	}
	span := time.Duration(s.EndMs-s.StartMs) * time.Millisecond
	switch {
	case span > maxDur:
		return RejectTooLong
	case minClipDuration > 0 && span < minClipDuration:
		return RejectTooShort
	case s.BoundaryConfidence < minConfidence(pol):
		// Both edges corroborated at the default 85: a chapter, the reel's own edge, or black and
		// silence that agree.
		return RejectBoundaryUncertain
	case !s.Sound.Clean():
		return RejectSoundNotClean
	case !split.CertifiedFillerRole(s.LookedRole):
		return RejectNotFiller
	case s.Whole != split.WholeOne:
		return RejectNotOneSpot
	}
	return AutoSplitOK
}
