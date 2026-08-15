package filler_test

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
)

// The language rung (§10 V40's gate, V51b's stage). Its whole risk is that it REFUSES clips, so
// most of these assert that it does not.
//
// ⚠ **Ported from `languagejob_test.go` when V51b retired the sweep, and two of the old cases went
// with it rather than being rewritten.** `BoundsOnePass` tested the job's batch constant and
// `SkipsClipsAlreadyChecked`'s work-list half tested its own filtering — both are the runner's
// concerns now, and both are covered against the runner in `pipeline_test.go`. Keeping them here
// as stage tests would have meant asserting behaviour this file no longer owns.

// fakeLangStore records what the rung wrote without a database.
type fakeLangStore struct {
	langs  map[string]string // path → recorded language
	setErr error
}

func newFakeLangStore() *fakeLangStore { return &fakeLangStore{langs: map[string]string{}} }

func (f *fakeLangStore) SetClipLanguage(_ context.Context, path, language string, _ time.Time) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.langs[path] = language
	return nil
}

// fixedDetector answers the same thing for every clip.
type fixedDetector struct {
	lang        string
	err         error
	unavailable string
	calls       int
}

func (d *fixedDetector) UnavailableReason() string { return d.unavailable }

func (d *fixedDetector) DetectLanguage(context.Context, string, int64, int64) (string, error) {
	d.calls++
	return d.lang, d.err
}

func langClip(path, language string) filler.StoreClip {
	return filler.StoreClip{Clip: filler.Clip{
		Hash: path + "-hash", Path: path, Name: path, Kind: filler.Commercial,
		DurationMs: 30_000, Language: language,
	}}
}

func newLangStage(st filler.LanguageClipStore, d filler.LanguageDetector, want string) *filler.LanguageStage {
	return filler.NewLanguageStage(d, st, "/filler",
		func() string { return want },
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() })
}

// Missing configuration is not a transient inference failure. It cannot improve with backoff, so
// the optional rung must step aside immediately instead of holding every new clip for 35 minutes.
func TestLanguageStage_UnavailableBackendDoesNotApply(t *testing.T) {
	det := &fixedDetector{unavailable: "the local language model is not configured"}
	applies, why := newLangStage(newFakeLangStore(), det, "en").
		Applies(context.Background(), langClip("a.mp4", ""))
	if applies {
		t.Fatal("an unavailable detector applied; the runner would waste its retry ladder")
	}
	if why != det.unavailable {
		t.Errorf("reason = %q, want %q", why, det.unavailable)
	}
	if det.calls != 0 {
		t.Errorf("detector called %d times, want no inference attempt", det.calls)
	}
}

// THE behaviour: a confidently foreign clip is refused.
//
// ⚠ It returns a REJECT VERDICT; it does not tombstone the clip itself. The runner owns the
// tombstone (`Pipeline.tombstone`), which is what stops a future rule shipping a reject that
// records a reason and leaves the clip playable.
func TestLanguageStage_RejectsAForeignClip(t *testing.T) {
	st := newFakeLangStore()
	out, err := newLangStage(st, &fixedDetector{lang: "es"}, "en").
		Run(context.Background(), langClip("a.mp4", ""))
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != filler.VerdictReject || out.Reason != filler.ReasonLanguage {
		t.Errorf("verdict=%v reason=%q, want the Spanish clip refused", out.Verdict, out.Reason)
	}
	// ⚠ The answer is recorded even though the clip is going away. Without it a re-scan that
	// resurrects the row would re-detect from scratch — ~341s under QEMU, for a known answer.
	if st.langs["a.mp4"] != "es" {
		t.Errorf("language = %q, want it recorded before the refusal", st.langs["a.mp4"])
	}
	// ⚠ The detail carries the MEASURED fact, which is what makes a reject arguable rather than an
	// assertion. A bare code tells an operator nothing about what to change.
	if out.Detail == "" {
		t.Error("detail is empty — a reject must say what was heard and what was wanted")
	}
}

