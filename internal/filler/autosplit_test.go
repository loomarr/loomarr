package filler_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// The auto-confirm gate (§10 V43). ⚠ Every assertion here is about REFUSING to cut. A gate that
// lets a bad reel through consumes the operator's compilation and puts half-adverts on air; a
// gate that refuses a good one costs a click. The tests are weighted accordingly.

func policy(on bool, minConf int, maxDur time.Duration) *filler.AutoSplitPolicy {
	return &filler.AutoSplitPolicy{
		Enabled:       func() bool { return on },
		MinConfidence: func() int { return minConf },
		MaxDuration:   func() time.Duration { return maxDur },
	}
}

// goodSeg is an advert-shaped, classified, grounded segment cut on corroborated boundaries — the
// only shape that qualifies.
//
// ⚠ `BoundaryConfidence` is part of "qualifies" since V54: the gate's LAST check is the score, so
// a segment perfect in every other respect is still held back if Loomarr has no evidence for where
// it was cut. 90 is the "black and silence agreed" ceiling — what a well-detected cut earns.
func goodSeg(i int, startMs, endMs int64) filler.SplitSegment {
	return filler.SplitSegment{
		Index: i, StartMs: startMs, EndMs: endMs,
		Name: "Advert", Era: 1993, Audience: filler.Kids, Category: "toys",
		BoundaryConfidence: 90, StartEvidence: "black + silence", EndEvidence: "black + silence",
	}
}

func proposalOf(segs ...filler.SplitSegment) filler.SplitProposal {
	return filler.SplitProposal{ID: "p1", ClipHash: "reel-hash", Segments: segs}
}

const floor = 10 * time.Second

func TestAutoConfirmable_AcceptsAnUnambiguousReel(t *testing.T) {
	p := proposalOf(
		goodSeg(0, 0, 30_000),
		goodSeg(1, 30_000, 60_000),
		goodSeg(2, 60_000, 105_000),
	)
	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor).Verdict(); got != filler.AutoSplitOK {
		t.Errorf("a clean reel was rejected: %q", got)
	}
}

// ⚠ OFF by default is the safety property, so it is asserted rather than assumed.
func TestAutoConfirmable_RefusesWhenDisabled(t *testing.T) {
	p := proposalOf(goodSeg(0, 0, 30_000))
	if got := filler.AutoConfirmable(p, policy(false, 85, 120*time.Second), floor).Verdict(); got != filler.RejectDisabled {
		t.Errorf("confirmed with the feature off: %q", got)
	}
	// A nil policy is an install that never wired it — must also refuse, not panic.
	if got := filler.AutoConfirmable(p, nil, floor).Verdict(); got != filler.RejectDisabled {
		t.Errorf("nil policy = %q, want a refusal", got)
	}
}

// ⚠ THE case this gate exists for. `Unsplittable` is the detector admitting it could not find a
// boundary; auto-confirming around that admission is exactly the 69% failure §10 measured.
func TestAutoConfirmable_RefusesAnUnsplittableSegment(t *testing.T) {
	bad := goodSeg(1, 30_000, 90_000)
	bad.Unsplittable = true
	p := proposalOf(goodSeg(0, 0, 30_000), bad, goodSeg(2, 90_000, 120_000))

	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor).Verdict(); got != filler.RejectUnsplittable {
		t.Errorf("confirmed a reel containing an unsplittable span: %q", got)
	}
}

// ⚠ **ONE BAD SEGMENT NO LONGER SINKS THE REEL (V54).** This test used to assert the opposite, and
// the rule it pinned is what made compilations the most manual part of the system: a single 5-minute
// block sent 51 good cuts back for review, so the operator's work never shrank and ~50 reels sat
// parked with none ever confirmed.
//
// The bad segment is still refused — that has not moved. What changed is its blast radius.
func TestAutoConfirmable_PartitionsRatherThanSinkingTheReel(t *testing.T) {
	tooLong := goodSeg(2, 60_000, 400_000) // 5m40s — not an advert
	p := proposalOf(goodSeg(0, 0, 30_000), goodSeg(1, 30_000, 60_000), tooLong)

	got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor)

	if len(got.Confirm) != 2 {
		t.Errorf("confirmed %d segments, want the 2 good ones — a doubtful third must not hold them",
			len(got.Confirm))
	}
	if len(got.Hold) != 1 || got.Hold[0].StartMs != 60_000 {
		t.Fatalf("held %+v, want only the 5-minute block", got.Hold)
	}
	// The reason travels WITH the segment, so the review can say why this one is still here.
	if got.Hold[0].HoldReason != string(filler.RejectTooLong) {
		t.Errorf("hold reason = %q, want %q", got.Hold[0].HoldReason, filler.RejectTooLong)
	}
	// …and the reel-level summary still names it, for the ladder note.
	if got.Verdict() != filler.RejectTooLong {
		t.Errorf("verdict = %q, want %q", got.Verdict(), filler.RejectTooLong)
	}
}

