package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func (s stubPods) CoverageDraft(ctx context.Context, _ string, _ filler.Selection) (filler.CoverageReport, error) {
	if err := ctx.Err(); err != nil {
		return filler.CoverageReport{}, err
	}
	return filler.CoverageReport{}, s.err
}

func (s stubPods) ClipFit(context.Context, string) (map[string]filler.Fit, error) {
	return nil, s.err
}

func (s stubPods) Pool(context.Context) (filler.PoolReport, error) {
	return filler.PoolReport{}, s.err
}

// clipAt builds the SHARD-relative location a clip row carries since V38c — `a3/f9/<hash>.mp4`.
//
// ⚠ These fixtures used bare names like `1994/toys.mp4`, which `ClipPath`'s allow-list now
// refuses. That refusal is not pedantry: playout hands a pod entry's path straight to ClipPath,
// so an unresolvable one makes the break fall back to flex and the channel plays SILENCE. These
// tests caught exactly that when identity changed, which is what they are for.
func clipAt(name string) string {
	sum := sha256.Sum256([]byte(name))
	return filler.ClipRelPath(hex.EncodeToString(sum[:]), ".mp4")
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
		{Path: clipAt("bumper.mp4"), Name: "bumper", DurationMs: 4000},
		{Path: clipAt("1994/toys.mp4"), Name: "toys", DurationMs: 8000},
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
		{Path: clipAt("bumper.mp4"), Name: "bumper", DurationMs: 4000},
		{Path: clipAt("1994/toys.mp4"), Name: "toys", DurationMs: 8000},
		{Path: clipAt("1994/cereal.mp4"), Name: "cereal", DurationMs: 8000},
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
		// ⚠ The RAW traversal string, deliberately NOT run through clipAt — hashing it would
		// produce a perfectly valid id and test nothing. What must be refused is the crafted
		// value itself, as it would arrive from a tampered database row.
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
		// ⚠ An EMPTY path, not a hashed one: the fallback card is generated rather than played
		// from a file, and "" is how the resolver knows. Hashing it would give the card a path
		// that resolves to a file which does not exist.
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

func (r *recordingPlays) RecordClipPlay(_ context.Context, clipHash string, _ time.Time) error {
	r.calls = append(r.calls, clipHash)
	return r.err
}

// V28: a clip that AIRS is counted. V41: the count identifies the clip by its HASH.
//
// ⚠ This test asserted the catalog PATH from V28 until V41, and it was right when written —
// identity was the path then. V38c moved identity to the hash and never revisited this
// assertion, so the test went on pinning the pre-V38c contract and kept `RecordClipPlay`'s
// path argument looking correct while every counter in production stayed at zero
// (`RecordClipPlay` is keyed `WHERE hash = ?`). A test that outlives the model it encodes
// stops being a guard and becomes a reason not to look.
func TestAiringNow_CountsTheClipThatAirs(t *testing.T) {
	dir := t.TempDir()
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: clipAt("1994/toys.mp4"), Hash: "hash-toys", Name: "toys", DurationMs: 8000},
	}}
	r := fillerResolver(t, dir, pod)
	plays := &recordingPlays{}
	r.clipPlays = plays

	if _, _, err := r.AiringNow(context.Background(), "ch1"); err != nil {
		t.Fatal(err)
	}
	// ⚠ The HASH, not the path and not the absolute file. The counter keys on identity, so
	// anything else counts against a clip that does not exist — silently, because the caller
	// swallows the error as telemetry.
	if len(plays.calls) != 1 || plays.calls[0] != "hash-toys" {
		t.Fatalf("recorded %v, want one play of the clip HASH", plays.calls)
	}
}

// ⚠ The bumper card has no catalog row, so it has no hash — and must not be counted. Without
// the `e.Hash != ""` guard this records a play against the empty string, which is not merely
// useless: it is an UPDATE with an attacker-independent but wrong key, and the swallowed error
// means it would never be noticed.
func TestAiringNow_DoesNotCountAnEntryWithNoHash(t *testing.T) {
	dir := t.TempDir()
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: clipAt("1994/toys.mp4"), Name: "no-hash", DurationMs: 8000},
	}}
	r := fillerResolver(t, dir, pod)
	plays := &recordingPlays{}
	r.clipPlays = plays

	if _, _, err := r.AiringNow(context.Background(), "ch1"); err != nil {
		t.Fatal(err)
	}
	if len(plays.calls) != 0 {
		t.Errorf("recorded %v for an entry with no hash, want no play counted", plays.calls)
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
		{Path: clipAt("1994/toys.mp4"), Name: "toys", DurationMs: 8000},
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
		{Path: clipAt("1994/toys.mp4"), Name: "toys", DurationMs: 8000},
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
		{Path: clipAt("1994/toys.mp4"), Name: "toys", DurationMs: 8000},
	}}
	r := fillerResolver(t, dir, pod) // clipPlays left nil
	if _, _, err := r.AiringNow(context.Background(), "ch1"); err != nil {
		t.Fatalf("a nil recorder must not break playout: %v", err)
	}
}
