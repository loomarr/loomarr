package filler

import "testing"

// The boundary-confidence ladder (§10 V34). ⚠ These pin the SHAPE of the rule — best evidence per
// edge, worst edge decides, segment facts only lower — not the specific numbers, which the
// constants carry and the doc justifies.

func seg(startSrc, endSrc boundarySource) SplitSegment {
	return SplitSegment{StartMs: 0, EndMs: 30_000, startSrc: startSrc, endSrc: endSrc}
}

func TestBoundaryScore_CorroborationBeatsASingleDetector(t *testing.T) {
	both := boundaryScore(seg(srcBlack|srcSilence, srcBlack|srcSilence))
	one := boundaryScore(seg(srcBlack, srcBlack))

	if both <= one {
		t.Fatalf("agreed=%d single=%d — corroboration must score higher; it is the 9-of-12 measurement", both, one)
	}
}

// ⚠ Nothing measures black being better than silence, so nothing may claim it.
func TestBoundaryScore_BlackAndSilenceRankEqually(t *testing.T) {
	if b, s := boundaryScore(seg(srcBlack, srcBlack)), boundaryScore(seg(srcSilence, srcSilence)); b != s {
		t.Errorf("black=%d silence=%d — ranking one above the other invents a number nothing measured", b, s)
	}
}

// ⚠ THE composition rule: a cut is only as trustworthy as its weaker end.
func TestBoundaryScore_WorstEdgeDecides(t *testing.T) {
	mixed := boundaryScore(seg(srcChapter, srcSilence)) // 100 and 65
	if want := boundaryScore(seg(srcSilence, srcSilence)); mixed != want {
		t.Errorf("chapter+silence = %d, want %d — the strong edge must not carry the weak one", mixed, want)
	}
}

// ⚠ The demoted duration prior. A sub-floor-ish bumper cut on corroborated boundaries is a GOOD
// cut: 39 of 82 segments on the measured reel were short and every one was right. Scoring by
// "is it a standard slot length" would flag half a clean reel.
func TestBoundaryScore_DurationIsNotAPrior(t *testing.T) {
	short := SplitSegment{StartMs: 0, EndMs: 6_000, startSrc: srcBlack | srcSilence, endSrc: srcBlack | srcSilence}
	classic := SplitSegment{StartMs: 0, EndMs: 30_000, startSrc: srcBlack | srcSilence, endSrc: srcBlack | srcSilence}

	if boundaryScore(short) != boundaryScore(classic) {
		t.Errorf("6s=%d 30s=%d — duration must not move a boundary score",
			boundaryScore(short), boundaryScore(classic))
	}
}

// Segment facts may only LOWER, and over-long is one of them — unless the rescue said otherwise.
func TestBoundaryScore_OverlongCapsUnlessTheRescueConfirmedIt(t *testing.T) {
	long := SplitSegment{StartMs: 0, EndMs: 200_000, startSrc: srcChapter, endSrc: srcChapter}
	if got := boundaryScore(long); got != confOverlong {
		t.Errorf("over-long on chapter edges = %d, want the over-long cap %d", got, confOverlong)
	}

	// ⚠ The measured 121s infomercial: the rescue looked and said "this is ONE advert", which is
	// precisely the fact that defeats "over-long means a missed boundary".
	long.rescueConfirmedWhole = true
	if got := boundaryScore(long); got != confDeclared {
		t.Errorf("rescue-confirmed over-long = %d, want the cap lifted to %d", got, confDeclared)
	}
}

// ⚠ An operator's typed timecode is not evidence Loomarr gathered. Scoring it would let the machine
// auto-confirm on the strength of a human's own half-finished edit.
func TestBoundaryScore_AnOperatorEdgeScoresNothing(t *testing.T) {
	if got := boundaryScore(seg(srcOperator, srcChapter)); got != 0 {
		t.Errorf("operator-touched edge = %d, want 0", got)
	}
}

// ⚠ A collision is AGREEMENT. Two gaps whose midpoints clamp onto the same millisecond used to have
// the second silently dropped — right for the position, wrong for the evidence, because it threw
// away a second detector confirming that very spot.
func TestBoundaryCuts_CollidingMidpointsUnionTheirEvidence(t *testing.T) {
	// ⚠ These two must NOT overlap, or they merge in the first loop and never reach the collision
	// branch at all — which is what a first draft of this test did, and the sabotage run caught it
	// passing with the branch deleted. They are separated (90_002 < 90_004) but both midpoints
	// clamp to durationMs-1, so they arrive at the dedupe as two cuts at the same millisecond.
	cuts := boundaryCuts([]detectedGap{
		{Interval: Interval{StartMs: 89_998, EndMs: 90_002}, Src: srcBlack},   // mid 90_000 → 89_999
		{Interval: Interval{StartMs: 90_004, EndMs: 90_010}, Src: srcSilence}, // mid 90_007 → 89_999
	}, 90_000)

	if len(cuts) != 1 {
		t.Fatalf("cuts = %+v, want the colliding midpoints deduped to one", cuts)
	}
	if cuts[0].Src != srcBlack|srcSilence {
		t.Errorf("evidence = %b, want both detectors — a dropped collision loses a corroboration", cuts[0].Src)
	}
}
