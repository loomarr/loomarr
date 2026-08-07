package filler_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/taxonomy"
)

// The vision-tagging job (§10 V44) — the expensive LAST tier. Its risk is the same as the transcribe
// job's, one level up: keyframes + a multimodal call cost real money/time, so the tests assert the
// job runs ONLY where the cheaper tiers left a gap, GROUNDS every tag against the on-screen text the
// model says it read (an inferred brand is dropped), and STAMPS vision_tagged so a re-run never pays
// twice.

// fakeVisionStore records what the job wrote and stamped, without a database.
type fakeVisionStore struct {
	clips    []filler.StoreClip
	tags     map[string]visionWrite // path → recorded vision tags
	setPaths []string               // order + count of SetClipVisionTags calls
	setErr   error
}

// visionWrite is one recorded SetClipVisionTags call. vision_tagged is always true on this path (the
// store method stamps it), so we record the grounded values a re-run's candidacy turns on, plus the
// suggestedEra the frame-heuristic tier feeds through.
type visionWrite struct {
	brand, visibleText, category string
	era, suggestedEra            int
}

func newFakeVisionStore(clips ...filler.StoreClip) *fakeVisionStore {
	return &fakeVisionStore{clips: clips, tags: map[string]visionWrite{}}
}

func (f *fakeVisionStore) ListClips(context.Context, filler.ClipQuery) ([]filler.StoreClip, error) {
	return f.clips, nil
}

func (f *fakeVisionStore) SetClipVisionTags(_ context.Context, path, brand, visibleText string, era, suggestedEra int, category string, _ time.Time) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.setPaths = append(f.setPaths, path)
	f.tags[path] = visionWrite{brand: brand, visibleText: visibleText, category: category, era: era, suggestedEra: suggestedEra}
	return nil
}

// ListTaxa serves the REAL seed forest (§10 V45a): the vision tier grounds its category against it, so
// a vision-read `toys` resolves and an off-vocabulary slug is dropped — the same resolve-or-drop gate
// the text tier uses.
func (f *fakeVisionStore) ListTaxa(_ context.Context) ([]taxonomy.Taxon, error) {
	return taxonomy.SeedForest(), nil
}

