package filler

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// blackGaps tags plain intervals as black-detected — the shape most of these tests want, since
// they are about CUT POSITIONS rather than about which detector found them. The provenance-aware
// cases below build `detectedGap` values directly.
func blackGaps(in ...Interval) []detectedGap {
	out := make([]detectedGap, 0, len(in))
	for _, g := range in {
		out = append(out, detectedGap{Interval: g, Src: srcBlack})
	}
	return out
}

// --- segmentsFromBoundaries -------------------------------------------------

func TestSegmentsFromBoundaries_CutsAtGapMidpoints(t *testing.T) {
	// Two black gaps at 30s and 60s (each ~0.5s wide) in a 90s compilation.
	gaps := []Interval{{StartMs: 29800, EndMs: 30200}, {StartMs: 59900, EndMs: 60100}}
	segs, _ := segmentsFromBoundaries(90_000, blackGaps(gaps...), newSegmentFloor(0))
	if len(segs) != 3 {
		t.Fatalf("segments = %+v, want 3", segs)
	}
	// Midpoints: 30000 and 60000.
	want := []Interval{{StartMs: 0, EndMs: 30000}, {StartMs: 30000, EndMs: 60000}, {StartMs: 60000, EndMs: 90000}}
	for i, w := range want {
		if segs[i].StartMs != w.StartMs || segs[i].EndMs != w.EndMs {
			t.Errorf("segment %d = [%d,%d), want [%d,%d)", i, segs[i].StartMs, segs[i].EndMs, w.StartMs, w.EndMs)
		}
	}
}

func TestSegmentsFromBoundaries_MergesBlackAndSilenceOnOneBoundary(t *testing.T) {
	// A fade that is BOTH black and silent must produce ONE cut, not two slivers —
	// blackdetect and silencedetect routinely bracket the same boundary.
	gaps := []Interval{
		{StartMs: 29800, EndMs: 30200}, // black
		{StartMs: 29900, EndMs: 31000}, // silence overlapping the black
	}
	segs, _ := segmentsFromBoundaries(90_000, blackGaps(gaps...), newSegmentFloor(0))
	if len(segs) != 2 {
		t.Fatalf("overlapping black+silence gave %d segments, want 2: %+v", len(segs), segs)
	}
}

func TestSegmentsFromBoundaries_DropsSlivers(t *testing.T) {
	// A gap 1s in — the "segment" before it is a fade-in artefact, not an advert.
	gaps := []Interval{{StartMs: 900, EndMs: 1100}, {StartMs: 45000, EndMs: 45500}}
	segs, _ := segmentsFromBoundaries(90_000, blackGaps(gaps...), newSegmentFloor(0))
	if len(segs) != 2 {
		t.Fatalf("segments = %+v, want the 1s sliver dropped", segs)
	}
	if segs[0].StartMs != 1000 {
		t.Errorf("first kept segment starts %d, want 1000 (the sliver's far side)", segs[0].StartMs)
	}
}

func TestSegmentsFromBoundaries_NoGapsIsOneWholeSegment(t *testing.T) {
	// Detection finding nothing is NOT "no segments": the whole file is one
	// (over-long) segment, which is exactly what sends a clean compilation to
	// the transcript rescue.
	segs, _ := segmentsFromBoundaries(149_000, nil, newSegmentFloor(0))
	if len(segs) != 1 || segs[0].StartMs != 0 || segs[0].EndMs != 149_000 {
		t.Fatalf("no-gap file = %+v, want one whole-file segment", segs)
	}
	if !segs[0].overlong() {
		t.Error("a 149s whole-file segment must be over-long (rescue-bound)")
	}
}

func TestSegmentsFromChapters_FreeSplitWithTitles(t *testing.T) {
	chapters := []Chapter{
		{StartMs: 0, EndMs: 30000, Title: "McDonald's"},
		{StartMs: 30000, EndMs: 30500, Title: "sliver"}, // dropped
		{StartMs: 30500, EndMs: 61000, Title: "Lego"},
	}
	segs, _ := segmentsFromChapters(chapters, newSegmentFloor(0))
	if len(segs) != 2 {
		t.Fatalf("chapters = %+v, want 2 segments (sliver dropped)", segs)
	}
	if segs[0].Name != "McDonald's" || segs[1].Name != "Lego" {
		t.Errorf("chapter titles not carried: %+v", segs)
	}
}

