package filler_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/taxonomy"
	"github.com/mantonx/loomarr/internal/testkit"
)

// errTagClipNotFound stands in for store.ErrNotFound, which this package cannot import (`filler`
// deliberately does not depend on `store`). What matters to the tagger is only that a miss is an
// ERROR — it returns fatally on one, so a mock that silently succeeded on a wrong key hid the bug.
var errTagClipNotFound = errors.New("clip not found")

// tagMemStore is an in-memory TagStore.
type tagMemStore struct {
	clips   map[string]filler.StoreClip
	updates map[string]filler.StoreClip
	// filed records the paths SetClipsHeld filed, in call order — the auto-file assertions
	// read this rather than inspecting clips, so a test says "this was filed" rather than
	// "this ended up not-held", which a bug could also produce.
	filed []string
	// brands records SetClipBrand writes keyed by PATH (the key the real store uses), so a test can
	// assert a GROUNDED brand was written — and, by its absence, that an ungrounded one never was.
	brands map[string]string
	// confidence records SetClipConfidence writes keyed by PATH (the key the real store uses).
	// ⚠ Kept as a map rather than folded into `clips`, so a test asserts that the score was
	// WRITTEN rather than that it happens to be non-zero — the distinction that mattered, since
	// `Score` produced a correct number for two phases while nothing persisted it (§10 V51a).
	confidence map[string]int
	// tags records SetClipTags writes keyed by HASH (§10 V45a) — the LEAVES the tagger persisted. A
	// test asserts the grounded tag set here; hash-keyed like UpdateClipTags (and unlike the
	// path-keyed brand map), so it MISSES a caller that passed a path, the same discipline the rest of
	// this double keeps.
	tags map[string][]string
}

func newTagMemStore() *tagMemStore {
	return &tagMemStore{
		clips:      map[string]filler.StoreClip{},
		updates:    map[string]filler.StoreClip{},
		brands:     map[string]string{},
		tags:       map[string][]string{},
		confidence: map[string]int{},
	}
}

// seedForest is the grounding graph a direct Classify test grounds against (§10 V45a) — the same
// shipped vocabulary the tagger's store serves via ListTaxa, so a direct call and a full Run() ground
// identically. `beer` resolves, `nonsense_widgets` is dropped.
func seedForest() *taxonomy.Forest { return taxonomy.New(taxonomy.SeedForest()) }

// seed adds a clip keyed by its own identity. ⚠ Use this rather than assigning into `clips`
// directly: the map key IS the hash the store looks up, so seeding `clips["c1"]` with a clip
// whose Hash is something else builds a row that cannot be found by either key. That was
// harmless while every fixture set hash == path and became four failures the moment they differed.
func (m *tagMemStore) seed(c filler.StoreClip) { m.clips[c.Hash] = c }

func (m *tagMemStore) ListUntaggedCommercials(_ context.Context) ([]filler.StoreClip, error) {
	var out []filler.StoreClip
	for _, c := range m.clips {
		out = append(out, c)
	}
	return out, nil
}

// ⚠ SetClipsHeld is keyed by PATH and UpdateClipTags by HASH, mirroring the real store exactly
// (`WHERE path IN (…)` vs `WHERE hash = ?`). A mock that accepted either — which this one did
// until V41, because every fixture set hash == path — cannot catch a caller passing the wrong
// key, and that is precisely the bug that shipped. Both methods now MISS loudly on a wrong key
// rather than quietly succeeding.
func (m *tagMemStore) SetClipsHeld(_ context.Context, paths []string, held, autoFiled bool, _ time.Time) (int, error) {
	n := 0
	for _, p := range paths {
		hash, ok := m.hashByPath(p)
		if !ok {
			continue // the real store's `WHERE path IN (…)` matches no row either
		}
		c := m.clips[hash]
		c.Held = held
		c.AutoFiled = autoFiled
		m.clips[hash] = c
		n++
		if !held {
			m.filed = append(m.filed, p)
		}
	}
	return n, nil
}

// SetClipBrand is keyed by PATH (the real store is `WHERE path = ?`), like SetClipsHeld and unlike
// the hash-keyed UpdateClipTags — so it MISSES on a wrong key rather than quietly succeeding, which
// is what would let a caller passing the hash slip through. It writes only `brand`, never
// `VisionTagged`: a text-grounded brand is not a vision-read one.
func (m *tagMemStore) SetClipBrand(_ context.Context, path, brand string, _ time.Time) error {
	hash, ok := m.hashByPath(path)
	if !ok {
		return errTagClipNotFound
	}
	m.brands[path] = brand
	c := m.clips[hash]
	c.Brand = brand
	m.clips[hash] = c
	// Reflect into `updates` too, so an assertion reading the tag-write map (where the era/audience
	// merge lands) sees the brand a run grounded in the same loop iteration.
	if u, ok := m.updates[hash]; ok {
		u.Brand = brand
		m.updates[hash] = u
	}
	return nil
}

// SetClipConfidence records the grounding-capped score, PATH-keyed like the real store's writer.
func (m *tagMemStore) SetClipConfidence(_ context.Context, path string, confidence int, _ time.Time) error {
	hash, ok := m.hashByPath(path)
	if !ok {
		return errTagClipNotFound
	}
	m.confidence[path] = confidence
	c := m.clips[hash]
	c.Confidence = confidence
	m.clips[hash] = c
	return nil
}

