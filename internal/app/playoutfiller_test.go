package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/schedule"
)

// stubCycle returns a fixed lineup: one program then a break gap.
type stubCycle struct{ slots []schedule.Slot }

func (s stubCycle) CyclePreview(context.Context, string, time.Time) (
	time.Time, []schedule.Slot, schedule.ActiveRuleAttribution, time.Duration, error,
) {
	return time.Time{}, s.slots, schedule.ActiveRuleAttribution{}, 0, nil
}

type stubPods struct {
	pod filler.Pod
	err error
}

func (s stubPods) Preview(context.Context, string) (filler.Pod, error) { return s.pod, s.err }
func (s stubPods) PreviewDraft(context.Context, string, filler.Selection) (filler.Pod, error) {
	return s.pod, s.err
}

func (s stubPods) PreviewAt(context.Context, string, int64) (filler.Pod, error) {
	return s.pod, s.err
}

func (s stubPods) Coverage(context.Context, string) (filler.CoverageReport, error) {
	return filler.CoverageReport{}, s.err
}

func (s stubPods) ClipFit(context.Context, string) (map[string]filler.Fit, error) {
	return nil, s.err
}

func (s stubPods) Pool(context.Context) (filler.PoolReport, error) {
	return filler.PoolReport{}, s.err
}

func fillerResolver(t *testing.T, dir string, pod filler.Pod) *playoutResolver {
	t.Helper()
	return &playoutResolver{
		// A break gap FIRST, so a clock at the epoch lands inside it.
		engine: stubCycle{slots: []schedule.Slot{
			{Kind: schedule.SlotFiller},
			{Kind: schedule.SlotProgram, LibraryItemID: "x", DurationMs: 60000},
		}},
		now:            func() time.Time { return playoutEpoch("ch1") },
		tier:           func() string { return "balanced" },
		encoder:        func() string { return "" },
		capacity:       func() int { return 4 },
		activeChannels: func() int { return 0 },
		pods:           stubPods{pod: pod},
		fillerDir:      func() string { return dir },
	}
}

// THE GAP THIS CLOSES: a break must resolve to a real commercial FILE. Tunarr used to do this
// via filler-lists; internal playout has no such negotiator, so without this every break was
// dead air (the offline card).
func TestAiringNow_BreakResolvesToACommercialFile(t *testing.T) {
	dir := t.TempDir()
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: "bumper.mp4", Name: "bumper", DurationMs: 4000},
		{Path: "1994/toys.mp4", Name: "toys", DurationMs: 8000},
	}}
	r := fillerResolver(t, dir, pod)

	airing, src, err := r.AiringNow(context.Background(), "ch1")
	if err != nil {
		t.Fatal(err)
	}
	if !airing.Playable() {
		t.Fatalf("a break with a full pod was not playable: %+v", airing)
	}
	if src == "" {
		t.Fatal("no ffmpeg input for the commercial")
	}
	// The FIRST pod entry, since the clock is at the start of the break.
	if airing.Title != "bumper" {
		t.Errorf("title = %q, want the first pod entry", airing.Title)
	}
	if airing.Remaining != 4000*time.Millisecond {
		t.Errorf("remaining = %v, want the clip's own duration", airing.Remaining)
	}
	// Source is what makes it playable — a clip has no LibraryItemID.
	if airing.Source == "" {
		t.Error("Source is empty, so Playable() would be false for every commercial")
	}
	if airing.LibraryItemID != "" {
		t.Errorf("a local clip must not carry a library id: %q", airing.LibraryItemID)
	}
}