// --- the CATALOG floor at detection (§10 V34, V54) --------------------------
//
// ⚠ These are the cases that had never been tested, and their absence is why auto-split shipped
// default-ON and structurally unable to fire. Every existing test above runs at the 3s sliver
// floor; production runs at the 10s catalog floor, and the 3–10s band is where real compilations
// live. Measured 2026-08-11 on an 82-segment archive.org reel: 39 segments in that band.

func TestSegmentsFromBoundaries_DropsUnderTheCatalogFloor(t *testing.T) {
	// A 5s span between two boundaries — comfortably over MinSegmentMs, under a 10s catalog floor.
	gaps := []Interval{{StartMs: 19_900, EndMs: 20_100}, {StartMs: 24_900, EndMs: 25_100}}

	segs, dropped := segmentsFromBoundaries(90_000, blackGaps(gaps...), newSegmentFloor(10*time.Second))
	if len(segs) != 2 {
		t.Fatalf("segments = %+v, want the 5s span dropped at a 10s floor", segs)
	}
	if dropped.Count != 1 || dropped.Ms != 5_000 {
		t.Errorf("tally = %+v, want 1 fragment / 5000ms — an unreported drop is time the operator "+
			"can only find by arithmetic", dropped)
	}

	// ⚠ The floor is the ONLY difference: the identical input keeps all three at the sliver floor.
	// Without this half the test would also pass if the span were dropped for some other reason.
	if kept, _ := segmentsFromBoundaries(90_000, blackGaps(gaps...), newSegmentFloor(0)); len(kept) != 3 {
		t.Errorf("at a 3s floor the 5s span must survive; got %d segments", len(kept))
	}
}

func TestSegmentsFromChapters_DropsUnderTheCatalogFloor(t *testing.T) {
	chapters := []Chapter{
		{StartMs: 0, EndMs: 30_000, Title: "McDonald's"},
		{StartMs: 30_000, EndMs: 35_000, Title: "station ID"}, // 5s — the band that sank reels
		{StartMs: 35_000, EndMs: 66_000, Title: "Lego"},
	}
	segs, dropped := segmentsFromChapters(chapters, newSegmentFloor(10*time.Second))
	if len(segs) != 2 || segs[0].Name != "McDonald's" || segs[1].Name != "Lego" {
		t.Fatalf("chapters = %+v, want the 5s station ID dropped and titles kept", segs)
	}
	if dropped.Count != 1 || dropped.Ms != 5_000 {
		t.Errorf("tally = %+v, want 1 fragment / 5000ms", dropped)
	}
}