// hashByPath resolves a clip's location to its identity, the lookup the real store does in SQL.
func (m *tagMemStore) hashByPath(path string) (string, bool) {
	for h, c := range m.clips {
		if c.Path == path {
			return h, true
		}
	}
	return "", false
}

func (m *tagMemStore) UpdateClipTags(_ context.Context, id string, era int, audience, category string, suggestedEra int, aiTagged bool, _ time.Time) error {
	c, ok := m.clips[id]
	if !ok {
		// The real store returns store.ErrNotFound here, and the tagger treats it as FATAL —
		// which is how "the tagger passed a path" became "no clip was ever tagged". A local
		// sentinel because `filler` must not import `store` (the layering the package relies on).
		return errTagClipNotFound
	}
	c.Era = era
	c.Audience = filler.Audience(audience)
	c.Category = category
	c.AITagged = aiTagged
	if era > 0 {
		c.SuggestedEra = 0 // confirming clears (the store's conditional write)
	} else if suggestedEra > 0 {
		c.SuggestedEra = suggestedEra
	}
	m.updates[id] = c
	return nil
}

// ListTaxa serves the REAL seed forest (§10 V45a) — the tagger grounds against it, so a test that
// served an empty graph would ground nothing and prove nothing. Grounding the double in the shipped
// vocabulary is what lets a test assert that `beer` resolves and `nonsense` is dropped.
func (m *tagMemStore) ListTaxa(_ context.Context) ([]taxonomy.Taxon, error) {
	return taxonomy.SeedForest(), nil
}

// GetClipTags returns the LEAVES a clip already has — HASH-keyed like UpdateClipTags, so the union
// the tagger does reads back exactly what it wrote. leavesOnly is honoured trivially: the double
// stores only leaves (rollup expansion is the real store's job, not modelled here).
func (m *tagMemStore) GetClipTags(_ context.Context, clipHash string, _ bool) ([]string, error) {
	return m.tags[clipHash], nil
}

// SetClipTags records the persisted leaf set, HASH-keyed. Like UpdateClipTags it MISSES loudly on a
// wrong key — a caller that passed a path finds no clip, the same not-quietly-succeed discipline the
// rest of this double keeps ([[loomarr-fixture-collapsed-keys]]).
func (m *tagMemStore) SetClipTags(_ context.Context, clipHash string, leaves []string, _ *taxonomy.Forest, _ time.Time) error {
	if _, ok := m.clips[clipHash]; !ok {
		return errTagClipNotFound
	}
	m.tags[clipHash] = leaves
	return nil
}

// untaggedClip builds a catalog clip. ⚠ Since V38c the sidecar is found by PATH, so a fixture
// giving a clip a sidecar must name it `<path minus extension>.info.json` — the shape intake
// produces. Fuzzy name matching is gone; see TestTagger_ReadsTheSidecarBesideTheClip.
//
// ⚠ Hash and Path are DELIBERATELY different. They were both `id` until V41, which made this
// mock unable to tell a hash-keyed call from a path-keyed one — so the tagger passing `clip.Path`
// into the hash-keyed `UpdateClipTags` passed every test here while failing on every real clip
// in production. `id` is the identity; the path is derived from it.
func untaggedClip(id, name string) filler.StoreClip {
	c := filler.StoreClip{}
	c.Hash = id
	c.Path = "p/" + id + ".mp4"
	c.Name = name
	c.Kind = filler.Commercial
	return c
}

// A well-formed classification is written with all three tags. The era year
// appears in the filename, so it is GROUNDED (§10 V34) and lands as a tag.
func TestTagger_WritesValidClassification(t *testing.T) {
	st := newTagMemStore()
	st.clips["c1"] = untaggedClip("c1", "Frosted Flakes ad 1992")
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":1992,"audience":"kids","tags":["cereal"]}`),
	)
	tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog())

	res, err := tagger.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Tagged != 1 {
		t.Fatalf("want 1 fully tagged, got %+v", res)
	}
	got := st.updates["c1"]
	if got.Era != 1992 || got.Audience != filler.Kids || got.Category != "cereal" || !got.AITagged {
		t.Errorf("classification not written correctly: %+v", got.Clip)
	}
}

// GROUNDING (§10): the model returns a HALLUCINATED audience + category (not in
// the enum sets). Those fields are DROPPED — never persisted as garbage. The valid
// field (era, its year in the filename) still lands.
func TestTagger_DropsHallucinatedEnums(t *testing.T) {
	st := newTagMemStore()
	st.clips["c1"] = untaggedClip("c1", "some ad 1993")
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":1993,"audience":"cyberpunk","tags":["nonsense_widgets"]}`),
	)
	tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog())

	res, err := tagger.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := st.updates["c1"]
	if got.Audience != "" {
		t.Errorf("hallucinated audience %q was persisted — grounding breached", got.Audience)
	}
	// §10 V45a: the off-vocabulary tag is DROPPED by the resolve-or-drop gate, so it reaches neither
	// the persisted leaf set nor the derived product-leaf shadow (`Category`). Both are proven empty —
	// the tag set is the primary grounding assertion now, `Category` its shadow.
	if len(st.tags["c1"]) != 0 {
		t.Errorf("hallucinated tag %v was persisted — grounding breached", st.tags["c1"])
	}
	if got.Category != "" {
		t.Errorf("hallucinated category %q was persisted — grounding breached", got.Category)
	}
	// The valid era survived → this is a partial tag.
	if got.Era != 1993 {
		t.Errorf("valid era dropped: %d", got.Era)
	}
	if res.Partial != 1 {
		t.Errorf("partial classification should count as partial, got %+v", res)
	}
}

