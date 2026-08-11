package filler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/taxonomy"
)

// The vision rung (§10 V44's job, V51b's stage) — the expensive LAST tier. Its risk is the same as
// the transcribe rung's, one level up: keyframes + a multimodal call cost real money and time, so
// these assert it runs ONLY where the cheaper tiers left a gap, GROUNDS every tag against the
// on-screen text the model says it read (an inferred brand is dropped), and STAMPS vision_tagged so
// a re-run never pays twice.
//
// ⚠ **Ported from `visionjob_test.go` when V51b retired the sweep.** `BoundsOnePass` went with it —
// the bound is `filler.pipeline.max_vision`, enforced by the runner and tested there. Everything
// else survives, because the grounding rule is the part that matters and it did not change.

// fakeVisionStore records what the rung wrote and stamped, without a database.
type fakeVisionStore struct {
	tags     map[string]visionWrite // path → recorded vision tags
	setPaths []string               // order + count of SetClipVisionTags calls
	setErr   error
}

// visionWrite is one recorded SetClipVisionTags call. vision_tagged is always true on this path
// (the store method stamps it), so we record the grounded values a re-run's candidacy turns on,
// plus the suggestedEra the frame-heuristic tier feeds through.
type visionWrite struct {
	brand, visibleText, category string
	era, suggestedEra            int
}

func newFakeVisionStore() *fakeVisionStore {
	return &fakeVisionStore{tags: map[string]visionWrite{}}
}

func (f *fakeVisionStore) SetClipVisionTags(_ context.Context, path, brand, visibleText string, era, suggestedEra int, category string, _ time.Time) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.setPaths = append(f.setPaths, path)
	f.tags[path] = visionWrite{brand: brand, visibleText: visibleText, category: category, era: era, suggestedEra: suggestedEra}
	return nil
}

// ListTaxa serves the REAL seed forest (§10 V45a): the vision tier grounds its category against it,
// so a vision-read `toys` resolves and an off-vocabulary slug is dropped — the same resolve-or-drop
// gate the text tier uses.
func (f *fakeVisionStore) ListTaxa(_ context.Context) ([]taxonomy.Taxon, error) {
	return taxonomy.SeedForest(), nil
}

// scriptedVision is a MediaTools whose only real method is Keyframes, returning a fixed set of JPEG
// stand-ins (any non-empty bytes — the rung never decodes them, only forwards them to the provider)
// or an error. The rest satisfy the interface and are never called by this rung.
type scriptedVision struct {
	frames [][]byte
	err    error
	calls  int
}

func (s *scriptedVision) KeyframesIn(ctx context.Context, f string, _, _ int64, n int) ([][]byte, error) {
	return s.Keyframes(ctx, f, n)
}

func (s *scriptedVision) Keyframes(_ context.Context, _ string, _ int) ([][]byte, error) {
	s.calls++
	return s.frames, s.err
}

func (s *scriptedVision) Chapters(context.Context, string) ([]filler.Chapter, error) { return nil, nil }
func (s *scriptedVision) BlackSilence(context.Context, string) ([]filler.Interval, []filler.Interval, error) {
	return nil, nil, nil
}
func (s *scriptedVision) Transcribe(context.Context, string, int64, int64) ([]filler.TranscriptSegment, error) {
	return nil, nil
}
func (s *scriptedVision) GrayFrames(context.Context, string, int64, int64) ([][]byte, error) {
	return nil, nil
}
func (s *scriptedVision) Cut(context.Context, string, int64, int64, string) error { return nil }

// scriptedProvider is a VisionProvider that returns one fixed JSON answer (or an error), and counts
// its calls so a test can assert the model was (or was not) asked.
type scriptedProvider struct {
	answer string
	err    error
	calls  int
}

func (p *scriptedProvider) AskAboutImages(context.Context, string, [][]byte) (llm.Response, error) {
	p.calls++
	if p.err != nil {
		return llm.Response{}, p.err
	}
	return llm.Response{Content: p.answer}, nil
}

// oneFrame is a non-empty keyframe stand-in — the rung forwards bytes, it never decodes them.
var oneFrame = [][]byte{[]byte("\xff\xd8jpeg\xff\xd9")}

