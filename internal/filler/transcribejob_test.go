package filler_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
)

// The on-demand transcribe job (§10 V44). Its whole risk is COST: Whisper is ~341s a clip under
// QEMU, so most of these assert that it does NOT run — a rich, already-tagged clip is skipped, an
// already-transcribed clip is skipped, and a wordless clip is recorded once so it is never re-run.

// fakeTranscribeStore records what the job wrote without a database.
type fakeTranscribeStore struct {
	clips       []filler.StoreClip
	transcripts map[string]string // path → recorded transcript ("" recorded is meaningful)
	setPaths    []string          // order + count of SetClipTranscript calls, incl. empties
	setErr      error
}

func newFakeTranscribeStore(clips ...filler.StoreClip) *fakeTranscribeStore {
	return &fakeTranscribeStore{clips: clips, transcripts: map[string]string{}}
}

func (f *fakeTranscribeStore) ListClips(context.Context, filler.ClipQuery) ([]filler.StoreClip, error) {
	return f.clips, nil
}

func (f *fakeTranscribeStore) SetClipTranscript(_ context.Context, path, transcript string, _ time.Time) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.setPaths = append(f.setPaths, path)
	f.transcripts[path] = transcript
	return nil
}

// scriptedTools is a MediaTools whose only real method is Transcribe, returning a fixed set of
// utterances (or an error). The rest satisfy the interface and are never called by this job.
type scriptedTools struct {
	segs  []filler.TranscriptSegment
	err   error
	calls int
}

func (s *scriptedTools) Transcribe(context.Context, string, int64, int64) ([]filler.TranscriptSegment, error) {
	s.calls++
	return s.segs, s.err
}

func (s *scriptedTools) Chapters(context.Context, string) ([]filler.Chapter, error) { return nil, nil }
func (s *scriptedTools) BlackSilence(context.Context, string) ([]filler.Interval, []filler.Interval, error) {
	return nil, nil, nil
}
func (s *scriptedTools) GrayFrames(context.Context, string, int64, int64) ([][]byte, error) {
	return nil, nil
}
func (s *scriptedTools) Keyframes(context.Context, string, int) ([][]byte, error) { return nil, nil }
func (s *scriptedTools) Cut(context.Context, string, int64, int64, string) error  { return nil }

// utter is a one-line whisper utterance.
func utter(text string) filler.TranscriptSegment {
	return filler.TranscriptSegment{StartMs: 0, EndMs: 1000, Text: text}
}

// transcribeClip builds a catalog clip. `transcript` seeds the not-yet vs recorded state; era +
// audience + category decide Tagged(); the path is where its sidecar is looked up.
func transcribeClip(path, transcript string) filler.StoreClip {
	return filler.StoreClip{Clip: filler.Clip{
		Hash: path, Path: path, Name: path, Kind: filler.Commercial,
		DurationMs: 30_000, Transcript: transcript,
	}}
}

// tagged marks a clip fully tagged (era + audience + category), so it is NOT a candidate on the
// "still untagged" arm — only its source text can make it one.
func tagged(c filler.StoreClip) filler.StoreClip {
	c.Era = 1994
	c.Audience = filler.Kids
	c.Category = "toys"
	return c
}

// newTranscribeJob wires the job with a scripted store/tools and a sidecar FS, always ENABLED and
// pointed at "/filler".
func newTranscribeJob(
	t *testing.T, st filler.TranscribeStore, tools filler.MediaTools, drop fstest.MapFS,
) *filler.TranscribeJob {
	t.Helper()
	return filler.NewTranscribeJob(st, tools,
		func() string { return "/filler" }, drop, func() bool { return true },
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() },
		slog.New(slog.DiscardHandler))
}

// sidecar builds a MapFS with one `<path minus ext>.info.json` carrying `description`.
func sidecar(clipPath, description string) fstest.MapFS {
	key := clipPath[:len(clipPath)-len(".mp4")] + ".info.json"
	return fstest.MapFS{key: {Data: []byte(`{"description":"` + description + `"}`)}}
}