// An implausible year is rejected too.
func TestTagger_RejectsBadYear(t *testing.T) {
	st := newTagMemStore()
	st.clips["c1"] = untaggedClip("c1", "ad")
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":9999,"audience":"kids","tags":["toys"]}`),
	)
	tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog())
	_, _ = tagger.Run(context.Background())
	if st.updates["c1"].Era != 0 {
		t.Errorf("implausible year 9999 was persisted: %d", st.updates["c1"].Era)
	}
}

// Classify directly: enum validation, with the era year present in the text.
func TestClassify_ValidatesEnums(t *testing.T) {
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":1994,"audience":"late_night","tags":["cars"]}`),
	)
	sug, err := filler.Classify(context.Background(), llmMock, seedForest(), "car ad 1994", "")
	if err != nil {
		t.Fatal(err)
	}
	if !sug.Complete() || sug.Era != 1994 || sug.Audience != filler.LateNight || sug.Category != "cars" {
		t.Errorf("valid classification mis-parsed: %+v", sug)
	}
	if sug.SuggestedEra != 0 {
		t.Errorf("grounded era leaked into SuggestedEra: %+v", sug)
	}
}

// --- Era grounding (§10, V34) ----------------------------------------------------
//
// Measured on real transcripts (plan §6.4): the model inferred a decade from TONE
// on 2 of 10 clips (era 1980, no year anywhere in the text), and the old
// validator persisted both as fact. The rule: an era is a tag only when the year
// appears LITERALLY in the text signals; otherwise it is a SUGGESTION.

// The measured defect, end to end: a plausible era with no year anywhere in the
// text is NOT persisted as a tag — it comes back as a suggestion, and the clip
// keeps era 0 so matching never sees it.
func TestClassify_UngroundedEraBecomesSuggestion(t *testing.T) {
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":1980,"audience":"general","tags":["tech"]}`),
	)
	// Ad copy full of period-sounding language, but no year — the §6.4 failure shape.
	sug, err := filler.Classify(context.Background(), llmMock, seedForest(), "aqua-globes", "Transcript: the amazing new Aqua Globes water your plants for weeks...")
	if err != nil {
		t.Fatal(err)
	}
	if sug.Era != 0 {
		t.Errorf("ungrounded era persisted as a tag — §8 breach: %+v", sug)
	}
	if sug.SuggestedEra != 1980 {
		t.Errorf("ungrounded era lost instead of suggested: %+v", sug)
	}
	// The other fields still land — the suggestion demotes era ONLY.
	if sug.Audience != filler.General || sug.Category != "tech" {
		t.Errorf("valid fields dropped with the era: %+v", sug)
	}
}

// The year appearing in the SIDECAR/TRANSCRIPT text grounds the era just as the
// filename does.
func TestClassify_EraGroundedBySourceText(t *testing.T) {
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":1978,"audience":"family","tags":["cereal"]}`),
	)
	sug, err := filler.Classify(context.Background(), llmMock, seedForest(), "rice-krispies-treats", "Transcript: making treats since 1978, so easy...")
	if err != nil {
		t.Fatal(err)
	}
	if sug.Era != 1978 || sug.SuggestedEra != 0 {
		t.Errorf("era grounded in source text mis-handled: %+v", sug)
	}
}

// The tag job WRITES the suggestion (not the era) — and still counts the clip:
// "the model guessed" is information the operator should see, not silence.
func TestTagger_PersistsSuggestionNotEra(t *testing.T) {
	st := newTagMemStore()
	st.clips["c1"] = untaggedClip("c1", "swiffer")
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":1999,"audience":"general","tags":["tech"]}`),
	)
	tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog())

	res, err := tagger.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := st.updates["c1"]
	if got.Era != 0 {
		t.Errorf("ungrounded era written to the clip: %+v", got.Clip)
	}
	if got.SuggestedEra != 1999 {
		t.Errorf("suggestion not recorded: %+v", got.Clip)
	}
	if res.Skipped != 0 {
		t.Errorf("a clip carrying a suggestion is not 'nothing usable': %+v", res)
	}
}

// A clip that already HAS an era gets no suggestion — a known year needs no
// second guess riding beside it.
func TestTagger_NoSuggestionWhenEraKnown(t *testing.T) {
	st := newTagMemStore()
	clip := untaggedClip("c1", "known")
	clip.Era = 1985
	st.clips["c1"] = clip
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":1999,"audience":"kids","tags":["toys"]}`),
	)
	tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog())

	if _, err := tagger.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := st.updates["c1"]
	if got.Era != 1985 || got.SuggestedEra != 0 {
		t.Errorf("known era disturbed by an ungrounded guess: era=%d suggestedEra=%d", got.Era, got.SuggestedEra)
	}
}

// --- V44: grounded brand --------------------------------------------------------
//
// Brand follows the era rule generalised (§10 V44): the advertiser set is open, so there is no enum
// to validate against — the only defence against a fabricated brand is the text. A brand is a tag
// ONLY when it appears literally in the clip's text signals; otherwise it is dropped (no "suggested
// brand" — an ungrounded advertiser is a guess, not a question).