// visionClip builds an untagged, not-yet-vision-tagged candidate clip. Hash is DERIVED from the
// path rather than equal to it, so a hash-keyed call and a path-keyed one stay distinguishable.
func visionClip(path string) filler.StoreClip {
	return filler.StoreClip{Clip: filler.Clip{
		Hash: path + "-hash", Path: path, Name: path, Kind: filler.Commercial, DurationMs: 30_000,
	}}
}

// wordless marks a clip as a silent visual spot — the case vision exists for: no dialogue
// (Language "none"), no transcript beyond the wordless sentinel. It stays untagged, so it is a
// candidate.
func wordless(c filler.StoreClip) filler.StoreClip {
	c.Language = filler.LangNone
	c.Transcript = filler.TranscriptNone
	return c
}

// fullyTagged marks a clip resolved by the cheaper tiers (era + audience + category), so it is NOT
// a vision candidate.
func fullyTagged(c filler.StoreClip) filler.StoreClip {
	c.Era = 1994
	c.Audience = filler.Kids
	c.Category = "toys"
	return c
}

// newVisionStage wires the rung with a scripted store/tools/provider, always ENABLED.
func newVisionStage(st filler.VisionClipStore, tools filler.MediaTools, provider llm.VisionProvider) *filler.VisionStage {
	return filler.NewVisionStage(tools, provider, st, "/filler",
		func() bool { return true },
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() })
}

// runVision applies + runs, failing the test on an unexpected error. It returns the applied flag so
// a test can assert "did not run" by COST rather than by outcome.
func runVision(t *testing.T, s *filler.VisionStage, c filler.StoreClip) bool {
	t.Helper()
	if applies, _ := s.Applies(context.Background(), c); !applies {
		return false
	}
	if _, err := s.Run(context.Background(), c); err != nil {
		t.Fatalf("run: %v", err)
	}
	return true
}

// THE grounding property: a brand that appears in the returned visibleText is PERSISTED.
func TestVisionStage_PersistsBrandGroundedInVisibleText(t *testing.T) {
	st := newFakeVisionStore()
	// The model read "KELLOGG'S FROSTED FLAKES" off the box and named the advertiser — the brand
	// appears literally in the visible text, so it grounds.
	prov := &scriptedProvider{answer: `{"visibleText":"KELLOGG'S FROSTED FLAKES","brand":"Kellogg's","era":0,"category":""}`}
	if !runVision(t, newVisionStage(st, &scriptedVision{frames: oneFrame}, prov), wordless(visionClip("silent.mp4"))) {
		t.Fatal("the wordless clip did not apply")
	}
	got := st.tags["silent.mp4"]
	if got.brand != "Kellogg's" {
		t.Errorf("brand = %q, want %q grounded off the visible text", got.brand, "Kellogg's")
	}
	if got.visibleText != "KELLOGG'S FROSTED FLAKES" {
		t.Errorf("visibleText = %q, want the on-screen text recorded verbatim", got.visibleText)
	}
}

// ⚠ THE load-bearing grounding test: a brand the model returns but that is NOT in the visibleText
// is an INFERENCE (it "feels like a Coke ad"), and it is DROPPED — the era rule applied to pixels.
// This goes RED if groundVisionTags stops checking the brand against the visible text.
func TestVisionStage_DropsBrandNotInVisibleText(t *testing.T) {
	st := newFakeVisionStore()
	// visibleText spells out something else entirely — "Coca-Cola" appears nowhere the model claims
	// to have read, so it is a guess and must not persist.
	prov := &scriptedProvider{answer: `{"visibleText":"ENJOY THE HOLIDAYS","brand":"Coca-Cola","era":0,"category":""}`}
	if !runVision(t, newVisionStage(st, &scriptedVision{frames: oneFrame}, prov), wordless(visionClip("silent.mp4"))) {
		t.Fatal("the clip did not apply")
	}
	got := st.tags["silent.mp4"]
	if got.brand != "" {
		t.Errorf("brand = %q, want it DROPPED — it is not in the visible text, so it is inferred", got.brand)
	}
	// The clip is still STAMPED (the visibleText was recorded and vision_tagged set), just with no
	// grounded tag — a read-only outcome, so a re-run does not pay for the frames again.
	if len(st.setPaths) != 1 {
		t.Errorf("stamped %d times, want 1 — a read-only outcome is still recorded", len(st.setPaths))
	}
	if got.visibleText != "ENJOY THE HOLIDAYS" {
		t.Errorf("visibleText = %q, want the read text recorded even when nothing grounds", got.visibleText)
	}
}

