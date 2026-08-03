package filler_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
)

// The language job (§10 V40). Its whole risk is that it DELETES clips, so most of these assert
// that it does not.

// fakeLangStore records what the job did without a database.
type fakeLangStore struct {
	clips   []filler.StoreClip
	langs   map[string]string // path → recorded language
	removed []string
	setErr  error
	rmErr   error
}

func newFakeLangStore(clips ...filler.StoreClip) *fakeLangStore {
	return &fakeLangStore{clips: clips, langs: map[string]string{}}
}

func (f *fakeLangStore) ListClips(context.Context, filler.ClipQuery) ([]filler.StoreClip, error) {
	return f.clips, nil
}

func (f *fakeLangStore) SetClipLanguage(_ context.Context, path, language string, _ time.Time) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.langs[path] = language
	return nil
}

func (f *fakeLangStore) SetClipsRemoved(_ context.Context, paths []string, _ time.Time) (int, error) {
	if f.rmErr != nil {
		return 0, f.rmErr
	}
	f.removed = append(f.removed, paths...)
	return len(paths), nil
}

// fixedDetector answers the same thing for every clip.
type fixedDetector struct {
	lang  string
	err   error
	calls int
}

func (d *fixedDetector) DetectLanguage(context.Context, string, int64, int64) (string, error) {
	d.calls++
	return d.lang, d.err
}

func langClip(path, language string) filler.StoreClip {
	return filler.StoreClip{Clip: filler.Clip{
		Hash: path, Path: path, Name: path, Kind: filler.Commercial,
		DurationMs: 30_000, Language: language,
	}}
}

func newJob(t *testing.T, st filler.LanguageStore, d filler.LanguageDetector, want string) *filler.LanguageJob {
	t.Helper()
	return filler.NewLanguageJob(st, d,
		func() string { return "/filler" }, func() string { return want },
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() },
		slog.New(slog.DiscardHandler))
}

// THE behaviour: a confidently foreign clip is tombstoned.
func TestLanguageJob_RemovesAForeignClip(t *testing.T) {
	st := newFakeLangStore(langClip("a.mp4", ""))
	res, err := newJob(t, st, &fixedDetector{lang: "es"}, "en").Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Rejected != 1 || len(st.removed) != 1 || st.removed[0] != "a.mp4" {
		t.Errorf("rejected=%d removed=%v, want the Spanish clip tombstoned", res.Rejected, st.removed)
	}
	// ⚠ The answer is recorded even though the clip is going away. Without it a re-scan that
	// resurrects the row would re-detect from scratch — ~341s under QEMU, for a known answer.
	if st.langs["a.mp4"] != "es" {
		t.Errorf("language = %q, want it recorded before the removal", st.langs["a.mp4"])
	}
}

// ⚠ **Silence must never be removed.** A wordless visual spot has no language, and those are often
// the best filler — `none` is a final answer that KEEPS.
func TestLanguageJob_KeepsASilentClip(t *testing.T) {
	st := newFakeLangStore(langClip("quiet.mp4", ""))
	res, err := newJob(t, st, &fixedDetector{lang: filler.LangNone}, "en").Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.removed) != 0 {
		t.Errorf("removed %v — a clip with no speech is not a foreign clip", st.removed)
	}
	if res.Silent != 1 {
		t.Errorf("silent = %d, want 1", res.Silent)
	}
	// Recorded as `none`, so it is never re-checked.
	if st.langs["quiet.mp4"] != filler.LangNone {
		t.Errorf("language = %q, want %q recorded so the job stops re-checking it",
			st.langs["quiet.mp4"], filler.LangNone)
	}
}

// ⚠ A backend FAILURE says nothing about the clip. It must not be recorded (or the next pass would
// skip it) and must not reject (or a missing whisper would empty the catalog).
//
// ⚠ The detector returns a LANGUAGE **and** an error, which looks odd and is the whole point: with
// `("", err)` this test cannot tell "handled the error" from "fell through to the undetermined
// branch", and it passed with the error check deleted. A partial answer alongside a failure is
// exactly what a backend that half-worked produces, and it must still not be trusted.
func TestLanguageJob_ABackendFailureNeitherRecordsNorRejects(t *testing.T) {
	st := newFakeLangStore(langClip("a.mp4", ""))
	det := &fixedDetector{lang: "es", err: errors.New("whisper missing")}
	res, err := newJob(t, st, det, "en").Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.removed) != 0 {
		t.Errorf("removed %v on a detector failure — a broken backend must not empty the catalog", st.removed)
	}
	if len(st.langs) != 0 {
		t.Errorf("recorded %v on a failure; leaving it empty is what makes the next pass retry", st.langs)
	}
	if res.Failed != 1 {
		t.Errorf("failed = %d, want 1", res.Failed)
	}
}