// The advertiser's name is literally in the source text → the brand is GROUNDED and kept.
func TestClassify_BrandGroundedInText(t *testing.T) {
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":1992,"audience":"kids","tags":["cereal"],"brand":"Kellogg's"}`),
	)
	sug, err := filler.Classify(context.Background(), llmMock, seedForest(), "cereal-ad-1992",
		"Transcript: Kellogg's Frosted Flakes, they're grrreat!")
	if err != nil {
		t.Fatal(err)
	}
	if sug.Brand != "Kellogg's" {
		t.Errorf("brand grounded in the text was dropped: %+v", sug)
	}
}

// ⚠ THE anti-fabrication property for brand (§8/§10 V44). The model asserts a brand that appears
// NOWHERE in the text — inferred from the category. It must be dropped, exactly as an ungrounded era
// is demoted. If this persisted, matching would carry a confidently-wrong advertiser.
func TestClassify_DropsUngroundedBrand(t *testing.T) {
	llmMock := testkit.NewLLM(
		// "cereal" is in the category, but no advertiser name is anywhere in the text.
		testkit.FinalResponse(`{"era":0,"audience":"kids","tags":["cereal"],"brand":"Kellogg's"}`),
	)
	sug, err := filler.Classify(context.Background(), llmMock, seedForest(), "a-cereal-ad",
		"Transcript: the crunchiest breakfast in town, part of a balanced morning")
	if err != nil {
		t.Fatal(err)
	}
	if sug.Brand != "" {
		t.Errorf("ungrounded brand %q was kept — §8 breach: a fabricated advertiser reached the tags", sug.Brand)
	}
	// The grounded fields still land — dropping the brand demotes brand ONLY.
	if sug.Audience != filler.Kids || sug.Category != "cereal" {
		t.Errorf("valid fields dropped with the brand: %+v", sug)
	}
}

// Grounding is case-INSENSITIVE (unlike era, which has no case): "KELLOGG'S" shouted on screen
// grounds a model that answered "Kellogg's", and vice-versa.
func TestClassify_BrandGroundingIsCaseInsensitive(t *testing.T) {
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":0,"audience":"kids","tags":["cereal"],"brand":"Kellogg's"}`),
	)
	sug, err := filler.Classify(context.Background(), llmMock, seedForest(), "shouty-ad",
		"Transcript: KELLOGG'S FROSTED FLAKES")
	if err != nil {
		t.Fatal(err)
	}
	if sug.Brand != "Kellogg's" {
		t.Errorf("case-different brand not grounded: %+v", sug)
	}
}

// ⚠ THE live-found case (§10 V44): the model answers "Campbell's" (apostrophe) but the archive.org
// identifier is "CampbellsSoupAdvert" (none). A raw substring check dropped this CORRECT brand; the
// punctuation-folded check grounds it. Found running the real tagger against the dev catalog — no
// unit fixture hit it because they all spelled the brand and the text identically.
func TestClassify_BrandGroundsAcrossPunctuation(t *testing.T) {
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":0,"audience":"family","tags":["fast_food"],"brand":"Campbell's"}`),
	)
	// The only text signal is the apostrophe-less filename, exactly as archive.org files it.
	sug, err := filler.Classify(context.Background(), llmMock, seedForest(), "CampbellsSoupAdvert", "")
	if err != nil {
		t.Fatal(err)
	}
	if sug.Brand != "Campbell's" {
		t.Errorf("brand = %q, want %q grounded across the apostrophe — a real filename strips it", sug.Brand, "Campbell's")
	}
}

// ⚠ Folding must NOT admit fabrication: a brand whose letters are absent still fails, so the
// anti-§8 guarantee is intact. "Coca-Cola" cannot fold into a Campbell's filename.
func TestClassify_FoldingStillDropsAbsentBrand(t *testing.T) {
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":0,"audience":"family","tags":["fast_food"],"brand":"Coca-Cola"}`),
	)
	sug, err := filler.Classify(context.Background(), llmMock, seedForest(), "CampbellsSoupAdvert", "")
	if err != nil {
		t.Fatal(err)
	}
	if sug.Brand != "" {
		t.Errorf("brand = %q, want it DROPPED — Coca-Cola is nowhere in the text, folding must not invent it", sug.Brand)
	}
}

// ⚠ Brand must NOT raise the grounded confidence ceiling (§10 V44). A clip with a grounded brand but
// a dropped audience is still PARTIAL, and its score is the partial ceiling — a brand riding along
// cannot buy a clip past the auto-file gate. (Belt-and-braces to the Score asymmetry: brand isn't in
// Score at all, so a grounded brand and no brand must score identically for the same other fields.)
func TestClassify_BrandDoesNotRaiseConfidence(t *testing.T) {
	withBrand := filler.TagSuggestion{Era: 1985, Audience: "kids", Category: "toys", Brand: "Lego"}
	without := filler.TagSuggestion{Era: 1985, Audience: "kids", Category: "toys"}
	if withBrand.Score(0) != without.Score(0) {
		t.Errorf("a grounded brand changed the score (%d vs %d) — brand must ride the existing ceiling",
			withBrand.Score(0), without.Score(0))
	}
	// A partial clip stays partial even with a grounded brand.
	partial := filler.TagSuggestion{Era: 1985, Audience: "", Category: "", Brand: "Lego"}
	if partial.Score(0) >= filler.MaxAutoFileConfidence {
		t.Errorf("a grounded brand lifted a PARTIAL clip's score to %d — it must not clear the gate", partial.Score(0))
	}
}