// ⚠ THE live-found robustness case (§10 V44): llava:7b wraps its JSON answer in a ```json fence, so
// a raw json.Unmarshal failed on the leading backtick and every clip came back "not JSON". The rung
// unwraps the fence (llm.ExtractJSONObject) before parsing, so a fenced-but-valid answer grounds.
// Found running the real vision job against the dev catalog; goes RED if the unwrap is removed.
func TestVisionStage_ParsesFencedJSON(t *testing.T) {
	st := newFakeVisionStore()
	// The model fenced its answer exactly as llava does in dev. "Ford" IS in the visibleText so it
	// grounds; "cars" is NOT (a car ad rarely prints the word "cars"), so category is correctly
	// dropped by the same grounding rule — this test is about the FENCE unwrap, and the grounding
	// staying intact through it is the point.
	prov := &scriptedProvider{answer: "```json\n{\"visibleText\":\"FORD MUSTANG\",\"brand\":\"Ford\",\"era\":0,\"category\":\"cars\"}\n```"}
	if !runVision(t, newVisionStage(st, &scriptedVision{frames: oneFrame}, prov), wordless(visionClip("silent.mp4"))) {
		t.Fatal("the clip did not apply")
	}
	got := st.tags["silent.mp4"]
	if got.brand != "Ford" {
		t.Errorf("brand = %q, want Ford grounded from the fenced answer (the fence must be unwrapped)", got.brand)
	}
	if got.visibleText != "FORD MUSTANG" {
		t.Errorf("visibleText = %q, want it recorded from inside the fence", got.visibleText)
	}
	if got.category != "" {
		t.Errorf("category = %q, want it DROPPED — 'cars' is not in the visible text, so it stays ungrounded even through the fence", got.category)
	}
}

// ⚠ THE tier-is-wired test: the free frame-heuristic tier must actually be INVOKED by the rung, not
// merely exist. A monochrome 4:3 keyframe + a model that grounded NO era must produce a
// SuggestedEra of 1960 ("pre-1970s?"). Without the AnalyzeFrames/SuggestedEraFrom call in Run this
// goes RED — which is the point, because a tier that is built and tested but never called is a dead
// capability (the frames are decoded here for real, unlike oneFrame elsewhere).
func TestVisionStage_FrameHintSeedsSuggestedEraWhenModelGroundsNoEra(t *testing.T) {
	st := newFakeVisionStore()
	// A REAL black-and-white 4:3 JPEG — grayF is the same helper the framehints tests use.
	tools := &scriptedVision{frames: [][]byte{grayF(t, 128)}}
	// The model read nothing datable off the frame — no visible year, era 0. The pixels are the only
	// era signal left, and B&W + 4:3 is the one combination the heuristic fires on.
	prov := &scriptedProvider{answer: `{"visibleText":"","brand":"","era":0,"category":""}`}
	if !runVision(t, newVisionStage(st, tools, prov), wordless(visionClip("silent.mp4"))) {
		t.Fatal("the clip did not apply")
	}
	got := st.tags["silent.mp4"]
	if got.suggestedEra != 1960 {
		t.Errorf("suggestedEra = %d, want 1960 — a B&W 4:3 frame with no model-grounded era must seed the pre-1970s hint through the rung", got.suggestedEra)
	}
	if got.era != 0 {
		t.Errorf("era = %d, want 0 — the frame heuristic is a SUGGESTION, never a grounded era", got.era)
	}
}

// A model that DID ground a year off the frame suppresses the heuristic — a known era has no
// question to ask, so the frame hint must not also fire.
func TestVisionStage_FrameHintSuppressedByGroundedEra(t *testing.T) {
	st := newFakeVisionStore()
	tools := &scriptedVision{frames: [][]byte{grayF(t, 128)}} // B&W 4:3, would hint pre-1970s alone
	// The model read "©1985" off the frame and it is in the visibleText, so the era grounds.
	prov := &scriptedProvider{answer: `{"visibleText":"NINTENDO ©1985","brand":"Nintendo","era":1985,"category":"games"}`}
	if !runVision(t, newVisionStage(st, tools, prov), wordless(visionClip("silent.mp4"))) {
		t.Fatal("the clip did not apply")
	}
	got := st.tags["silent.mp4"]
	if got.era != 1985 {
		t.Errorf("era = %d, want 1985 grounded off the visible year", got.era)
	}
	if got.suggestedEra != 0 {
		t.Errorf("suggestedEra = %d, want 0 — a grounded era suppresses the frame hint", got.suggestedEra)
	}
}