// The pod is a SEQUENCE, so the resolver must pick whichever clip covers this instant —
// otherwise every viewer sees the bumper on loop and the ads never play.
func TestAiringNow_BreakWalksThePodByOffset(t *testing.T) {
	dir := t.TempDir()
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: "bumper.mp4", Name: "bumper", DurationMs: 4000},
		{Path: "1994/toys.mp4", Name: "toys", DurationMs: 8000},
		{Path: "1994/cereal.mp4", Name: "cereal", DurationMs: 8000},
	}}
	r := fillerResolver(t, dir, pod)

	// 6s into the break: past the 4s bumper, 2s into the first ad.
	base := playoutEpoch("ch1")
	r.now = func() time.Time { return base.Add(6 * time.Second) }

	airing, _, err := r.AiringNow(context.Background(), "ch1")
	if err != nil {
		t.Fatal(err)
	}
	if airing.Title != "toys" {
		t.Errorf("title = %q, want the ad the clock is inside of", airing.Title)
	}
	if airing.Offset != 2*time.Second {
		t.Errorf("offset = %v, want 2s INTO the ad — a mid-break joiner must not restart it",
			airing.Offset)
	}
}

// No pool ⇒ the offline card, not an error. A channel with no commercials must still tune.
func TestAiringNow_BreakWithNoPodIsNotAnError(t *testing.T) {
	r := fillerResolver(t, t.TempDir(), filler.Pod{})
	airing, src, err := r.AiringNow(context.Background(), "ch1")
	if err != nil {
		t.Fatalf("an empty pod was an error: %v", err)
	}
	if airing.Playable() || src != "" {
		t.Errorf("want the offline card, got %+v / %q", airing, src)
	}
}

// ⚠ A crafted clip id must NOT stream a file from outside FILLER_DIR. The id reaches here from
// the database, so containment is enforced at resolution, not assumed at write time.
func TestAiringNow_BreakRefusesAPathEscape(t *testing.T) {
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: "../../../../etc/passwd", Name: "evil", DurationMs: 4000},
	}}
	r := fillerResolver(t, t.TempDir(), pod)

	airing, src, err := r.AiringNow(context.Background(), "ch1")
	if err != nil {
		t.Fatal(err)
	}
	if airing.Playable() || src != "" {
		t.Errorf("a traversal id resolved to %q — playout would stream it", src)
	}
}

// The embedded fallback bumper card is generated, not a file: it has no path to play.
func TestAiringNow_BreakWithOnlyTheFallbackCardPlaysNoFile(t *testing.T) {
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: "", Name: "fallback card", DurationMs: 5000, IsFallbackCard: true},
	}}
	r := fillerResolver(t, t.TempDir(), pod)

	_, src, err := r.AiringNow(context.Background(), "ch1")
	if err != nil {
		t.Fatal(err)
	}
	if src != "" {
		t.Errorf("the fallback card resolved to a file: %q", src)
	}
}

// --- Encoder selection (the silent-software-fallback bug) ---

// An operator override wins outright and must NOT trigger a probe: probing trial-encodes every
// candidate at ~5s apiece, which would be paid for nothing.
func TestProfile_OperatorOverrideSkipsTheProbe(t *testing.T) {
	probed := false
	r := fillerResolver(t, t.TempDir(), filler.Pod{})
	r.encoder = func() string { return "h264_nvenc" }
	r.ffmpegPath = func() string { probed = true; return "/nonexistent/ffmpeg" }

	p := r.Profile(context.Background())
	if p.Encoder != "h264_nvenc" {
		t.Errorf("encoder = %q, want the operator's override", p.Encoder)
	}
	if probed {
		t.Error("probed despite an explicit override — that is ~20s of trial encodes for nothing")
	}
}

// With no override, the encoder is MEASURED rather than assumed. Previously this fell straight
// through to libx264 with a comment claiming the wizard had stored a choice — nothing did, so a
// box with a working GPU silently encoded in software forever.
//
// Detect never errors: "no hardware" is the expected outcome on most machines and software is a
// correct answer. So with a bogus ffmpeg path the probe finds nothing and yields libx264 — which
// is exactly the contract worth pinning.
func TestProfile_NoOverrideProbesAndFallsBackSafely(t *testing.T) {
	r := fillerResolver(t, t.TempDir(), filler.Pod{})
	r.encoder = func() string { return "" }
	r.ffmpegPath = func() string { return "/nonexistent/ffmpeg" }

	p := r.Profile(context.Background())
	if p.Encoder != playout.EncoderSoftware {
		t.Errorf("encoder = %q, want the software fallback when nothing probes clean", p.Encoder)
	}
}

