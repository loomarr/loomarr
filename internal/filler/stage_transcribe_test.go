package filler_test

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
)

// The transcribe rung (§10 V44's job, V51b's stage). Its whole risk is COST: Whisper is ~341s a
// clip under QEMU, so most of these assert that it does NOT run — a rich, already-tagged clip does
// not apply, an already-transcribed clip does not apply, and a wordless clip is recorded once so
// it is never re-run.
//
// ⚠ **Ported from `transcribejob_test.go` when V51b retired the sweep.** `BoundsOnePass` went with
// it: the bound is now `filler.pipeline.max_whisper`, enforced by the runner and tested against
// the runner in `pipeline_test.go`. Asserting it here would be testing a constant this file no
// longer owns.

// fakeTranscribeStore records what the rung wrote without a database.
type fakeTranscribeStore struct {
	transcripts map[string]string // path → recorded transcript
	setPaths    []string          // order + count of SetClipTranscript calls
	setErr      error
}

func newFakeTranscribeStore() *fakeTranscribeStore {
	return &fakeTranscribeStore{transcripts: map[string]string{}}
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
// utterances (or an error). The rest satisfy the interface and are never called by this rung.
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
func (s *scriptedTools) Boundaries(context.Context, string, int64, int64) ([]filler.Interval, []filler.Interval, error) {
	return nil, nil, nil
}
func (s *scriptedTools) GrayFrames(context.Context, string, int64, int64) ([][]byte, error) {
	return nil, nil
}
func (s *scriptedTools) KeyframesIn(context.Context, string, int64, int64, int) ([][]byte, error) {
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
//
// ⚠ Hash is DERIVED from the path rather than equal to it. A fixture that sets the two to the same
// string cannot tell a hash-keyed call from a path-keyed one, which is how two shipped bugs
// survived their own tests (see conformance_filler.go).
func transcribeClip(path, transcript string) filler.StoreClip {
	return filler.StoreClip{Clip: filler.Clip{
		Hash: path + "-hash", Path: path, Name: path, Kind: filler.Commercial,
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

// newTranscribeStage wires the rung with scripted tools and a sidecar FS, always ENABLED.
func newTranscribeStage(st filler.TranscribeClipStore, tools filler.MediaTools, drop fstest.MapFS) *filler.TranscribeStage {
	return filler.NewTranscribeStage(tools, st, "/filler", drop,
		func() bool { return true },
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() })
}

// sidecar builds a MapFS with one `<path minus ext>.info.json` carrying `description`.
func sidecar(clipPath, description string) fstest.MapFS {
	key := clipPath[:len(clipPath)-len(".mp4")] + ".info.json"
	return fstest.MapFS{key: {Data: []byte(`{"description":"` + description + `"}`)}}
}

// runIfApplies is the two-question shape every rung has: does this apply, and if so what happens.
// Returning the applied flag lets a test assert "did not run" by COST rather than by outcome.
func runIfApplies(t *testing.T, s filler.Stage, c filler.StoreClip) (filler.StageResult, bool) {
	t.Helper()
	if applies, _ := s.Applies(context.Background(), c); !applies {
		return filler.StageResult{}, false
	}
	out, err := s.Run(context.Background(), c)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return out, true
}

// THE behaviour: a clip with a thin (empty) source description is transcribed and the text stored.
func TestTranscribeStage_TranscribesAThinSourceClip(t *testing.T) {
	st := newFakeTranscribeStore()
	tools := &scriptedTools{segs: []filler.TranscriptSegment{utter("buy our cereal today")}}
	// No sidecar entry ⇒ SidecarText is "" ⇒ thin.
	_, applied := runIfApplies(t, newTranscribeStage(st, tools, fstest.MapFS{}), transcribeClip("a.mp4", ""))
	if !applied || tools.calls != 1 {
		t.Fatalf("applied=%v calls=%d, want the thin clip transcribed once", applied, tools.calls)
	}
	if got := st.transcripts["a.mp4"]; got == "" || got == "\n" {
		t.Errorf("transcript = %q, want the spoken text recorded", got)
	}
}

// ⚠ A RICH, already-TAGGED clip is the one case that must NOT pay for Whisper (§10 V44 selective).
// A rich description that is still untagged, and a thin description that is tagged, are BOTH
// candidates — only rich-AND-tagged is skipped, which this asserts by cost (tools.calls == 0).
func TestTranscribeStage_SkipsARichTaggedClip(t *testing.T) {
	st := newFakeTranscribeStore()
	tools := &scriptedTools{segs: []filler.TranscriptSegment{utter("should never run")}}
	drop := sidecar("rich.mp4",
		"Original 1994 broadcast commercial for the Turbo Teen toy line, a full two-sentence description.")
	_, applied := runIfApplies(t, newTranscribeStage(st, tools, drop), tagged(transcribeClip("rich.mp4", "")))
	if applied {
		t.Error("a richly-described, already-tagged clip applied — it must not pay for Whisper")
	}
	if tools.calls != 0 || len(st.transcripts) != 0 {
		t.Errorf("calls=%d recorded=%v, want nothing for a skipped clip", tools.calls, st.transcripts)
	}
}

// ⚠ A rich description does NOT exempt a clip that is still UNTAGGED — the description did not
// carry what the tagger needed, so the spoken text is the next signal to try (the OR arm).
func TestTranscribeStage_TranscribesARichButUntaggedClip(t *testing.T) {
	st := newFakeTranscribeStore()
	tools := &scriptedTools{segs: []filler.TranscriptSegment{utter("Kellogg's Frosted Flakes")}}
	drop := sidecar("untagged.mp4",
		"A long, genuinely descriptive sentence that clears the thin threshold with room to spare.")
	_, applied := runIfApplies(t, newTranscribeStage(st, tools, drop), transcribeClip("untagged.mp4", ""))
	if !applied || tools.calls != 1 {
		t.Errorf("applied=%v calls=%d, want a rich-but-untagged clip transcribed", applied, tools.calls)
	}
}

// ⚠ Already-transcribed clips do not apply, and `""` is the only value meaning "not yet". A
// recorded transcript is a final answer; re-running Whisper on it burns ~341s under QEMU to learn
// what is already known.
func TestTranscribeStage_SkipsClipsAlreadyTranscribed(t *testing.T) {
	tools := &scriptedTools{segs: []filler.TranscriptSegment{utter("hello")}}
	s := newTranscribeStage(newFakeTranscribeStore(), tools, fstest.MapFS{})
	if applies, _ := s.Applies(context.Background(), transcribeClip("done.mp4", "already has words")); applies {
		t.Error("a transcribed clip applied — Whisper would run again for nothing")
	}
	if applies, _ := s.Applies(context.Background(), transcribeClip("fresh.mp4", "")); !applies {
		t.Error("an untranscribed clip did not apply")
	}
	if tools.calls != 0 {
		t.Errorf("Applies transcribed %d times — it must answer WITHOUT exec", tools.calls)
	}
}

// ⚠ **A WORDLESS clip records the NON-EMPTY sentinel and is never re-visited.** A silent visual
// spot yields no utterances; recording `TranscriptNone` (not a literal "") is what stops the rung
// re-running Whisper on it forever — the same ""-vs-recorded discipline the language gate uses for
// `none`. A literal "" would be indistinguishable from "not yet transcribed", which is precisely
// the regression the second pass below asserts against (it went RED with a literal-"" recording).
func TestTranscribeStage_WordlessRecordsSentinelAndDoesNotRevisit(t *testing.T) {
	st := newFakeTranscribeStore()
	tools := &scriptedTools{segs: nil} // whisper heard nothing
	out, applied := runIfApplies(t, newTranscribeStage(st, tools, fstest.MapFS{}), transcribeClip("silent.mp4", ""))
	if !applied || out.Verdict != filler.VerdictContinue {
		t.Fatalf("applied=%v verdict=%v, want a wordless clip carried forward", applied, out.Verdict)
	}
	// Recorded as the sentinel — present AND non-empty, so an `!= ""` check treats it as handled.
	got, ok := st.transcripts["silent.mp4"]
	if !ok || got != filler.TranscriptNone {
		t.Fatalf("transcripts[silent.mp4] = (%q, present=%v), want (%q, true) so the clip is never re-run",
			got, ok, filler.TranscriptNone)
	}
	if len(st.setPaths) != 1 {
		t.Fatalf("SetClipTranscript called %d times, want exactly 1 for the wordless clip", len(st.setPaths))
	}

	// Feed the recorded row back in — the clip now carries the non-empty sentinel, so a second look
	// must recognise it as already handled.
	tools2 := &scriptedTools{}
	_, applied2 := runIfApplies(t, newTranscribeStage(newFakeTranscribeStore(), tools2, fstest.MapFS{}),
		transcribeClip("silent.mp4", got))
	if applied2 || tools2.calls != 0 {
		t.Errorf("applied=%v calls=%d — a recorded wordless answer must not be re-visited", applied2, tools2.calls)
	}
}

// ⚠ A backend FAILURE (whisper unrunnable, hosted key missing) says nothing about the clip. It must
// NOT be recorded, or the rung would skip a clip that was never actually transcribed.
//
// ⚠ It returns an ERROR rather than a verdict, which is what puts the clip on the retry ladder —
// and `transcribe` is deliberately not in `fatalStages`, so exhausting those retries skips the
// rung and lets the clip advance. A missing transcript must never strand a commercial.
func TestTranscribeStage_ABackendFailureIsNotRecorded(t *testing.T) {
	st := newFakeTranscribeStore()
	tools := &scriptedTools{err: errors.New("whisper not configured")}
	s := newTranscribeStage(st, tools, fstest.MapFS{})
	if applies, _ := s.Applies(context.Background(), transcribeClip("a.mp4", "")); !applies {
		t.Fatal("the clip should apply; the failure is at run time")
	}
	if _, err := s.Run(context.Background(), transcribeClip("a.mp4", "")); err == nil {
		t.Fatal("want an error so the runner retries")
	}
	if len(st.transcripts) != 0 {
		t.Errorf("recorded %v on a failure; leaving it empty is what makes a later pass retry", st.transcripts)
	}
}

// The opt-in gate: `enabled` false — and a nil closure, the un-opted-in default — make the rung
// inapplicable rather than panicking, with a note so the ladder says why.
func TestTranscribeStage_OffStatesAreInapplicableWithAReason(t *testing.T) {
	tools := &scriptedTools{segs: []filler.TranscriptSegment{utter("hi")}}
	for _, tc := range []struct {
		name    string
		enabled func() bool
	}{
		{"switched off", func() bool { return false }},
		{"never opted in", nil},
	} {
		s := filler.NewTranscribeStage(tools, newFakeTranscribeStore(), "/filler", fstest.MapFS{},
			tc.enabled, func() time.Time { return time.Unix(1, 0) })
		applies, note := s.Applies(context.Background(), transcribeClip("a.mp4", ""))
		if applies {
			t.Errorf("%s: applies, want not", tc.name)
		}
		if note == "" {
			t.Errorf("%s: no note — a skipped rung has to say why", tc.name)
		}
	}
	if tools.calls != 0 {
		t.Errorf("transcribed %d times with the rung off", tools.calls)
	}
}