// Grounding is case-insensitive (a logo has a case a year does not): "FORD" on the car grounds a
// model that answered "Ford".
func TestVisionStage_BrandGroundingIsCaseInsensitive(t *testing.T) {
	st := newFakeVisionStore()
	prov := &scriptedProvider{answer: `{"visibleText":"the new FORD mustang","brand":"Ford","era":0,"category":""}`}
	if !runVision(t, newVisionStage(st, &scriptedVision{frames: oneFrame}, prov), wordless(visionClip("silent.mp4"))) {
		t.Fatal("the clip did not apply")
	}
	if got := st.tags["silent.mp4"]; got.brand != "Ford" {
		t.Errorf("brand = %q, want %q grounded case-insensitively against %q", got.brand, "Ford", "the new FORD mustang")
	}
}

// An era grounds ONLY when the year is literally in the visible text — a "1987" the model read off
// a corner grounds it; a year it inferred from the film stock does not.
func TestVisionStage_GroundsEraOnlyWhenYearIsVisible(t *testing.T) {
	stDated := newFakeVisionStore()
	provDated := &scriptedProvider{answer: `{"visibleText":"available now - (c) 1987","brand":"","era":1987,"category":""}`}
	runVision(t, newVisionStage(stDated, &scriptedVision{frames: oneFrame}, provDated), wordless(visionClip("dated.mp4")))
	if got := stDated.tags["dated.mp4"]; got.era != 1987 {
		t.Errorf("era = %d, want 1987 grounded off the visible year", got.era)
	}

	stUndated := newFakeVisionStore()
	provUndated := &scriptedProvider{answer: `{"visibleText":"the future is here","brand":"","era":1987,"category":""}`}
	runVision(t, newVisionStage(stUndated, &scriptedVision{frames: oneFrame}, provUndated), wordless(visionClip("undated.mp4")))
	if got := stUndated.tags["undated.mp4"]; got.era != 0 {
		t.Errorf("era = %d, want it DROPPED — 1987 is not in the visible text, so it is inferred", got.era)
	}
}

// An out-of-enum category is dropped exactly as validateTags drops one (the enum check survives the
// grounding generalisation).
func TestVisionStage_DropsOutOfEnumCategory(t *testing.T) {
	st := newFakeVisionStore()
	prov := &scriptedProvider{answer: `{"visibleText":"insurance you can trust","brand":"","era":0,"category":"insurance"}`}
	runVision(t, newVisionStage(st, &scriptedVision{frames: oneFrame}, prov), wordless(visionClip("silent.mp4")))
	if got := st.tags["silent.mp4"]; got.category != "" {
		t.Errorf("category = %q, want it DROPPED — 'insurance' is not a known category", got.category)
	}
}

// The positive of category grounding: a KNOWN category whose word appears in the visible text is
// kept — both halves (enum + grounded) must hold.
func TestVisionStage_GroundsAKnownCategoryPresentInVisibleText(t *testing.T) {
	st := newFakeVisionStore()
	prov := &scriptedProvider{answer: `{"visibleText":"the best cereal for kids","brand":"","era":0,"category":"cereal"}`}
	runVision(t, newVisionStage(st, &scriptedVision{frames: oneFrame}, prov), wordless(visionClip("silent.mp4")))
	if got := st.tags["silent.mp4"]; got.category != "cereal" {
		t.Errorf("category = %q, want %q grounded off the visible text", got.category, "cereal")
	}
}

// ⚠ A WORDLESS clip applies — the exact spot vision exists for (silent, no transcript, still
// untagged). This is the positive of the selectivity: the tiers below vision could not tag it.
func TestVisionStage_WordlessClipIsACandidate(t *testing.T) {
	prov := &scriptedProvider{answer: `{"visibleText":"SONY","brand":"Sony","era":0,"category":"tech"}`}
	if !runVision(t, newVisionStage(newFakeVisionStore(), &scriptedVision{frames: oneFrame}, prov), wordless(visionClip("silent.mp4"))) {
		t.Fatal("the wordless clip did not apply — it is the case this tier exists for")
	}
	if prov.calls != 1 {
		t.Errorf("provider calls = %d, want 1", prov.calls)
	}
}