// End to end: a brand grounded ONLY in the persisted TRANSCRIPT (empty sidecar, no year, no
// advertiser in the filename) is written to the clip via the path-keyed SetClipBrand. This is the
// V44 case that motivates persisting transcripts — the archive.org wordless-description clip that
// still SAYS its advertiser.
func TestTagger_PersistsBrandGroundedInTranscript(t *testing.T) {
	st := newTagMemStore()
	clip := untaggedClip("c1", "clip-0001") // filename carries no brand
	clip.Transcript = "Have a Coke and a smile — the real thing."
	st.seed(clip)

	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":0,"audience":"general","tags":["general"],"brand":"Coke"}`),
	)
	tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog())

	if _, err := tagger.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Written by PATH (SetClipBrand is `WHERE path = ?`), which is what the mock records.
	if got := st.brands[clip.Path]; got != "Coke" {
		t.Errorf("brand grounded in the transcript was not persisted (brands[%q] = %q)", clip.Path, got)
	}
	if st.clips["c1"].Brand != "Coke" {
		t.Errorf("clip brand = %q, want Coke", st.clips["c1"].Brand)
	}
}

// The transcript reaches the model as a text signal (§10 V44) — the whole reason a brand can ground
// for a description-less clip. Asserts on the PROMPT, because that is where the plumbing lives.
func TestTagger_TranscriptIsAThirdTextSignal(t *testing.T) {
	st := newTagMemStore()
	clip := untaggedClip("c1", "clip-0002")
	clip.Transcript = "Ford Mustang — built for the open road, since 1979."
	st.seed(clip)

	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1979,"audience":"general","tags":["cars"],"brand":"Ford"}`))
	tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog())

	if _, err := tagger.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(llmMock.Prompt(), "Ford Mustang") {
		t.Errorf("the persisted transcript never reached the prompt:\n%s", llmMock.Prompt())
	}
	// And it grounds both the brand and the era it literally contains.
	got := st.updates["c1"]
	if got.Era != 1979 {
		t.Errorf("era 1979 (in the transcript) not grounded: %+v", got.Clip)
	}
	if st.brands[clip.Path] != "Ford" {
		t.Errorf("brand Ford (in the transcript) not persisted: %q", st.brands[clip.Path])
	}
}

// Brand is a FILL-GAP merge, never a clear (§10 V44). A clip that already has a brand — from a
// vision pass or an earlier run — must keep it when a text run reads none.
func TestTagger_DoesNotClearASetBrand(t *testing.T) {
	st := newTagMemStore()
	clip := untaggedClip("c1", "some-ad-1990")
	clip.Brand = "Pepsi" // already grounded, e.g. off a frame by the vision pass
	st.seed(clip)

	// The text model returns NO brand (empty) — a text signal with no advertiser in it.
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1990,"audience":"general","tags":["general"],"brand":""}`))
	tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog())

	if _, err := tagger.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st.clips["c1"].Brand != "Pepsi" {
		t.Errorf("a set brand was cleared by a text run that read none: %q", st.clips["c1"].Brand)
	}
	// And SetClipBrand was NOT called — nothing new was grounded, so there is nothing to write.
	if _, wrote := st.brands[clip.Path]; wrote {
		t.Errorf("SetClipBrand was called with an unchanged brand — pointless churn on updated_at")
	}
}

// --- info-JSON sidecars (§10) ---------------------------------------------------
//
// Ingest writes a sidecar beside every downloaded clip precisely so tagging has real
// text. Nothing read them: Classify was handed `Clip.Source` — a provenance enum — so
// every prompt carried "Source description: tunarr-local". These tests assert on the
// PROMPT, because that is where the defect lived; no assertion about the model's
// output could have caught it.

func sidecarFS(files map[string]string) fstest.MapFS {
	out := fstest.MapFS{}
	for name, body := range files {
		out[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return out
}

func TestTagger_SendsSidecarTextNotProvenance(t *testing.T) {
	st := newTagMemStore()
	clip := untaggedClip("c1", "toy-ad-1994")
	clip.Path = "toy-ad-1994.mp4" // the sidecar is found beside the PATH, not the identity
	clip.Source = "tunarr-local"  // the provenance enum that used to reach the model
	st.seed(clip)

	drop := sidecarFS(map[string]string{
		"toy-ad-1994.info.json": `{
			"title": "Turbo Teen Transforming Car — TV Spot",
			"description": "Original 1994 broadcast commercial for the Turbo Teen toy line.",
			"uploader": "RetroAdVault"
		}`,
	})
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1994,"audience":"kids","tags":["toys"]}`))
	tagger := filler.NewTagger(st, llmMock, drop, func() time.Time { return time.Unix(1, 0) }, discardLog())

	if _, err := tagger.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	prompt := llmMock.Prompt()
	// The regression this test exists for: the provenance enum must not be presented
	// to the model as a description.
	if strings.Contains(prompt, "tunarr-local") {
		t.Errorf("prompt still carries the provenance enum:\n%s", prompt)
	}
	// The sidecar's real signals must be there instead.
	for _, want := range []string{"Turbo Teen", "1994 broadcast commercial", "RetroAdVault"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing sidecar signal %q:\n%s", want, prompt)
		}
	}
}

func TestTagger_FallsBackToFilenameWhenNoSidecar(t *testing.T) {
	st := newTagMemStore()
	clip := untaggedClip("c1", "mystery-clip-1990")
	clip.Source = "tunarr-local"
	st.clips["c1"] = clip

	// A drop-folder clip hand-copied by the operator: no sidecar anywhere.
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1990,"audience":"general","tags":["cars"]}`))
	tagger := filler.NewTagger(st, llmMock, sidecarFS(nil), func() time.Time { return time.Unix(1, 0) }, discardLog())

	res, err := tagger.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Tagging still succeeds — a missing sidecar degrades the signal, never the job.
	if res.Tagged != 1 {
		t.Errorf("Tagged = %d, want 1 (a missing sidecar must not fail the tag)", res.Tagged)
	}
	prompt := llmMock.Prompt()
	if !strings.Contains(prompt, "mystery-clip-1990") {
		t.Errorf("prompt lost the filename:\n%s", prompt)
	}
	// With no sidecar there is no description line at all — better than a misleading one.
	if strings.Contains(prompt, "Source description:") {
		t.Errorf("prompt has an empty description line:\n%s", prompt)
	}
}

