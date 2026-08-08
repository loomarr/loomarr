package filler

import (
	"strings"
	"testing"
)

// --- segmentsFromBoundaries -------------------------------------------------

func TestSegmentsFromBoundaries_CutsAtGapMidpoints(t *testing.T) {
	// Two black gaps at 30s and 60s (each ~0.5s wide) in a 90s compilation.
	gaps := []Interval{{StartMs: 29800, EndMs: 30200}, {StartMs: 59900, EndMs: 60100}}
	segs := segmentsFromBoundaries(90_000, gaps)
	if len(segs) != 3 {
		t.Fatalf("segments = %+v, want 3", segs)
	}
	// Midpoints: 30000 and 60000.
	want := []Interval{{0, 30000}, {30000, 60000}, {60000, 90000}}
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
	segs := segmentsFromBoundaries(90_000, gaps)
	if len(segs) != 2 {
		t.Fatalf("overlapping black+silence gave %d segments, want 2: %+v", len(segs), segs)
	}
}

func TestSegmentsFromBoundaries_DropsSlivers(t *testing.T) {
	// A gap 1s in — the "segment" before it is a fade-in artefact, not an advert.
	gaps := []Interval{{StartMs: 900, EndMs: 1100}, {StartMs: 45000, EndMs: 45500}}
	segs := segmentsFromBoundaries(90_000, gaps)
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
	segs := segmentsFromBoundaries(149_000, nil)
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
	segs := segmentsFromChapters(chapters)
	if len(segs) != 2 {
		t.Fatalf("chapters = %+v, want 2 segments (sliver dropped)", segs)
	}
	if segs[0].Name != "McDonald's" || segs[1].Name != "Lego" {
		t.Errorf("chapter titles not carried: %+v", segs)
	}
}

// --- parsers (pinned against captured tool output shape) ---------------------

const sampleFFmpegStderr = `Input #0, matroska,webm, from 'comp.mp4':
  Duration: 00:01:29.50, start: 0.000000, bitrate: 1200 kb/s
[blackdetect @ 0x7f9] black_start:29.8 black_end:30.2 black_duration:0.4
[blackdetect @ 0x7f9] black_start:59.9 black_end:60.1 black_duration:0.2
[silencedetect @ 0xabc] silence_start: 3.36
[silencedetect @ 0xabc] silence_end: 5.84 | silence_duration: 2.48
[silencedetect @ 0xabc] silence_start: 87.1
`

func TestParseBlackdetect(t *testing.T) {
	got := parseBlackdetect(sampleFFmpegStderr)
	if len(got) != 2 {
		t.Fatalf("black intervals = %+v, want 2", got)
	}
	if got[0] != (Interval{StartMs: 29800, EndMs: 30200}) {
		t.Errorf("first black interval = %+v", got[0])
	}
}

func TestParseSilencedetect_PairsStartsAndEnds(t *testing.T) {
	got := parseSilencedetect(sampleFFmpegStderr)
	if len(got) != 2 {
		t.Fatalf("silence intervals = %+v, want a pair + an unclosed tail", got)
	}
	if got[0] != (Interval{StartMs: 3360, EndMs: 5840}) {
		t.Errorf("paired silence = %+v", got[0])
	}
	// Trailing silence_start with no end (runs to EOF): kept, closed by the
	// duration clamp downstream — dropping it would miss the final boundary.
	if got[1].StartMs != 87100 || got[1].EndMs != got[1].StartMs {
		t.Errorf("unclosed tail = %+v", got[1])
	}
}

func TestParseFFprobeChapters_ScalesTimeBase(t *testing.T) {
	out := []byte(`{"chapters":[
		{"id":0,"time_base":"1/1000","start":0,"end":30000,"tags":{"title":"McDonald's"}},
		{"id":1,"time_base":"1/1000000","start":30000000,"end":61000000,"tags":{"title":"Lego"}}
	]}`)
	chs, err := parseFFprobeChapters(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(chs) != 2 {
		t.Fatalf("chapters = %+v", chs)
	}
	// The second chapter's time_base is 1/1000000 — assuming 1/1000 would put it
	// 1000× out. Scaling, not assuming, is the assertion.
	if chs[1].StartMs != 30000 || chs[1].EndMs != 61000 {
		t.Errorf("time_base not scaled: %+v", chs[1])
	}
	if chs[0].Title != "McDonald's" {
		t.Errorf("title lost: %+v", chs[0])
	}
}

func TestParseWhisperJSON(t *testing.T) {
	out := []byte(`{"transcription":[
		{"timestamps":{"from":"00:00:00,000","to":"00:00:02,000"},"offsets":{"from":0,"to":2000},"text":" Rice Krispies treats are so easy to make"},
		{"timestamps":{"from":"00:00:02,000","to":"00:00:04,500"},"offsets":{"from":2000,"to":4500},"text":" "},
		{"timestamps":{"from":"00:00:04,500","to":"00:00:06,000"},"offsets":{"from":4500,"to":6000},"text":" even the kids can do it"}
	]}`)
	segs, err := parseWhisperJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	// The blank utterance is dropped; offsets (ms), not the timestamp strings,
	// are the source of truth.
	if len(segs) != 2 || segs[0].EndMs != 2000 || segs[1].StartMs != 4500 {
		t.Errorf("transcript = %+v", segs)
	}
	text := TranscriptText(segs)
	if !strings.Contains(text, "[00:00] Rice Krispies") || !strings.Contains(text, "[00:04] even the kids") {
		t.Errorf("prompt rendering wrong:\n%s", text)
	}
}

// --- rescue validation -------------------------------------------------------

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
	spans, err := validateRescueSpans(out, 121_000)
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
	spans, err := validateRescueSpans(out, 149_000)
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
	if _, err := validateRescueSpans(out, 121_000); err == nil {
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
	if err := validateConfirmedSegments(ok, dur); err != nil {
		t.Errorf("valid cut list rejected: %v", err)
	}
	overlap := []SplitSegment{{StartMs: 0, EndMs: 31000}, {StartMs: 30000, EndMs: 60000}}
	if err := validateConfirmedSegments(overlap, dur); err == nil {
		t.Error("overlapping cut list accepted — two clips would share seconds")
	}
	outside := []SplitSegment{{StartMs: 0, EndMs: 91_000}}
	if err := validateConfirmedSegments(outside, dur); err == nil {
		t.Error("segment past the clip end accepted")
	}
	sliver := []SplitSegment{{StartMs: 0, EndMs: 1000}}
	if err := validateConfirmedSegments(sliver, dur); err == nil {
		t.Error("sub-minimum segment accepted")
	}
}