// ⚠ **THE laundering guard.** `autosplit.go` argues that inventing a confidence number would
// "launder that refusal into a number". It stays true only because refusals are checked FIRST and
// absolutely: a segment with a guessed era is refused at 100, and `boundaryScore` cannot even see
// the field. If this ever fails, the ordering has been inverted and a score is standing in for a
// refusal.
func TestAutoConfirmable_RefusalsBeatAnyScore(t *testing.T) {
	perfectBoundaries := func(s filler.SplitSegment) filler.SplitSegment {
		s.BoundaryConfidence = 100
		s.StartEvidence, s.EndEvidence = "chapter", "chapter"
		return s
	}

	guessed := perfectBoundaries(goodSeg(0, 0, 30_000))
	guessed.Era, guessed.SuggestedEra = 0, 1985

	unsplittable := perfectBoundaries(goodSeg(1, 30_000, 60_000))
	unsplittable.Unsplittable = true

	dup := perfectBoundaries(goodSeg(2, 60_000, 90_000))
	dup.DupOf = "cc/dd/existing.mp4"

	for _, tc := range []struct {
		name string
		seg  filler.SplitSegment
		want filler.AutoSplitReject
	}{
		{"a guessed era", guessed, filler.RejectUngrounded},
		{"an unsplittable span", unsplittable, filler.RejectUnsplittable},
		{"a known duplicate", dup, filler.RejectDuplicate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := filler.AutoConfirmable(proposalOf(tc.seg), policy(true, 85, 120*time.Second), floor)
			if len(got.Confirm) != 0 {
				t.Fatalf("%s was CONFIRMED at boundary confidence 100 — a score has been allowed to "+
					"override a refusal", tc.name)
			}
			if got.Hold[0].HoldReason != string(tc.want) {
				t.Errorf("hold reason = %q, want %q", got.Hold[0].HoldReason, tc.want)
			}
		})
	}
}

// The threshold's own job: hold back a segment that passes every refusal but was cut somewhere
// Loomarr cannot vouch for.
func TestAutoConfirmable_HoldsBackAnUncertainBoundary(t *testing.T) {
	weak := goodSeg(0, 0, 30_000)
	weak.BoundaryConfidence = 65 // one detector only
	weak.StartEvidence, weak.EndEvidence = "silence only", "silence only"

	got := filler.AutoConfirmable(proposalOf(weak), policy(true, 85, 120*time.Second), floor)

	if len(got.Confirm) != 0 {
		t.Fatal("confirmed a segment on a single detector's word at the default threshold")
	}
	if got.Hold[0].HoldReason != string(filler.RejectBoundaryUncertain) {
		t.Errorf("hold reason = %q, want the boundary reason", got.Hold[0].HoldReason)
	}
	// ⚠ Lowering the threshold is an operator choice and must actually work, or the setting is
	// decoration.
	if lower := filler.AutoConfirmable(proposalOf(weak), policy(true, 50, 120*time.Second), floor); len(lower.Confirm) != 1 {
		t.Errorf("at min=50 a single-detector boundary should clear; confirmed %d", len(lower.Confirm))
	}
}

// ⚠ Every proposal detected before V54 deserialises with no score at all. Auto-confirming those on
// no evidence — straight into the airable pool, with no second gate — is exactly what the threshold
// exists to prevent, so an unscored segment is HELD and its remedy is an operator re-detect.
func TestAutoConfirmable_HoldsBackAnUnscoredSegment(t *testing.T) {
	legacy := goodSeg(0, 0, 30_000)
	legacy.BoundaryConfidence = 0
	legacy.StartEvidence, legacy.EndEvidence = "", ""

	got := filler.AutoConfirmable(proposalOf(legacy), policy(true, 85, 120*time.Second), floor)

	if len(got.Confirm) != 0 {
		t.Fatal("confirmed a pre-V54 segment with no boundary evidence at all")
	}
}