// The sidecar is found BESIDE the clip, at its real sharded path (§10 V38c).
//
// ⚠ This replaces TestTagger_MatchesSidecarAcrossNameNormalization, which pinned a capability
// that has been deliberately removed rather than one that broke. That test walked the folder
// comparing normalized basenames, because clip identity used to be a display name Tunarr might
// have tidied. Intake now files every clip as `a3/f9/<hash>.mp4`, so there is no name to fuzzily
// match — and the walk would have matched nothing, silently costing every filed clip its sidecar
// text. Reading the path the row already carries is both simpler and correct.
func TestTagger_ReadsTheSidecarBesideTheClip(t *testing.T) {
	st := newTagMemStore()
	clip := untaggedClip("a3f9deadbeef", "Toy Ad")
	clip.Path = "a3/f9/a3f9deadbeef.mp4" // the filed shard path the sidecar sits beside
	st.seed(clip)

	drop := sidecarFS(map[string]string{
		"a3/f9/a3f9deadbeef.info.json": `{"title": "Cereal Prize Spot", "description": "Saturday morning."}`,
	})
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1994,"audience":"kids","tags":["cereal"]}`))
	tagger := filler.NewTagger(st, llmMock, drop, func() time.Time { return time.Unix(1, 0) }, discardLog())

	if _, err := tagger.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(llmMock.Prompt(), "Cereal Prize Spot") {
		t.Errorf("the sidecar beside the clip was not read; prompt:\n%s", llmMock.Prompt())
	}
}

// ⚠ THE grounding-survival property (§10 V38c). §8 accepts an era only when the year appears
// LITERALLY in a text signal. Intake renames `Frosted Flakes 1993.mp4` to `a3f9….mp4`, so after
// filing, the only surviving copy of that year is the sidecar's `originalName` — and if the
// tagger does not read it back, every clip whose era came from its filename silently becomes
// ungrounded. The model's answer would be capped to a suggestion and nobody would be told why.
func TestTagger_GroundsTheEraOnTheOriginalFilename(t *testing.T) {
	st := newTagMemStore()
	// The catalog name has NO year — this is a clip whose era lived only in its filename.
	//
	// ⚠ Keyed by its HASH. This said "keyed by its PATH, which is what the tagger updates by"
	// until V41 — a comment that documented the bug as the design. The tagger passed a path into
	// the hash-keyed `UpdateClipTags`, so in production the write matched nothing; here it landed
	// because the fixture was keyed to match the defect. The sidecar is still found by PATH.
	const hash = "a3f9deadbeef"
	clip := untaggedClip(hash, "a3f9deadbeef")
	clip.Path = "a3/f9/a3f9deadbeef.mp4"
	st.seed(clip)

	drop := sidecarFS(map[string]string{
		"a3/f9/a3f9deadbeef.info.json": `{"loomarr":{"originalName":"Frosted Flakes 1993.mp4"}}`,
	})
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1993,"audience":"kids","tags":["cereal"]}`))
	tagger := filler.NewTagger(st, llmMock, drop, func() time.Time { return time.Unix(1, 0) }, discardLog())

	if _, err := tagger.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(llmMock.Prompt(), "Frosted Flakes 1993") {
		t.Errorf("the prompt carries the hash, not the original filename — the era signal is gone:\n%s",
			llmMock.Prompt())
	}
	// And it lands as a GROUNDED era, not a suggestion: 1993 appears literally in the text.
	// (Written to `updates`, which is where the double records tag writes — `clips` is the
	// work list it was seeded with.)
	got := st.updates[hash]
	if got.Era != 1993 {
		t.Errorf("Era = %d (suggested %d), want a grounded 1993 — the year was in the text signals",
			got.Era, got.SuggestedEra)
	}
}