// The probe runs ONCE. It is called per program boundary, and trial-encoding every candidate on
// each one would make every program transition take ~20 seconds.
func TestProfile_ProbesOnlyOnce(t *testing.T) {
	calls := 0
	r := fillerResolver(t, t.TempDir(), filler.Pod{})
	r.encoder = func() string { return "" }
	r.ffmpegPath = func() string { calls++; return "/nonexistent/ffmpeg" }

	for i := 0; i < 5; i++ {
		r.Profile(context.Background())
	}
	if calls != 1 {
		t.Errorf("probed %d times across 5 programs, want 1 — each probe is ~20s", calls)
	}
}

// recordingPlays captures RecordClipPlay calls (V28).
type recordingPlays struct {
	calls []string
	err   error
}

func (r *recordingPlays) RecordClipPlay(_ context.Context, clipPath string, _ time.Time) error {
	r.calls = append(r.calls, clipPath)
	return r.err
}

// V28: a clip that AIRS is counted, and the count identifies the clip by its catalog path
// (not the absolute file, which is a deployment detail).
func TestAiringNow_CountsTheClipThatAirs(t *testing.T) {
	dir := t.TempDir()
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: "1994/toys.mp4", Name: "toys", DurationMs: 8000},
	}}
	r := fillerResolver(t, dir, pod)
	plays := &recordingPlays{}
	r.clipPlays = plays

	if _, _, err := r.AiringNow(context.Background(), "ch1"); err != nil {
		t.Fatal(err)
	}
	if len(plays.calls) != 1 || plays.calls[0] != "1994/toys.mp4" {
		t.Fatalf("recorded %v, want one play of the CATALOG path", plays.calls)
	}
}

// ⚠ The overcounting guard. A mid-clip re-resolve — which happens whenever anything asks what
// is on while a clip is already rolling — must NOT count again. Only the item's start does.
//
// Without the `into == 0` check this counts on every resolve, and a 30s advert re-resolved by
// a reconnecting viewer would report several plays for one airing.
func TestAiringNow_DoesNotCountAMidClipResolve(t *testing.T) {
	dir := t.TempDir()
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: "1994/toys.mp4", Name: "toys", DurationMs: 8000},
	}}
	r := fillerResolver(t, dir, pod)
	plays := &recordingPlays{}
	r.clipPlays = plays

	// Three seconds into the break: the clip is already rolling.
	base := playoutEpoch("ch1")
	r.now = func() time.Time { return base.Add(3 * time.Second) }

	if _, _, err := r.AiringNow(context.Background(), "ch1"); err != nil {
		t.Fatal(err)
	}
	if len(plays.calls) != 0 {
		t.Errorf("a mid-clip resolve counted %v — only the clip's START is an airing", plays.calls)
	}
}

// Counting is telemetry: a store failure must never stop a break from airing.
func TestAiringNow_ACountingFailureStillAirsTheBreak(t *testing.T) {
	dir := t.TempDir()
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: "1994/toys.mp4", Name: "toys", DurationMs: 8000},
	}}
	r := fillerResolver(t, dir, pod)
	r.clipPlays = &recordingPlays{err: errors.New("database is gone")}

	airing, src, err := r.AiringNow(context.Background(), "ch1")
	if err != nil {
		t.Fatalf("a counting failure became a playout error: %v", err)
	}
	if !airing.Playable() || src == "" {
		t.Errorf("the break must still air: %+v src=%q", airing, src)
	}
}

// No recorder wired (Tunarr-backed, or telemetry off) is a supported configuration.
func TestAiringNow_NoRecorderIsFine(t *testing.T) {
	dir := t.TempDir()
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: "1994/toys.mp4", Name: "toys", DurationMs: 8000},
	}}
	r := fillerResolver(t, dir, pod) // clipPlays left nil
	if _, _, err := r.AiringNow(context.Background(), "ch1"); err != nil {
		t.Fatalf("a nil recorder must not break playout: %v", err)
	}
}