// ⚠ The SECOND floor comparison, after overlap truncation — a distinct code path from the first.
// A span can be admitted at its proposed length and then shortened under the floor by truncation,
// and only this test would notice if that check were dropped.
func TestValidateRescueSpans_TruncationCanPushASpanUnderTheFloor(t *testing.T) {
	build := func() rescueOutput {
		out := rescueOutput{}
		add := func(start, end, product string) {
			out.Adverts = append(out.Adverts, struct {
				Start   string `json:"start"`
				End     string `json:"end"`
				Product string `json:"product"`
			}{Start: start, End: end, Product: product})
		}
		add("00:00", "00:30", "Swiffer")
		// 12s as proposed — over a 10s floor — but it overlaps Swiffer by 8s, so truncation
		// leaves 4s. It must pass the first check and fail the second.
		add("00:22", "00:34", "Aqua Globes")
		return out
	}

	spans, err := validateRescueSpans(build(), 60_000, newSegmentFloor(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 || spans[0].Product != "Swiffer" {
		t.Fatalf("spans = %+v, want only Swiffer — the truncated 4s remainder is under the floor", spans)
	}

	// At a 3s floor the truncated 4s remainder survives, which is what proves the drop above was
	// the floor's doing and not the truncation's.
	loose, err := validateRescueSpans(build(), 60_000, newSegmentFloor(0))
	if err != nil {
		t.Fatal(err)
	}
	if len(loose) != 2 || loose[1].StartMs != 30_000 || loose[1].EndMs != 34_000 {
		t.Fatalf("at a 3s floor = %+v, want the truncated [30000,34000) kept", loose)
	}
}

func TestValidateConfirmedSegments_RefusesAnOperatorCutUnderTheCatalogFloor(t *testing.T) {
	// ⚠ An 8s hand-drawn cut used to be written, spawned, and THEN rejected `too_short` by the
	// probe rung — a silent downstream loss the operator never connected to their edit.
	dur := int64(90_000)
	segs := []SplitSegment{{StartMs: 0, EndMs: 30_000}, {StartMs: 30_000, EndMs: 38_000}}

	err := validateConfirmedSegments(segs, dur, newSegmentFloor(10*time.Second))
	if err == nil {
		t.Fatal("an 8s hand-drawn cut was accepted; it would be cut, spawned and then thrown away")
	}
	if !errors.Is(err, ErrSplitValidation) {
		t.Errorf("err = %v, want ErrSplitValidation so the API answers 422", err)
	}
	if !strings.Contains(err.Error(), "filler.min_duration") {
		t.Errorf("err = %q, must name the setting that refused it — otherwise the 422 is unarguable", err)
	}
}

// --- parsers (pinned against captured tool output shape) ---------------------
func TestValidateRescueSpans_SingleAdvertStaysWhole(t *testing.T) {
	// ⚠ THE measured failure (plan §6.4): a 121s infomercial for ONE product was
	// split at round 30/61/92s marks. The prompt's single-advert rule is the
	// first defence; when it works, the model returns one span and the segment
	// must survive WHOLE — no manufactured clips.
	out := rescueOutput{}
	out.Adverts = append(out.Adverts, struct {
		Start   string `json:"start"`
		End     string `json:"end"`
		Product string `json:"product"`
	}{Start: "00:00", End: "02:01", Product: "Amazing Knife"})
	spans, err := validateRescueSpans(out, 121_000, newSegmentFloor(0))
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 || spans[0].StartMs != 0 || spans[0].EndMs != 121_000 {
		t.Fatalf("single advert = %+v, want one whole-segment span", spans)
	}
}

func TestValidateRescueSpans_MultiAdvertWithClampsAndOverlaps(t *testing.T) {
	out := rescueOutput{}
	add := func(start, end, product string) {
		out.Adverts = append(out.Adverts, struct {
			Start   string `json:"start"`
			End     string `json:"end"`
			Product string `json:"product"`
		}{Start: start, End: end, Product: product})
	}
	add("00:00", "00:27", "Swiffer")
	add("00:26", "00:54", "Aqua Globes")    // overlaps Swiffer by 1s — truncated to 27s
	add("02:30", "03:00", "Beyond the end") // clamps into the segment, then < min → dropped
	add("junk", "00:10", "Unparseable")     // dropped
	spans, err := validateRescueSpans(out, 149_000, newSegmentFloor(0))
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 {
		t.Fatalf("spans = %+v, want 2 (overlap truncated, out-of-range and junk dropped)", spans)
	}
	if spans[1].StartMs != 27_000 {
		t.Errorf("overlap not truncated to the previous end: %+v", spans[1])
	}
	if spans[1].Product != "Aqua Globes" {
		t.Errorf("product label lost: %+v", spans[1])
	}
}

func TestValidateRescueSpans_NothingValidIsAnError(t *testing.T) {
	// Confident nonsense (the poor-transcript case) must surface as UNSPLITTABLE,
	// never as a guessed cut.
	out := rescueOutput{}
	out.Adverts = append(out.Adverts, struct {
		Start   string `json:"start"`
		End     string `json:"end"`
		Product string `json:"product"`
	}{Start: "99:99", End: "99:99", Product: "800-555-0199"})
	if _, err := validateRescueSpans(out, 121_000, newSegmentFloor(0)); err == nil {
		t.Fatal("invalid spans accepted — a guessed cut would ship")
	}
}

func TestParseMMS(t *testing.T) {
	for s, want := range map[string]int64{"01:23": 83_000, "1:02:03": 3_723_000, "45": 45_000, "00:27.4": 27_400} {
		got, err := parseMMS(s)
		if err != nil || got != want {
			t.Errorf("parseMMS(%q) = %d, %v; want %d", s, got, err, want)
		}
	}
	if _, err := parseMMS("abc"); err == nil {
		t.Error("parseMMS(abc) accepted")
	}
}

// --- dHash -------------------------------------------------------------------

func grayFrame(v byte) []byte { // a flat 9x8 frame of one intensity
	f := make([]byte, 72)
	for i := range f {
		f[i] = v
	}
	return f
}

func TestDHash_IdenticalSpansAreZero(t *testing.T) {
	frames := [][]byte{grayFrame(10), grayFrame(200), grayFrame(90)}
	a, b := dHashFrames(frames), dHashFrames(frames)
	mean, ok := meanHamming(a, b)
	if !ok || mean != 0 {
		t.Errorf("identical span mean = %v, ok=%v; want 0", mean, ok)
	}
}

func TestDHash_DifferentAdvertsSeparate(t *testing.T) {
	// Two unrelated frame sequences: flat (all bits clear) vs a DECREASING
	// gradient (every left pixel brighter than its right neighbour → all bits
	// set — the maximum Hamming distance from flat).
	var grad []byte
	for row := 0; row < 8; row++ {
		for col := 0; col < 9; col++ {
			grad = append(grad, byte(224-col*28))
		}
	}
	a := dHashFrames([][]byte{grayFrame(10), grayFrame(200)})
	b := dHashFrames([][]byte{grad, grad, grad}) // longer — min-length compare
	mean, ok := meanHamming(a, b)
	if !ok {
		t.Fatal("no result for differing lengths — must compare over the shorter")
	}
	if mean <= DupHashThreshold {
		t.Errorf("different adverts flagged as duplicates (mean %v ≤ %v)", mean, DupHashThreshold)
	}
}

func TestDHash_NoFramesIsNotADuplicate(t *testing.T) {
	// An undecodable span must NEVER read as a duplicate — that is a silent drop,
	// the exact failure the flag exists to avoid.
	if _, ok := meanHamming(nil, dHashFrames([][]byte{grayFrame(1)})); ok {
		t.Error("empty frame sequence compared equal")
	}
}

// --- names / transcripts -----------------------------------------------------

// ⚠ `TestSanitizeClipName` was deleted with the function it covered (§10 V51a). A segment's
// filename is its content hash now, so there is no display name to make filesystem-safe — the
// name lives on the catalog row and is read by people, not by the filesystem.

func TestSliceTranscript(t *testing.T) {
	tr := []TranscriptSegment{
		{StartMs: 0, EndMs: 2000, Text: "first"},
		{StartMs: 2000, EndMs: 4000, Text: "second"},
		{StartMs: 4000, EndMs: 6000, Text: "third"},
	}
	got := sliceTranscript(tr, 2500, 4500)
	if len(got) != 2 || got[0].Text != "second" || got[1].Text != "third" {
		t.Errorf("slice = %+v, want the two overlapping utterances", got)
	}
}

func TestValidateConfirmedSegments(t *testing.T) {
	dur := int64(90_000)
	ok := []SplitSegment{{StartMs: 0, EndMs: 30000}, {StartMs: 30000, EndMs: 60000}}
	if err := validateConfirmedSegments(ok, dur, newSegmentFloor(0)); err != nil {
		t.Errorf("valid cut list rejected: %v", err)
	}
	overlap := []SplitSegment{{StartMs: 0, EndMs: 31000}, {StartMs: 30000, EndMs: 60000}}
	if err := validateConfirmedSegments(overlap, dur, newSegmentFloor(0)); err == nil {
		t.Error("overlapping cut list accepted — two clips would share seconds")
	}
	outside := []SplitSegment{{StartMs: 0, EndMs: 91_000}}
	if err := validateConfirmedSegments(outside, dur, newSegmentFloor(0)); err == nil {
		t.Error("segment past the clip end accepted")
	}
	sliver := []SplitSegment{{StartMs: 0, EndMs: 1000}}
	if err := validateConfirmedSegments(sliver, dur, newSegmentFloor(0)); err == nil {
		t.Error("sub-minimum segment accepted")
	}
}