func TestTagger_MalformedSidecarDegradesToFilename(t *testing.T) {
	st := newTagMemStore()
	st.clips["c1"] = untaggedClip("c1", "broken")

	drop := sidecarFS(map[string]string{"broken.info.json": `{not json at all`})
	// era 0: with no readable sidecar the year is nowhere in the text, so a nonzero
	// era would now be demoted to a suggestion (§10 V34) — 0 keeps this test about
	// the malformed-sidecar degradation, not the era rule covered above.
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":0,"audience":"general","tags":["tech"]}`))
	tagger := filler.NewTagger(st, llmMock, drop, func() time.Time { return time.Unix(1, 0) }, discardLog())

	res, err := tagger.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Partial != 1 {
		t.Errorf("Partial = %d, want 1 (a malformed sidecar must not fail the tag): %+v", res.Partial, res)
	}
}

// --- V38: the grounding-capped confidence score ---

// ⚠ THE safety property of V38, stated as a test: a model that is CERTAIN about an era it
// invented must not be able to talk its way past the auto-file gate.
//
// This is the same failure §10's grounding rule and migration 00024 exist for — the tagger
// inferred a decade from tone on 2 of 10 real clips — one level up: the model that fabricates
// the era also grades how sure it is about the era. If the score were self-reported, a confident
// fabrication would be auto-filed into the catalog and onto a channel with nobody looking.
func TestScore_UngroundedEraIsCappedBelowEveryReachableThreshold(t *testing.T) {
	// The model claims total certainty about an era that is NOT in the clip's text.
	ungrounded := filler.TagSuggestion{SuggestedEra: 1985, Audience: "kids", Category: "toys"}

	got := ungrounded.Score(100)
	if got >= filler.MaxAutoFileConfidence {
		t.Fatalf("Score = %d with a model claiming 100 — an ungrounded era reached %d, the highest "+
			"threshold an operator can set. A fabricated era would be auto-filed.", got, filler.MaxAutoFileConfidence)
	}
	// Belt and braces: no threshold in the settable range may admit it.
	for threshold := 0; threshold <= filler.MaxAutoFileConfidence; threshold++ {
		if threshold > got && threshold <= filler.MaxAutoFileConfidence {
			return // there exists a reachable threshold that excludes it — the gate can work
		}
	}
	t.Fatalf("no reachable threshold excludes an ungrounded era scoring %d", got)
}

// The model may LOWER the grounded ceiling but never lift it. A model unsure about a
// fully-verified clip is still worth surfacing; that direction is safe and allowed.
func TestScore_ModelCanOnlyLowerTheGroundedCeiling(t *testing.T) {
	grounded := filler.TagSuggestion{Era: 1985, Audience: "kids", Category: "toys"}

	if got := grounded.Score(30); got != 30 {
		t.Errorf("Score(30) on a grounded clip = %d, want 30 — the model must be able to lower it", got)
	}
	// 0 means "the model said nothing", so the grounding ceiling stands alone.
	full := grounded.Score(0)
	if got := grounded.Score(100); got != full {
		t.Errorf("Score(100) = %d but Score(0) = %d — the model lifted the ceiling", got, full)
	}
}

// An out-of-range self-report arrives in the same untrusted JSON as the tags. It must read as
// "the model said nothing" rather than becoming the score.
func TestClassify_ClampsAnOutOfRangeModelConfidence(t *testing.T) {
	for _, raw := range []string{"150", "-1"} {
		llmMock := testkit.NewLLM(testkit.FinalResponse(
			`{"era":1985,"audience":"kids","tags":["toys"],"confidence":` + raw + `}`))
		got, err := filler.Classify(context.Background(), llmMock, seedForest(), "Toy ad 1985", "A 1985 toy commercial")
		if err != nil {
			t.Fatal(err)
		}
		if got.Confidence < 0 || got.Confidence > 100 {
			t.Errorf("confidence %q produced a score of %d, outside 0-100", raw, got.Confidence)
		}
	}
}

// --- V38: the filing decision ---

// heldClip is an INGESTED clip waiting to be tagged and filed. The drop-folder path never
// produces one of these — a file an operator hand-copies is filed on sight (§10).
func heldClip(id, name string) filler.StoreClip {
	c := untaggedClip(id, name)
	c.Held = true
	return c
}

func autoFileAt(min int) filler.AutoFilePolicy {
	return filler.AutoFilePolicy{
		Enabled:       func() bool { return true },
		MinConfidence: func() int { return min },
	}
}

