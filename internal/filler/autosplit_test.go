package filler_test

import (
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
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

// goodSeg is an advert-shaped, classified, grounded segment — the only shape that qualifies.
func goodSeg(i int, startMs, endMs int64) filler.SplitSegment {
	return filler.SplitSegment{
		Index: i, StartMs: startMs, EndMs: endMs,
		Name: "Advert", Era: 1993, Audience: filler.Kids, Category: "toys",
	}
}

func proposalOf(segs ...filler.SplitSegment) filler.SplitProposal {
	return filler.SplitProposal{ID: "p1", ClipPath: "aa/bb/reel.mp4", Segments: segs}
}

const floor = 10 * time.Second

func TestAutoConfirmable_AcceptsAnUnambiguousReel(t *testing.T) {
	p := proposalOf(
		goodSeg(0, 0, 30_000),
		goodSeg(1, 30_000, 60_000),
		goodSeg(2, 60_000, 105_000),
	)
	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor); got != filler.AutoSplitOK {
		t.Errorf("a clean reel was rejected: %q", got)
	}
}

// ⚠ OFF by default is the safety property, so it is asserted rather than assumed.
func TestAutoConfirmable_RefusesWhenDisabled(t *testing.T) {
	p := proposalOf(goodSeg(0, 0, 30_000))
	if got := filler.AutoConfirmable(p, policy(false, 85, 120*time.Second), floor); got != filler.RejectDisabled {
		t.Errorf("confirmed with the feature off: %q", got)
	}
	// A nil policy is an install that never wired it — must also refuse, not panic.
	if got := filler.AutoConfirmable(p, nil, floor); got != filler.RejectDisabled {
		t.Errorf("nil policy = %q, want a refusal", got)
	}
}

// ⚠ THE case this gate exists for. `Unsplittable` is the detector admitting it could not find a
// boundary; auto-confirming around that admission is exactly the 69% failure §10 measured.
func TestAutoConfirmable_RefusesAnUnsplittableSegment(t *testing.T) {
	bad := goodSeg(1, 30_000, 90_000)
	bad.Unsplittable = true
	p := proposalOf(goodSeg(0, 0, 30_000), bad, goodSeg(2, 90_000, 120_000))

	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor); got != filler.RejectUnsplittable {
		t.Errorf("confirmed a reel containing an unsplittable span: %q", got)
	}
}

// ⚠ ALL-OR-NOTHING. Two perfectly good segments do not earn a confirm when the third is doubtful —
// the doubtful one is evidence about the REEL's detection quality, not just about itself.
func TestAutoConfirmable_OneBadSegmentSinksTheWholeReel(t *testing.T) {
	tooLong := goodSeg(2, 60_000, 400_000) // 5m40s — not an advert
	p := proposalOf(goodSeg(0, 0, 30_000), goodSeg(1, 30_000, 60_000), tooLong)

	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor); got != filler.RejectTooLong {
		t.Errorf("confirmed a reel with a 5-minute 'advert': %q", got)
	}
}

// ⚠ The floor is the SAME one the scan boundary rejects on. A segment this gate confirms and the
// scan then throws away is work done to produce nothing — out of a file that has been consumed.
func TestAutoConfirmable_RefusesASegmentBelowTheClipFloor(t *testing.T) {
	p := proposalOf(goodSeg(0, 0, 30_000), goodSeg(1, 30_000, 34_000)) // 4s, under a 10s floor

	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor); got != filler.RejectTooShort {
		t.Errorf("confirmed a segment the scan would reject: %q", got)
	}
}

// §10: a dedup match is a FLAG and never a silent drop — so it is never a silent keep either.
func TestAutoConfirmable_RefusesAFlaggedDuplicate(t *testing.T) {
	dup := goodSeg(1, 30_000, 60_000)
	dup.DupOf = "cc/dd/existing.mp4"
	p := proposalOf(goodSeg(0, 0, 30_000), dup)

	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor); got != filler.RejectDuplicate {
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
		if got := filler.AutoConfirmable(p, policy(true, min, 120*time.Second), floor); got != filler.RejectUngrounded {
			t.Errorf("min=%d confirmed a GUESSED era: %q", min, got)
		}
	}
}

// A segment pod assembly cannot match on is one that could only ever be a fallback pick — not
// something to create unattended out of a file the operator still had.
func TestAutoConfirmable_RefusesAnUnclassifiedSegment(t *testing.T) {
	bare := filler.SplitSegment{Index: 0, StartMs: 0, EndMs: 30_000, Name: "Unknown"}
	p := proposalOf(bare)

	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor); got != filler.RejectUntagged {
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

	if got := filler.AutoConfirmable(p, policy(true, 85, 120*time.Second), floor); got != filler.AutoSplitOK {
		t.Errorf("min=85 rejected a tagged segment with no era: %q", got)
	}
	if got := filler.AutoConfirmable(p, policy(true, 95, 120*time.Second), floor); got != filler.RejectUngrounded {
		t.Errorf("min=95 accepted a segment with no era read: %q", got)
	}
}

func TestAutoConfirmable_RefusesAnEmptyProposal(t *testing.T) {
	if got := filler.AutoConfirmable(proposalOf(), policy(true, 85, 120*time.Second), floor); got != filler.RejectNoSegments {
		t.Errorf("confirmed a proposal with no segments: %q", got)
	}
}