// scriptedVision is a MediaTools whose only real method is Keyframes, returning a fixed set of JPEG
// stand-ins (any non-empty bytes — the job never decodes them, only forwards them to the provider)
// or an error. The rest satisfy the interface and are never called by this job.
type scriptedVision struct {
	frames [][]byte
	err    error
	calls  int
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

// oneFrame is a non-empty keyframe stand-in — the job forwards bytes, it never decodes them.
var oneFrame = [][]byte{[]byte("\xff\xd8jpeg\xff\xd9")}

// visionClip builds an untagged, not-yet-vision-tagged candidate clip.
func visionClip(path string) filler.StoreClip {
	return filler.StoreClip{Clip: filler.Clip{
		Hash: path, Path: path, Name: path, Kind: filler.Commercial, DurationMs: 30_000,
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

// fullyTagged marks a clip resolved by the cheaper tiers (era + audience + category), so it is NOT a
// vision candidate.
func fullyTagged(c filler.StoreClip) filler.StoreClip {
	c.Era = 1994
	c.Audience = filler.Kids
	c.Category = "toys"
	return c
}

// newVisionJob wires the job with a scripted store/tools/provider, always ENABLED and pointed at
// "/filler".
func newVisionJob(
	t *testing.T, st filler.VisionStore, tools filler.MediaTools, provider llm.VisionProvider,
) *filler.VisionJob {
	t.Helper()
	return filler.NewVisionJob(st, tools, provider,
		func() string { return "/filler" }, func() bool { return true },
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() },
		slog.New(slog.DiscardHandler))
}

// THE grounding property: a brand that appears in the returned visibleText is PERSISTED.
func TestVisionJob_PersistsBrandGroundedInVisibleText(t *testing.T) {
	st := newFakeVisionStore(wordless(visionClip("silent.mp4")))
	tools := &scriptedVision{frames: oneFrame}
	// The model read "KELLOGG'S FROSTED FLAKES" off the box and named the advertiser — the brand
	// appears literally in the visible text, so it grounds.
	prov := &scriptedProvider{answer: `{"visibleText":"KELLOGG'S FROSTED FLAKES","brand":"Kellogg's","era":0,"category":""}`}
	res, err := newVisionJob(t, st, tools, prov).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Grounded != 1 || res.Considered != 1 {
		t.Fatalf("grounded=%d considered=%d, want a single grounded clip", res.Grounded, res.Considered)
	}
	got := st.tags["silent.mp4"]
	if got.brand != "Kellogg's" {
		t.Errorf("brand = %q, want %q grounded off the visible text", got.brand, "Kellogg's")
	}
	if got.visibleText != "KELLOGG'S FROSTED FLAKES" {
		t.Errorf("visibleText = %q, want the on-screen text recorded verbatim", got.visibleText)
	}
}

// ⚠ THE load-bearing grounding test: a brand the model returns but that is NOT in the visibleText is
// an INFERENCE (it "feels like a Coke ad"), and it is DROPPED — the era rule applied to pixels. This
// goes RED if groundVisionTags stops checking the brand against the visible text.
func TestVisionJob_DropsBrandNotInVisibleText(t *testing.T) {
	st := newFakeVisionStore(wordless(visionClip("silent.mp4")))
	tools := &scriptedVision{frames: oneFrame}
	// visibleText spells out something else entirely — "Coca-Cola" appears nowhere the model claims
	// to have read, so it is a guess and must not persist.
	prov := &scriptedProvider{answer: `{"visibleText":"ENJOY THE HOLIDAYS","brand":"Coca-Cola","era":0,"category":""}`}
	res, err := newVisionJob(t, st, tools, prov).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := st.tags["silent.mp4"]
	if got.brand != "" {
		t.Errorf("brand = %q, want it DROPPED — it is not in the visible text, so it is inferred", got.brand)
	}
	// The clip is still STAMPED (the visibleText was recorded and vision_tagged set), just with no
	// grounded tag — a read-only outcome, so a re-run does not pay for the frames again.
	if res.ReadOnly != 1 || res.Grounded != 0 {
		t.Errorf("readOnly=%d grounded=%d, want the clip stamped with nothing grounded", res.ReadOnly, res.Grounded)
	}
	if got.visibleText != "ENJOY THE HOLIDAYS" {
		t.Errorf("visibleText = %q, want the read text recorded even when nothing grounds", got.visibleText)
	}
}

// ⚠ THE live-found robustness case (§10 V44): llava:7b wraps its JSON answer in a ```json fence, so
// a raw json.Unmarshal failed on the leading backtick and every clip came back "not JSON". The
// vision job unwraps the fence (llm.ExtractJSONObject) before parsing, so a fenced-but-valid answer
// grounds. Found running the real vision job against the dev catalog; goes RED if the unwrap is
// removed.
func TestVisionJob_ParsesFencedJSON(t *testing.T) {
	st := newFakeVisionStore(wordless(visionClip("silent.mp4")))
	tools := &scriptedVision{frames: oneFrame}
	// The model fenced its answer exactly as llava does in dev. "Ford" IS in the visibleText so it
	// grounds; "cars" is NOT (a car ad rarely prints the word "cars"), so category is correctly
	// dropped by the same grounding rule — this test is about the FENCE unwrap, and the grounding
	// staying intact through it is the point.
	prov := &scriptedProvider{answer: "```json\n{\"visibleText\":\"FORD MUSTANG\",\"brand\":\"Ford\",\"era\":0,\"category\":\"cars\"}\n```"}
	res, err := newVisionJob(t, st, tools, prov).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Grounded != 1 {
		t.Fatalf("grounded=%d, want 1 — a fenced-but-valid answer must be unwrapped and grounded, not dropped as 'not JSON'", res.Grounded)
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

// ⚠ THE tier-is-wired test: the free frame-heuristic tier must actually be INVOKED by the vision
// job, not merely exist. A monochrome 4:3 keyframe + a model that grounded NO era must produce a
// SuggestedEra of 1960 ("pre-1970s?"). Without the AnalyzeFrames/SuggestedEraFrom call in Run this
// goes RED — which is the whole point, because a tier that is built and tested but never called is a
// dead capability (the frames are decoded here for real, unlike oneFrame elsewhere).
func TestVisionJob_FrameHintSeedsSuggestedEraWhenModelGroundsNoEra(t *testing.T) {
	st := newFakeVisionStore(wordless(visionClip("silent.mp4")))
	// A REAL black-and-white 4:3 JPEG — grayF is the same helper the framehints tests use. The bytes
	// must decode, because the job now runs AnalyzeFrames over them.
	tools := &scriptedVision{frames: [][]byte{grayF(t, 128)}}
	// The model read nothing datable off the frame — no visible year, era 0. The pixels are the only
	// era signal left, and B&W + 4:3 is the one combination the heuristic fires on.
	prov := &scriptedProvider{answer: `{"visibleText":"","brand":"","era":0,"category":""}`}
	if _, err := newVisionJob(t, st, tools, prov).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := st.tags["silent.mp4"]
	if got.suggestedEra != 1960 {
		t.Errorf("suggestedEra = %d, want 1960 — a B&W 4:3 frame with no model-grounded era must seed the pre-1970s hint through the vision job", got.suggestedEra)
	}
	if got.era != 0 {
		t.Errorf("era = %d, want 0 — the frame heuristic is a SUGGESTION, never a grounded era", got.era)
	}
}

// A model that DID ground a year off the frame suppresses the heuristic — a known era has no question
// to ask, so the frame hint must not also fire.
func TestVisionJob_FrameHintSuppressedByGroundedEra(t *testing.T) {
	st := newFakeVisionStore(wordless(visionClip("silent.mp4")))
	tools := &scriptedVision{frames: [][]byte{grayF(t, 128)}} // B&W 4:3, would hint pre-1970s on its own
	// The model read "©1985" off the frame and it is in the visibleText, so the era grounds.
	prov := &scriptedProvider{answer: `{"visibleText":"NINTENDO ©1985","brand":"Nintendo","era":1985,"category":"games"}`}
	if _, err := newVisionJob(t, st, tools, prov).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := st.tags["silent.mp4"]
	if got.era != 1985 {
		t.Errorf("era = %d, want 1985 grounded off the visible year", got.era)
	}
	if got.suggestedEra != 0 {
		t.Errorf("suggestedEra = %d, want 0 — a grounded era suppresses the frame hint", got.suggestedEra)
	}
}

// Grounding is case-insensitive (a logo has a case a year does not): "KELLOGG'S" on the box grounds a
// model that answered "Kellogg's".
func TestVisionJob_BrandGroundingIsCaseInsensitive(t *testing.T) {
	st := newFakeVisionStore(wordless(visionClip("silent.mp4")))
	tools := &scriptedVision{frames: oneFrame}
	prov := &scriptedProvider{answer: `{"visibleText":"the new FORD mustang","brand":"Ford","era":0,"category":""}`}
	if _, err := newVisionJob(t, st, tools, prov).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := st.tags["silent.mp4"]
	if got.brand != "Ford" {
		t.Errorf("brand = %q, want %q grounded case-insensitively against %q", got.brand, "Ford", "the new FORD mustang")
	}
}

// An era grounds ONLY when the year is literally in the visible text — a "1987" the model read off a
// corner grounds it; a year it inferred from the film stock does not.
func TestVisionJob_GroundsEraOnlyWhenYearIsVisible(t *testing.T) {
	// The scripted provider answers the same JSON for every clip in one Run, so the "year visible"
	// and "year absent" cases run as two separate jobs with distinct stores.
	stDated := newFakeVisionStore(wordless(visionClip("dated.mp4")))
	provDated := &scriptedProvider{answer: `{"visibleText":"available now - (c) 1987","brand":"","era":1987,"category":""}`}
	if _, err := newVisionJob(t, stDated, &scriptedVision{frames: oneFrame}, provDated).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := stDated.tags["dated.mp4"]; got.era != 1987 {
		t.Errorf("era = %d, want 1987 grounded off the visible year", got.era)
	}

	stUndated := newFakeVisionStore(wordless(visionClip("undated.mp4")))
	provUndated := &scriptedProvider{answer: `{"visibleText":"the future is here","brand":"","era":1987,"category":""}`}
	if _, err := newVisionJob(t, stUndated, &scriptedVision{frames: oneFrame}, provUndated).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := stUndated.tags["undated.mp4"]; got.era != 0 {
		t.Errorf("era = %d, want it DROPPED — 1987 is not in the visible text, so it is inferred", got.era)
	}
}

// An out-of-enum category is dropped exactly as validateTags drops one (the enum check survives the
// grounding generalisation).
func TestVisionJob_DropsOutOfEnumCategory(t *testing.T) {
	st := newFakeVisionStore(wordless(visionClip("silent.mp4")))
	tools := &scriptedVision{frames: oneFrame}
	prov := &scriptedProvider{answer: `{"visibleText":"insurance you can trust","brand":"","era":0,"category":"insurance"}`}
	if _, err := newVisionJob(t, st, tools, prov).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := st.tags["silent.mp4"]; got.category != "" {
		t.Errorf("category = %q, want it DROPPED — 'insurance' is not a known category", got.category)
	}
}

// The positive of category grounding: a KNOWN category whose word appears in the visible text is
// kept — both halves (enum + grounded) must hold, and this exercises the kept path the drop test
// above only exercises the reject of.
func TestVisionJob_GroundsAKnownCategoryPresentInVisibleText(t *testing.T) {
	st := newFakeVisionStore(wordless(visionClip("silent.mp4")))
	tools := &scriptedVision{frames: oneFrame}
	prov := &scriptedProvider{answer: `{"visibleText":"the best cereal for kids","brand":"","era":0,"category":"cereal"}`}
	if _, err := newVisionJob(t, st, tools, prov).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := st.tags["silent.mp4"]; got.category != "cereal" {
		t.Errorf("category = %q, want %q grounded off the visible text", got.category, "cereal")
	}
}

// ⚠ A WORDLESS clip is a candidate — the exact spot vision exists for (silent, no transcript, still
// untagged). This is the positive of the selectivity: the tiers below vision could not tag it.
func TestVisionJob_WordlessClipIsACandidate(t *testing.T) {
	st := newFakeVisionStore(wordless(visionClip("silent.mp4")))
	tools := &scriptedVision{frames: oneFrame}
	prov := &scriptedProvider{answer: `{"visibleText":"SONY","brand":"Sony","era":0,"category":"tech"}`}
	res, err := newVisionJob(t, st, tools, prov).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if prov.calls != 1 || res.Considered != 1 {
		t.Errorf("provider calls=%d considered=%d, want the wordless clip looked at", prov.calls, res.Considered)
	}
}

// ⚠ A clip a vision pass ALREADY tagged (VisionTagged true) is NOT re-processed — the stamp is what
// stops a re-run paying for the same frames. This goes RED if isCandidate stops checking VisionTagged.
func TestVisionJob_VisionTaggedPreventsReprocessing(t *testing.T) {
	clip := wordless(visionClip("seen.mp4"))
	clip.VisionTagged = true // a prior pass already read this one
	st := newFakeVisionStore(clip)
	tools := &scriptedVision{frames: oneFrame}
	prov := &scriptedProvider{answer: `{"visibleText":"anything","brand":"","era":0,"category":""}`}
	res, err := newVisionJob(t, st, tools, prov).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tools.calls != 0 || prov.calls != 0 {
		t.Errorf("keyframes=%d vision=%d, want a vision-tagged clip left untouched", tools.calls, prov.calls)
	}
	if res.Considered != 0 {
		t.Errorf("considered = %d, want 0 — the clip was already vision-tagged", res.Considered)
	}
}

// A clip the cheaper tiers already fully tagged has nothing for vision to add and a cost to avoid — it
// is NOT a candidate even though no vision pass has looked.
func TestVisionJob_SkipsAFullyTaggedClip(t *testing.T) {
	st := newFakeVisionStore(fullyTagged(visionClip("done.mp4")))
	tools := &scriptedVision{frames: oneFrame}
	prov := &scriptedProvider{answer: `{"visibleText":"x","brand":"","era":0,"category":""}`}
	res, err := newVisionJob(t, st, tools, prov).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tools.calls != 0 || prov.calls != 0 || res.Considered != 0 {
		t.Errorf("keyframes=%d vision=%d considered=%d, want a fully-tagged clip skipped without cost",
			tools.calls, prov.calls, res.Considered)
	}
}

// ⚠ A keyframe/model FAILURE is NOT stamped — leaving vision_tagged false is what makes the next pass
// retry a clip that was never actually read.
func TestVisionJob_AFailureIsNotStamped(t *testing.T) {
	st := newFakeVisionStore(wordless(visionClip("a.mp4")))
	tools := &scriptedVision{err: errors.New("ffmpeg not runnable")}
	prov := &scriptedProvider{answer: `{"visibleText":"x"}`}
	res, err := newVisionJob(t, st, tools, prov).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 {
		t.Errorf("failed = %d, want 1", res.Failed)
	}
	if prov.calls != 0 {
		t.Errorf("provider called %d times after a keyframe failure, want 0", prov.calls)
	}
	if len(st.setPaths) != 0 {
		t.Errorf("stamped %v on a failure; leaving it unstamped is what makes the next pass retry", st.setPaths)
	}
}

// A clip with no video stream yields zero frames — treated as a retryable failure, never asked about
// (there is nothing to show the model), and never stamped.
func TestVisionJob_NoFramesIsAFailureNotAModelCall(t *testing.T) {
	st := newFakeVisionStore(wordless(visionClip("audioonly.mp4")))
	tools := &scriptedVision{frames: nil} // no video stream
	prov := &scriptedProvider{answer: `{"visibleText":"x"}`}
	res, err := newVisionJob(t, st, tools, prov).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if prov.calls != 0 {
		t.Errorf("provider called %d times with no frames, want 0", prov.calls)
	}
	if res.Failed != 1 || len(st.setPaths) != 0 {
		t.Errorf("failed=%d stamped=%v, want a retryable un-stamped failure", res.Failed, st.setPaths)
	}
}

// The opt-in gate has FOUR off states, each a complete no-op: nil provider (no vision model — the
// common case), enabled false, nil enabled, and an empty dir. None may touch ffmpeg or the model.
func TestVisionJob_OffStatesAreNoOps(t *testing.T) {
	clip := wordless(visionClip("a.mp4"))
	answer := `{"visibleText":"x","brand":"","era":0,"category":""}`
	now := func() time.Time { return time.Unix(1, 0) }
	log := slog.New(slog.DiscardHandler)

	cases := []struct {
		name     string
		provider llm.VisionProvider
		enabled  func() bool
		dir      func() string
	}{
		{"nil provider", nil, func() bool { return true }, func() string { return "/filler" }},
		{"enabled false", &scriptedProvider{answer: answer}, func() bool { return false }, func() string { return "/filler" }},
		{"nil enabled", &scriptedProvider{answer: answer}, nil, func() string { return "/filler" }},
		{"empty dir", &scriptedProvider{answer: answer}, func() bool { return true }, func() string { return "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newFakeVisionStore(clip)
			tools := &scriptedVision{frames: oneFrame}
			job := filler.NewVisionJob(st, tools, tc.provider, tc.dir, tc.enabled, now, log)
			res, err := job.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if tools.calls != 0 || res.Considered != 0 {
				t.Errorf("keyframes=%d considered=%d, want a complete no-op", tools.calls, res.Considered)
			}
			if p, ok := tc.provider.(*scriptedProvider); ok && p.calls != 0 {
				t.Errorf("provider called %d times, want a no-op", p.calls)
			}
		})
	}
}

// ⚠ One pass is BOUNDED to VisionBatch — the most expensive tier must not run over a whole fresh
// catalog in one go (a bill on the hosted path, hours of decoding on the local one).
func TestVisionJob_BoundsOnePass(t *testing.T) {
	var clips []filler.StoreClip
	for i := 0; i < filler.VisionBatch+3; i++ {
		clips = append(clips, wordless(visionClip(string(rune('a'+i))+".mp4")))
	}
	st := newFakeVisionStore(clips...)
	tools := &scriptedVision{frames: oneFrame}
	prov := &scriptedProvider{answer: `{"visibleText":"x","brand":"","era":0,"category":""}`}
	res, err := newVisionJob(t, st, tools, prov).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Considered != filler.VisionBatch {
		t.Errorf("considered = %d, want the batch size %d", res.Considered, filler.VisionBatch)
	}
	if prov.calls > filler.VisionBatch {
		t.Errorf("vision ran %d times, past the batch bound %d", prov.calls, filler.VisionBatch)
	}
}