// ⚠ **Silence must never be refused.** A wordless visual spot has no language, and those are often
// the best filler — `none` is a final answer that KEEPS.
func TestLanguageStage_KeepsASilentClip(t *testing.T) {
	st := newFakeLangStore()
	out, err := newLangStage(st, &fixedDetector{lang: filler.LangNone}, "en").
		Run(context.Background(), langClip("quiet.mp4", ""))
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != filler.VerdictContinue {
		t.Errorf("verdict = %v — a clip with no speech is not a foreign clip", out.Verdict)
	}
	// Recorded as `none`, so it is never re-checked.
	if st.langs["quiet.mp4"] != filler.LangNone {
		t.Errorf("language = %q, want %q recorded so the rung stops re-checking it",
			st.langs["quiet.mp4"], filler.LangNone)
	}
}

// ⚠ A backend FAILURE says nothing about the clip. It must not be recorded (or the rung would
// never look again) and must not reject (or a missing whisper would empty the catalog).
//
// ⚠ The detector returns a LANGUAGE **and** an error, which looks odd and is the whole point: with
// `("", err)` this test cannot tell "handled the error" from "fell through to the undetermined
// branch", and it passed with the error check deleted. A partial answer alongside a failure is
// exactly what a backend that half-worked produces, and it must still not be trusted.
func TestLanguageStage_ABackendFailureNeitherRecordsNorRejects(t *testing.T) {
	st := newFakeLangStore()
	det := &fixedDetector{lang: "es", err: errors.New("whisper missing")}
	out, err := newLangStage(st, det, "en").Run(context.Background(), langClip("a.mp4", ""))
	if err == nil {
		t.Fatal("want an error so the runner retries with backoff")
	}
	if out.Verdict == filler.VerdictReject {
		t.Error("refused the clip on a detector failure — a broken backend must not empty the catalog")
	}
	if len(st.langs) != 0 {
		t.Errorf("recorded %v on a failure; leaving it empty is what makes a later pass retry", st.langs)
	}
}

// The same for an "I could not tell" answer, which is a successful call with no verdict.
//
// ⚠ It is an ERROR rather than a silent pass, and that is V51b's deliberate change from V40. The
// old job left the clip unrecorded and re-asked forever — so an install with `want` set and no
// multilingual model re-ran the same unanswerable question hourly, at full cost. The retry ladder
// bounds it: three attempts, then the rung is skipped and the clip advances.
func TestLanguageStage_UndeterminedIsNotAVerdict(t *testing.T) {
	st := newFakeLangStore()
	out, err := newLangStage(st, &fixedDetector{lang: filler.LangUndetermined}, "en").
		Run(context.Background(), langClip("a.mp4", ""))
	if err == nil {
		t.Fatal("want an error so the attempt is counted and eventually given up on")
	}
	if out.Verdict == filler.VerdictReject || len(st.langs) != 0 {
		t.Errorf("verdict=%v recorded=%v, want neither for an undetermined answer", out.Verdict, st.langs)
	}
}

// A matching clip is kept and recorded.
func TestLanguageStage_KeepsAMatchingClip(t *testing.T) {
	st := newFakeLangStore()
	out, err := newLangStage(st, &fixedDetector{lang: "en"}, "en").
		Run(context.Background(), langClip("a.mp4", ""))
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != filler.VerdictContinue {
		t.Errorf("verdict = %v — this clip is in the expected language", out.Verdict)
	}
	if st.langs["a.mp4"] != "en" {
		t.Errorf("langs = %v, want the match recorded", st.langs)
	}
}

// ⚠ **An already-heard clip does not APPLY**, and `""` is the only value meaning "not yet".
// Without this the rung re-runs the detector every time a clip is rewound past it — and `none` is
// a final answer, not an absence.
func TestLanguageStage_DoesNotApplyToAnAlreadyHeardClip(t *testing.T) {
	det := &fixedDetector{lang: "en"}
	stage := newLangStage(newFakeLangStore(), det, "en")
	for _, tc := range []struct {
		name string
		clip filler.StoreClip
		want bool
	}{
		{"never heard", langClip("fresh.mp4", ""), true},
		{"heard, English", langClip("checked.mp4", "en"), false},
		{"heard, wordless", langClip("silent.mp4", filler.LangNone), false},
	} {
		if got, _ := stage.Applies(context.Background(), tc.clip); got != tc.want {
			t.Errorf("%s: applies = %v, want %v", tc.name, got, tc.want)
		}
	}
	if det.calls != 0 {
		t.Errorf("Applies called the detector %d times — it must answer WITHOUT exec", det.calls)
	}
}