// The same for an "I could not tell" answer, which is a successful call with no verdict.
func TestLanguageJob_UndeterminedIsNotAVerdict(t *testing.T) {
	st := newFakeLangStore(langClip("a.mp4", ""))
	_, err := newJob(t, st, &fixedDetector{lang: filler.LangUndetermined}, "en").Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.removed) != 0 || len(st.langs) != 0 {
		t.Errorf("removed=%v recorded=%v, want neither for an undetermined answer", st.removed, st.langs)
	}
}

// A matching clip is kept and recorded.
func TestLanguageJob_KeepsAMatchingClip(t *testing.T) {
	st := newFakeLangStore(langClip("a.mp4", ""))
	res, err := newJob(t, st, &fixedDetector{lang: "en"}, "en").Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.removed) != 0 {
		t.Errorf("removed %v — this clip is in the expected language", st.removed)
	}
	if res.Detected != 1 || st.langs["a.mp4"] != "en" {
		t.Errorf("detected=%d langs=%v, want the match recorded", res.Detected, st.langs)
	}
}

// ⚠ **Already-checked clips are skipped**, and `""` is the only value meaning "not yet". Without
// this the job re-runs the detector over the whole catalog every pass — hours per cycle on the
// local backend.
func TestLanguageJob_SkipsClipsAlreadyChecked(t *testing.T) {
	st := newFakeLangStore(
		langClip("checked.mp4", "en"),
		langClip("silent.mp4", filler.LangNone),
		langClip("fresh.mp4", ""),
	)
	det := &fixedDetector{lang: "en"}
	res, err := newJob(t, st, det, "en").Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if det.calls != 1 {
		t.Errorf("detector ran %d times, want 1 — only the unchecked clip needs work", det.calls)
	}
	if res.Considered != 1 {
		t.Errorf("considered = %d, want 1", res.Considered)
	}
}

// An empty `filler.language` turns the gate off entirely — no detection, no cost.
func TestLanguageJob_EmptyWantIsANoOp(t *testing.T) {
	st := newFakeLangStore(langClip("a.mp4", ""))
	det := &fixedDetector{lang: "es"}
	res, err := newJob(t, st, det, "").Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if det.calls != 0 || len(st.removed) != 0 || res.Considered != 0 {
		t.Errorf("calls=%d removed=%v considered=%d, want a complete no-op with the gate off",
			det.calls, st.removed, res.Considered)
	}
}

// ⚠ One pass is BOUNDED. On the local backend an unbounded first run over a fresh catalog is
// hours, holding a job worker the whole time; the timer drains it over several passes instead.
func TestLanguageJob_BoundsOnePass(t *testing.T) {
	var clips []filler.StoreClip
	for i := 0; i < filler.LanguageBatch+10; i++ {
		clips = append(clips, langClip(string(rune('a'+i%26))+"-clip.mp4", ""))
	}
	st := newFakeLangStore(clips...)
	det := &fixedDetector{lang: "en"}
	res, err := newJob(t, st, det, "en").Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Considered != filler.LanguageBatch {
		t.Errorf("considered = %d, want the batch size %d", res.Considered, filler.LanguageBatch)
	}
	if det.calls > filler.LanguageBatch {
		t.Errorf("detector ran %d times, past the batch bound", det.calls)
	}
}

// ⚠ If the tombstone fails the answer is STILL recorded, so the next pass does not pay to
// re-detect a clip whose language is already known.
func TestLanguageJob_RecordsTheAnswerEvenWhenRemovalFails(t *testing.T) {
	st := newFakeLangStore(langClip("a.mp4", ""))
	st.rmErr = errors.New("db down")
	res, err := newJob(t, st, &fixedDetector{lang: "es"}, "en").Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.langs["a.mp4"] != "es" {
		t.Errorf("language = %q, want it recorded despite the failed removal", st.langs["a.mp4"])
	}
	if res.Rejected != 0 {
		t.Errorf("rejected = %d, want 0 — nothing was actually removed", res.Rejected)
	}
}