// ⚠ The floor is the SAME one the scan boundary rejects on. A segment this gate confirms and the
// scan then throws away is work done to produce nothing — out of a file that has been consumed.
//
// ⚠ **This check is now DEFENCE-IN-DEPTH, not the primary enforcement, and it is not dead.** Since
// V54 the floor is applied at DETECTION (`segmentFloor`), so a freshly-detected proposal cannot
// reach here holding a sub-floor segment. Two things still can: a proposal detected BEFORE V54 and
// re-gated afterwards, and a hand-edited cut list arriving through `Confirm`. Deleting this because
// "detection already handles it" would trust an input this gate does not control.
//
// Why the enforcement moved: `AutoConfirmable` returns on the first failing segment, so one
// sub-floor fragment sank the whole reel — and a real commercial compilation is made of them
// (measured: 39 of 82 segments under 10s on one archive.org reel). Auto-split had never once fired.
func TestAutoConfirmable_RefusesASegmentBelowTheClipFloor(t *testing.T) {
	p := proposalOf(goodSeg(0, 0, 30_000), goodSeg(1, 30_000, 34_000)) // 4s, under a 10s floor

	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor).Verdict(); got != filler.RejectTooShort {
		t.Errorf("confirmed a segment the scan would reject: %q", got)
	}
}

// Defense in depth: the automated stage prunes duplicates before the gate, but an uncurated
// proposal handed directly to the gate must still refuse rather than silently create it.
func TestAutoConfirmable_RefusesAFlaggedDuplicate(t *testing.T) {
	dup := goodSeg(1, 30_000, 60_000)
	dup.DupOf = "cc/dd/existing.mp4"
	p := proposalOf(goodSeg(0, 0, 30_000), dup)

	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor).Verdict(); got != filler.RejectDuplicate {
		t.Errorf("confirmed a reel containing a known duplicate: %q", got)
	}
}

// ⚠ THE GROUNDING RULE, reused rather than re-derived. A `SuggestedEra` is the classifier saying
// it guessed; that must disqualify AT EVERY SETTING, including the lowest one an operator can
// choose. Otherwise lowering the threshold would launder guesses into the catalog.
func TestAutoConfirmable_RefusesAGuessedEraAtAnyThreshold(t *testing.T) {
	guessed := goodSeg(0, 0, 30_000)
	guessed.Era = 0
	guessed.SuggestedEra = 1985
	p := proposalOf(guessed)

	for _, min := range []int{50, 85, 95} {
		if got := filler.AutoConfirmable(p, policy(true, min, 120*time.Second), floor).Verdict(); got != filler.RejectUngrounded {
			t.Errorf("min=%d confirmed a GUESSED era: %q", min, got)
		}
	}
}

// A segment pod assembly cannot match on is one that could only ever be a fallback pick — not
// something to create unattended out of a file the operator still had.
func TestAutoConfirmable_RefusesAnUnclassifiedSegment(t *testing.T) {
	bare := filler.SplitSegment{Index: 0, StartMs: 0, EndMs: 30_000, Name: "Unknown"}
	p := proposalOf(bare)

	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor).Verdict(); got != filler.RejectUntagged {
		t.Errorf("confirmed an unclassifiable segment: %q", got)
	}
}

// ⚠ The threshold has to do REAL work or it is decoration. At the ceiling a grounded era is
// required; below it, tags alone suffice — and the two must actually differ, or the setting is a
// number nobody's change affects.
func TestAutoConfirmable_ThresholdChangesTheAnswer(t *testing.T) {
	tagged := goodSeg(0, 0, 30_000)
	tagged.Era = 0 // classified, but no year was read
	p := proposalOf(tagged)

	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor).Verdict(); got != filler.AutoSplitOK {
		t.Errorf("min=85 rejected a tagged segment with no era: %q", got)
	}
	if got := filler.AutoConfirmable(p, policy(true, 95, 120*time.Second), floor).Verdict(); got != filler.RejectUngrounded {
		t.Errorf("min=95 accepted a segment with no era read: %q", got)
	}
}

func TestAutoConfirmable_RefusesAnEmptyProposal(t *testing.T) {
	if got := filler.AutoConfirmable(proposalOf(), policy(true, 85, 120*time.Second), floor).Verdict(); got != filler.RejectNoSegments {
		t.Errorf("confirmed a proposal with no segments: %q", got)
	}
}