// A fully-grounded classification scores 100 and files without a human.
func TestTagger_FilesAHeldClipWhoseTagsAllVerify(t *testing.T) {
	st := newTagMemStore()
	st.clips["c1"] = heldClip("c1", "Toy ad 1985")

	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1985,"audience":"kids","tags":["toys"],"confidence":90}`))
	tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog()).
		WithAutoFile(autoFileAt(85))

	res, err := tagger.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.AutoFiled != 1 {
		t.Fatalf("AutoFiled = %d, want 1 (era 1985 is literally in the name)", res.AutoFiled)
	}
	// ⚠ Filing is keyed by PATH (`SetClipsHeld` is `WHERE path IN (…)`), unlike the tag write
	// two calls earlier, which is keyed by hash. The same loop legitimately uses both keys, and
	// this assertion names the one filing actually takes.
	if len(st.filed) != 1 || st.filed[0] != st.clips["c1"].Path {
		t.Fatalf("filed = %v, want [%s]", st.filed, st.clips["c1"].Path)
	}
	if !st.clips["c1"].AutoFiled {
		t.Error("auto_filed not recorded — the operator cannot find what was filed without them")
	}
}

// ⚠ THE end-to-end safety property, at the level a user experiences it. A model that is CERTAIN
// about an era found nowhere in the clip's text must leave that clip HELD, whatever it claims and
// whatever the threshold is set to.
//
// The unit test on Score pins the arithmetic; this pins that the arithmetic is actually wired to
// the filing decision. Both are needed: a correct cap that nothing consults protects nothing.
func TestTagger_NeverAutoFilesAnUngroundedEra(t *testing.T) {
	for _, threshold := range []int{50, 85, 95} {
		st := newTagMemStore()
		// No year anywhere in the name — so era 1985 can only have been inferred from tone.
		st.clips["c1"] = heldClip("c1", "Retro toy commercial")

		llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1985,"audience":"kids","tags":["toys"],"confidence":100}`))
		tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog()).
			WithAutoFile(autoFileAt(threshold))

		if _, err := tagger.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(st.filed) != 0 {
			t.Errorf("threshold %d: an UNGROUNDED era was auto-filed — a fabricated tag reached "+
				"a live channel with nobody looking", threshold)
		}
		if !st.clips["c1"].Held {
			t.Errorf("threshold %d: the clip is no longer held", threshold)
		}
		// The guess is still recorded as a question for the operator, not discarded.
		// ⚠ Read from `updates`, which is where this fake records a tag write (`clips` holds
		// the seeded state plus what SetClipsHeld changed). Asserting on the wrong map here
		// looked like a lost suggestion and was purely the fake's shape.
		if st.updates["c1"].SuggestedEra != 1985 {
			t.Errorf("threshold %d: the suggestion was lost (%d) — holding must not throw the "+
				"model's guess away, it is what makes the review one click",
				threshold, st.updates["c1"].SuggestedEra)
		}
	}
}

// Auto-filing off ⇒ everything waits for a human, however confident.
func TestTagger_FilesNothingWhenAutoFilingIsOff(t *testing.T) {
	st := newTagMemStore()
	st.clips["c1"] = heldClip("c1", "Toy ad 1985")

	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1985,"audience":"kids","tags":["toys"],"confidence":100}`))
	tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog()).
		WithAutoFile(filler.AutoFilePolicy{
			Enabled:       func() bool { return false },
			MinConfidence: func() int { return 85 },
		})

	if _, err := tagger.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(st.filed) != 0 {
		t.Errorf("filed %v with auto-filing switched off", st.filed)
	}
}

// ⚠ An ALREADY-FILED clip is never re-filed. Re-filing would set `auto_filed` on a clip a human
// had filed by hand, rewriting the answer to "did anyone look at this?" — and that flag is the
// only thing that can answer it.
func TestTagger_DoesNotRefileAClipAHumanAlreadyFiled(t *testing.T) {
	st := newTagMemStore()
	st.clips["c1"] = untaggedClip("c1", "Toy ad 1985") // not held: already in the catalog

	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1985,"audience":"kids","tags":["toys"],"confidence":100}`))
	tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog()).
		WithAutoFile(autoFileAt(85))

	res, err := tagger.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.AutoFiled != 0 || len(st.filed) != 0 {
		t.Errorf("re-filed an already-filed clip (%v) — that would stamp auto_filed on a human's "+
			"own decision", st.filed)
	}
}

// ⚠ **The score has to be PERSISTED, not just computed — the gap V51a closed.**
//
// `TagSuggestion.Score` was correct and unit-tested for two phases, and `Run` consulted it for the
// auto-file decision, but nothing ever wrote it to the clip. `UpsertClip` inserts a literal 0 and
// deliberately omits the column from its DO UPDATE, so `confidence` was 0 for every clip in every
// catalog — and the Incoming meter renders nothing at 0 (correctly: 0 means "never scored"), so
// the number an operator uses to decide whether to trust an auto-filed clip was permanently absent.
//
// The store's own conformance case passed throughout, because it wrote a value by hand and
// asserted the round trip. A column can round-trip perfectly and still have no producer, which is
// why this asserts the WRITE happened rather than that the value looks plausible.
func TestTagger_PersistsTheGroundedScore(t *testing.T) {
	for _, tc := range []struct {
		name  string
		clip  string
		reply string
		want  int
	}{
		// Everything verifies (the year is literally in the name) and the model does not lower it.
		{"fully grounded", "Toy ad 1985", `{"era":1985,"audience":"kids","tags":["toys"],"confidence":100}`, 100},
		// ⚠ The load-bearing row: an era found nowhere in the text is capped at 40 however certain
		// the model claims to be. If this ever records 100, the grounding cap is not wired.
		{"ungrounded era", "Retro toy commercial", `{"era":1985,"audience":"kids","tags":["toys"],"confidence":100}`, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newTagMemStore()
			st.clips["c1"] = heldClip("c1", tc.clip)
			path := st.clips["c1"].Path

			llmMock := testkit.NewLLM(testkit.FinalResponse(tc.reply))
			tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog()).
				WithAutoFile(autoFileAt(85))
			if _, err := tagger.Run(context.Background()); err != nil {
				t.Fatal(err)
			}

			got, written := st.confidence[path]
			if !written {
				t.Fatalf("no score was written for %q — Score computed one and the tagger dropped "+
					"it, which is exactly the state that shipped", path)
			}
			if got != tc.want {
				t.Errorf("confidence = %d, want %d", got, tc.want)
			}
		})
	}
}