// THE behaviour: a clip with a thin (empty) source description is transcribed and the text stored.
func TestTranscribeJob_TranscribesAThinSourceClip(t *testing.T) {
	st := newFakeTranscribeStore(transcribeClip("a.mp4", ""))
	tools := &scriptedTools{segs: []filler.TranscriptSegment{utter("buy our cereal today")}}
	// No sidecar entry ⇒ SidecarText is "" ⇒ thin.
	res, err := newTranscribeJob(t, st, tools, fstest.MapFS{}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tools.calls != 1 {
		t.Fatalf("transcribe ran %d times, want 1", tools.calls)
	}
	if res.Transcribed != 1 {
		t.Errorf("transcribed = %d, want 1", res.Transcribed)
	}
	got := st.transcripts["a.mp4"]
	if got == "" || got == "\n" {
		t.Errorf("transcript = %q, want the spoken text recorded", got)
	}
}

// ⚠ A RICH, already-TAGGED clip is the one case that must NOT pay for Whisper (§10 V44 selective).
// A rich description that is still untagged, and a thin description that is tagged, are BOTH
// candidates — only rich-AND-tagged is skipped, which this asserts by cost (tools.calls == 0).
func TestTranscribeJob_SkipsARichTaggedClip(t *testing.T) {
	clip := tagged(transcribeClip("rich.mp4", ""))
	st := newFakeTranscribeStore(clip)
	tools := &scriptedTools{segs: []filler.TranscriptSegment{utter("should never run")}}
	drop := sidecar("rich.mp4",
		"Original 1994 broadcast commercial for the Turbo Teen toy line, a full two-sentence description.")
	res, err := newTranscribeJob(t, st, tools, drop).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tools.calls != 0 {
		t.Errorf("transcribe ran %d times — a richly-described, already-tagged clip must not pay for Whisper", tools.calls)
	}
	if res.SkippedRich != 1 || res.Considered != 0 {
		t.Errorf("skippedRich=%d considered=%d, want the clip skipped without transcription", res.SkippedRich, res.Considered)
	}
	if len(st.transcripts) != 0 {
		t.Errorf("recorded %v, want nothing for a skipped clip", st.transcripts)
	}
}

// ⚠ A rich description does NOT exempt a clip that is still UNTAGGED — the description did not carry
// what the tagger needed, so the spoken text is the next signal to try (the OR arm).
func TestTranscribeJob_TranscribesARichButUntaggedClip(t *testing.T) {
	st := newFakeTranscribeStore(transcribeClip("untagged.mp4", "")) // rich source, no tags
	tools := &scriptedTools{segs: []filler.TranscriptSegment{utter("Kellogg's Frosted Flakes")}}
	drop := sidecar("untagged.mp4",
		"A long, genuinely descriptive sentence that clears the thin threshold with room to spare.")
	res, err := newTranscribeJob(t, st, tools, drop).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tools.calls != 1 || res.Transcribed != 1 {
		t.Errorf("calls=%d transcribed=%d, want a rich-but-untagged clip transcribed", tools.calls, res.Transcribed)
	}
}

// ⚠ Already-transcribed clips are skipped, and `""` is the only value meaning "not yet". A recorded
// transcript — even an empty one — is a final answer; re-running Whisper on it burns ~341s under
// QEMU to learn what is already known.
func TestTranscribeJob_SkipsClipsAlreadyTranscribed(t *testing.T) {
	st := newFakeTranscribeStore(
		transcribeClip("done.mp4", "already has words"),
		transcribeClip("fresh.mp4", ""),
	)
	tools := &scriptedTools{segs: []filler.TranscriptSegment{utter("hello")}}
	res, err := newTranscribeJob(t, st, tools, fstest.MapFS{}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tools.calls != 1 {
		t.Errorf("transcribe ran %d times, want 1 — only the untranscribed clip needs work", tools.calls)
	}
	if res.Considered != 1 {
		t.Errorf("considered = %d, want 1", res.Considered)
	}
}

// ⚠ **A WORDLESS clip records the NON-EMPTY sentinel and is never re-visited.** A silent visual
// spot yields no utterances; recording `TranscriptNone` (not a literal "") is what stops the job
// re-running Whisper on it forever — the same ""-vs-recorded discipline the language gate uses for
// `none`. A literal "" would be indistinguishable from "not yet transcribed" and every pass would
// pay ~341s under QEMU to re-learn the clip is silent — which is precisely the regression the
// second pass below asserts against (it went RED with a literal-"" recording).
func TestTranscribeJob_WordlessRecordsSentinelAndDoesNotRevisit(t *testing.T) {
	st := newFakeTranscribeStore(transcribeClip("silent.mp4", ""))
	tools := &scriptedTools{segs: nil} // whisper heard nothing
	res, err := newTranscribeJob(t, st, tools, fstest.MapFS{}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Wordless != 1 {
		t.Errorf("wordless = %d, want 1", res.Wordless)
	}
	// Recorded as the sentinel — present AND non-empty, so a re-list's `transcript != ""` filter
	// treats it as handled.
	got, ok := st.transcripts["silent.mp4"]
	if !ok || got != filler.TranscriptNone {
		t.Fatalf("transcripts[silent.mp4] = (%q, present=%v), want (%q, true) so the clip is never re-run",
			got, ok, filler.TranscriptNone)
	}
	if len(st.setPaths) != 1 {
		t.Fatalf("SetClipTranscript called %d times, want exactly 1 for the wordless clip", len(st.setPaths))
	}

	// Feed the recorded row back in — the clip now carries the non-empty sentinel, so a second pass
	// must recognise it as already handled and not touch the clip again.
	st2 := newFakeTranscribeStore(transcribeClip("silent.mp4", got))
	tools2 := &scriptedTools{}
	if _, err := newTranscribeJob(t, st2, tools2, fstest.MapFS{}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tools2.calls != 0 {
		t.Errorf("second pass ran transcribe %d times — a recorded wordless answer must not be re-visited", tools2.calls)
	}
}

// ⚠ A backend FAILURE (whisper unrunnable, hosted key missing) says nothing about the clip. It must
// NOT be recorded, or the next pass would skip a clip that was never actually transcribed.
func TestTranscribeJob_ABackendFailureIsNotRecorded(t *testing.T) {
	st := newFakeTranscribeStore(transcribeClip("a.mp4", ""))
	tools := &scriptedTools{err: errors.New("whisper not configured")}
	res, err := newTranscribeJob(t, st, tools, fstest.MapFS{}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 {
		t.Errorf("failed = %d, want 1", res.Failed)
	}
	if len(st.transcripts) != 0 {
		t.Errorf("recorded %v on a failure; leaving it empty is what makes the next pass retry", st.transcripts)
	}
}

// The opt-in gate: `enabled` false turns the job off entirely — no work list, no transcription.
func TestTranscribeJob_DisabledIsANoOp(t *testing.T) {
	st := newFakeTranscribeStore(transcribeClip("a.mp4", ""))
	tools := &scriptedTools{segs: []filler.TranscriptSegment{utter("hi")}}
	job := filler.NewTranscribeJob(st, tools,
		func() string { return "/filler" }, fstest.MapFS{}, func() bool { return false },
		func() time.Time { return time.Unix(1, 0) }, slog.New(slog.DiscardHandler))
	res, err := job.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tools.calls != 0 || res.Considered != 0 {
		t.Errorf("calls=%d considered=%d, want a complete no-op with the job off", tools.calls, res.Considered)
	}
}

// A nil `enabled` is also off (the un-opted-in default), never a panic.
func TestTranscribeJob_NilEnabledIsOff(t *testing.T) {
	st := newFakeTranscribeStore(transcribeClip("a.mp4", ""))
	tools := &scriptedTools{segs: []filler.TranscriptSegment{utter("hi")}}
	job := filler.NewTranscribeJob(st, tools,
		func() string { return "/filler" }, fstest.MapFS{}, nil,
		func() time.Time { return time.Unix(1, 0) }, slog.New(slog.DiscardHandler))
	res, err := job.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tools.calls != 0 || res.Considered != 0 {
		t.Errorf("calls=%d considered=%d, want off for a nil enabled closure", tools.calls, res.Considered)
	}
}

// ⚠ One pass is BOUNDED. On the local backend an unbounded first run over a fresh catalog is hours,
// holding a job worker the whole time; the timer drains it over several passes instead.
func TestTranscribeJob_BoundsOnePass(t *testing.T) {
	var clips []filler.StoreClip
	for i := 0; i < filler.TranscribeBatch+5; i++ {
		clips = append(clips, transcribeClip(string(rune('a'+i))+".mp4", ""))
	}
	st := newFakeTranscribeStore(clips...)
	tools := &scriptedTools{segs: []filler.TranscriptSegment{utter("words")}}
	res, err := newTranscribeJob(t, st, tools, fstest.MapFS{}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Considered != filler.TranscribeBatch {
		t.Errorf("considered = %d, want the batch size %d", res.Considered, filler.TranscribeBatch)
	}
	if tools.calls > filler.TranscribeBatch {
		t.Errorf("transcribe ran %d times, past the batch bound", tools.calls)
	}
}