// ⚠ The gate off (no `filler.language`) makes the rung inapplicable — with a REASON, so the
// ladder says why rather than showing a silent gap.
func TestLanguageStage_OffStatesAreInapplicableWithAReason(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage *filler.LanguageStage
	}{
		{"no wanted language", newLangStage(newFakeLangStore(), &fixedDetector{lang: "en"}, "")},
		{"no backend", newLangStage(newFakeLangStore(), nil, "en")},
	} {
		applies, note := tc.stage.Applies(context.Background(), langClip("a.mp4", ""))
		if applies {
			t.Errorf("%s: applies, want not", tc.name)
		}
		if note == "" {
			t.Errorf("%s: no note — a skipped rung has to say why", tc.name)
		}
	}
}

// ⚠ A store failure is an ERROR, not a silent reject. The clip's answer is unrecorded, so refusing
// it here would tombstone a clip the next pass would have to re-hear anyway.
func TestLanguageStage_AFailedRecordIsNotAReject(t *testing.T) {
	st := newFakeLangStore()
	st.setErr = errors.New("db down")
	out, err := newLangStage(st, &fixedDetector{lang: "es"}, "en").
		Run(context.Background(), langClip("a.mp4", ""))
	if err == nil {
		t.Fatal("want the store failure surfaced")
	}
	if out.Verdict == filler.VerdictReject {
		t.Error("refused the clip although its language was never recorded")
	}
}

// recordingAsker captures which model it was asked for, so a test can prove the value was read
// LIVE rather than captured when the detector was built.
type recordingAsker struct {
	models []string
	answer string
}

func (r *recordingAsker) AskAboutAudio(_ context.Context, req filler.AudioAsk) (string, error) {
	r.models = append(r.models, req.Model)
	return r.answer, nil
}

// ⚠ **THE regression this exists for, found live and not by a test.** The first cut resolved
// `llm.url`/`llm.model`/`llm.api_key` ONCE at construction and baked them into a client. Changing
// the model in Settings then did nothing: the detector kept calling whatever was configured when
// the process started, which had no audio input, and every clip failed with a real 404 —
// "No endpoints found that support input audio" — about a request the operator believed they had
// already fixed. The error was accurate and the config looked right, which is what made it
// expensive to find.
//
// Everything else in this feature reads live (`filler.dir`, `filler.language`). The one setting
// that decides whether the backend can work at all has to as well.
func TestHostedLanguage_ReadsItsModelLivePerCall(t *testing.T) {
	rec := &recordingAsker{answer: "en"}
	model := "stale-model-with-no-audio"

	det := filler.NewHostedLanguage(
		func() filler.AudioAsker { return rec },
		func() string { return model }, // read per call, like a settings closure
		"", t.TempDir())

	// A real file is needed for the ffmpeg extract; a silent one is enough to reach the ask.
	// If ffmpeg is unavailable the detector reports undetermined, which still exercises nothing
	// useful — so skip rather than assert a false pass.
	dir := t.TempDir()
	clip := filepath.Join(dir, "c.wav")
	if err := exec.Command("ffmpeg", "-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3", "-ac", "1", "-ar", "16000",
		"-y", clip).Run(); err != nil {
		t.Skip("ffmpeg unavailable")
	}

	if _, err := det.DetectLanguage(context.Background(), clip, 0, 3000); err != nil {
		t.Fatal(err)
	}
	// The operator changes the model in Settings — no restart, no rebuild.
	model = "corrected-model-with-audio"
	if _, err := det.DetectLanguage(context.Background(), clip, 0, 3000); err != nil {
		t.Fatal(err)
	}

	if len(rec.models) != 2 {
		t.Fatalf("asked %d times, want 2", len(rec.models))
	}
	if rec.models[1] != "corrected-model-with-audio" {
		t.Errorf("second call used %q, want the CORRECTED model — a captured value means changing "+
			"the setting silently does nothing and every clip 404s", rec.models[1])
	}
}

// A nil asker (hosted selected, nothing configured) keeps every clip rather than erroring.
func TestHostedLanguage_UnconfiguredIsInertNotBroken(t *testing.T) {
	det := filler.NewHostedLanguage(func() filler.AudioAsker { return nil }, func() string { return "" }, "", t.TempDir())
	got, err := det.DetectLanguage(context.Background(), "/nonexistent.mp4", 0, 10_000)
	if err != nil || got != filler.LangUndetermined {
		t.Errorf("got (%q, %v), want (undetermined, nil) — an unconfigured backend must not reject", got, err)
	}
}