// ⚠ A clip a vision pass ALREADY tagged (VisionTagged true) does not apply — the stamp is what
// stops a re-run paying for the same frames. A fully-tagged clip does not apply either: the cheaper
// tiers already resolved it, so there is nothing to add and a real cost to avoid.
func TestVisionStage_SkipsClipsWithNothingToGain(t *testing.T) {
	seen := wordless(visionClip("seen.mp4"))
	seen.VisionTagged = true // a prior pass already read this one
	for _, tc := range []struct {
		name string
		clip filler.StoreClip
	}{
		{"already vision-tagged", seen},
		{"already fully tagged", fullyTagged(visionClip("done.mp4"))},
	} {
		tools := &scriptedVision{frames: oneFrame}
		prov := &scriptedProvider{answer: `{"visibleText":"anything","brand":"","era":0,"category":""}`}
		if runVision(t, newVisionStage(newFakeVisionStore(), tools, prov), tc.clip) {
			t.Errorf("%s: applied, want skipped", tc.name)
		}
		if tools.calls != 0 || prov.calls != 0 {
			t.Errorf("%s: keyframes=%d vision=%d, want it left untouched", tc.name, tools.calls, prov.calls)
		}
	}
}

// ⚠ A keyframe/model FAILURE is NOT stamped — leaving vision_tagged false is what makes a later
// pass retry a clip that was never actually read. And a clip with no video stream yields zero
// frames: a retryable failure, never a model call (there is nothing to show it).
func TestVisionStage_FailuresAreNotStamped(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tools *scriptedVision
	}{
		{"keyframes failed", &scriptedVision{err: errors.New("ffmpeg not runnable")}},
		{"no video stream", &scriptedVision{frames: nil}},
	} {
		st := newFakeVisionStore()
		prov := &scriptedProvider{answer: `{"visibleText":"x"}`}
		s := newVisionStage(st, tc.tools, prov)
		if applies, _ := s.Applies(context.Background(), wordless(visionClip("a.mp4"))); !applies {
			t.Fatalf("%s: the clip should apply; the failure is at run time", tc.name)
		}
		if _, err := s.Run(context.Background(), wordless(visionClip("a.mp4"))); err == nil {
			t.Errorf("%s: want an error so the runner retries", tc.name)
		}
		if prov.calls != 0 {
			t.Errorf("%s: provider called %d times, want 0", tc.name, prov.calls)
		}
		if len(st.setPaths) != 0 {
			t.Errorf("%s: stamped %v on a failure; leaving it unstamped is what makes a later pass retry", tc.name, st.setPaths)
		}
	}
}

// The opt-in gate's off states — nil provider (no vision model, the common case), enabled false,
// and a nil enabled closure. Each is inapplicable WITH A REASON, and none may touch ffmpeg or the
// model.
func TestVisionStage_OffStatesAreInapplicableWithAReason(t *testing.T) {
	answer := `{"visibleText":"x","brand":"","era":0,"category":""}`
	now := func() time.Time { return time.Unix(1, 0) }
	for _, tc := range []struct {
		name     string
		provider llm.VisionProvider
		enabled  func() bool
	}{
		{"nil provider", nil, func() bool { return true }},
		{"enabled false", &scriptedProvider{answer: answer}, func() bool { return false }},
		{"nil enabled", &scriptedProvider{answer: answer}, nil},
	} {
		tools := &scriptedVision{frames: oneFrame}
		s := filler.NewVisionStage(tools, tc.provider, newFakeVisionStore(), "/filler", tc.enabled, now)
		applies, note := s.Applies(context.Background(), wordless(visionClip("a.mp4")))
		if applies {
			t.Errorf("%s: applies, want not", tc.name)
		}
		if note == "" {
			t.Errorf("%s: no note — a skipped rung has to say why", tc.name)
		}
		if tools.calls != 0 {
			t.Errorf("%s: extracted keyframes %d times with the rung off", tc.name, tools.calls)
		}
		if p, ok := tc.provider.(*scriptedProvider); ok && p.calls != 0 {
			t.Errorf("%s: provider called %d times, want a no-op", tc.name, p.calls)
		}
	}
}
