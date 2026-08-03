package filler_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/testkit"
)

// tagMemStore is an in-memory TagStore.
type tagMemStore struct {
	clips   map[string]filler.StoreClip
	updates map[string]filler.StoreClip
	// filed records the paths SetClipsHeld filed, in call order — the auto-file assertions
	// read this rather than inspecting clips, so a test says "this was filed" rather than
	// "this ended up not-held", which a bug could also produce.
	filed []string
}

func newTagMemStore() *tagMemStore {
	return &tagMemStore{clips: map[string]filler.StoreClip{}, updates: map[string]filler.StoreClip{}}
}

func (m *tagMemStore) ListUntaggedCommercials(_ context.Context) ([]filler.StoreClip, error) {
	var out []filler.StoreClip
	for _, c := range m.clips {
		out = append(out, c)
	}
	return out, nil
}
func (m *tagMemStore) SetClipsHeld(_ context.Context, paths []string, held, autoFiled bool, _ time.Time) (int, error) {
	for _, p := range paths {
		c := m.clips[p]
		c.Held = held
		c.AutoFiled = autoFiled
		m.clips[p] = c
		if !held {
			m.filed = append(m.filed, p)
		}
	}
	return len(paths), nil
}
func (m *tagMemStore) UpdateClipTags(_ context.Context, id string, era int, audience, category string, suggestedEra int, aiTagged bool, _ time.Time) error {
	c := m.clips[id]
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

// untaggedClip builds a catalog clip. ⚠ Since V38c the sidecar is found by PATH, so a fixture
// giving a clip a sidecar must name it `<path minus extension>.info.json` — the shape intake
// produces. Fuzzy name matching is gone; see TestTagger_ReadsTheSidecarBesideTheClip.
func untaggedClip(id, name string) filler.StoreClip {
	c := filler.StoreClip{}
	c.Hash = id
	c.Path = id
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
		testkit.FinalResponse(`{"era":1992,"audience":"kids","category":"cereal"}`),
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
		testkit.FinalResponse(`{"era":1993,"audience":"cyberpunk","category":"nonsense_widgets"}`),
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
		testkit.FinalResponse(`{"era":9999,"audience":"kids","category":"toys"}`),
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
		testkit.FinalResponse(`{"era":1994,"audience":"late_night","category":"cars"}`),
	)
	sug, err := filler.Classify(context.Background(), llmMock, "car ad 1994", "")
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
		testkit.FinalResponse(`{"era":1980,"audience":"general","category":"tech"}`),
	)
	// Ad copy full of period-sounding language, but no year — the §6.4 failure shape.
	sug, err := filler.Classify(context.Background(), llmMock, "aqua-globes", "Transcript: the amazing new Aqua Globes water your plants for weeks...")
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
		testkit.FinalResponse(`{"era":1978,"audience":"family","category":"cereal"}`),
	)
	sug, err := filler.Classify(context.Background(), llmMock, "rice-krispies-treats", "Transcript: making treats since 1978, so easy...")
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
		testkit.FinalResponse(`{"era":1999,"audience":"general","category":"tech"}`),
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
		testkit.FinalResponse(`{"era":1999,"audience":"kids","category":"toys"}`),
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
	clip := untaggedClip("toy-ad-1994.mp4", "toy-ad-1994")
	clip.Source = "tunarr-local" // the provenance enum that used to reach the model
	st.clips["c1"] = clip

	drop := sidecarFS(map[string]string{
		"toy-ad-1994.info.json": `{
			"title": "Turbo Teen Transforming Car — TV Spot",
			"description": "Original 1994 broadcast commercial for the Turbo Teen toy line.",
			"uploader": "RetroAdVault"
		}`,
	})
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1994,"audience":"kids","category":"toys"}`))
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
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1990,"audience":"general","category":"cars"}`))
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
	clip := untaggedClip("a3/f9/a3f9deadbeef.mp4", "Toy Ad")
	st.clips["c1"] = clip

	drop := sidecarFS(map[string]string{
		"a3/f9/a3f9deadbeef.info.json": `{"title": "Cereal Prize Spot", "description": "Saturday morning."}`,
	})
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1994,"audience":"kids","category":"cereal"}`))
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
	// ⚠ Keyed by its PATH, which is what the tagger updates by; keying it "c1" would send the
	// write to a different entry and the assertion below would read an untouched zero value.
	const path = "a3/f9/a3f9deadbeef.mp4"
	st.clips[path] = untaggedClip(path, "a3f9deadbeef")

	drop := sidecarFS(map[string]string{
		"a3/f9/a3f9deadbeef.info.json": `{"loomarr":{"originalName":"Frosted Flakes 1993.mp4"}}`,
	})
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1993,"audience":"kids","category":"cereal"}`))
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
	got := st.updates[path]
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
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":0,"audience":"general","category":"tech"}`))
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
			`{"era":1985,"audience":"kids","category":"toys","confidence":` + raw + `}`))
		got, err := filler.Classify(context.Background(), llmMock, "Toy ad 1985", "A 1985 toy commercial")
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

	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1985,"audience":"kids","category":"toys","confidence":90}`))
	tagger := filler.NewTagger(st, llmMock, nil, func() time.Time { return time.Unix(1, 0) }, discardLog()).
		WithAutoFile(autoFileAt(85))

	res, err := tagger.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.AutoFiled != 1 {
		t.Fatalf("AutoFiled = %d, want 1 (era 1985 is literally in the name)", res.AutoFiled)
	}
	if len(st.filed) != 1 || st.filed[0] != "c1" {
		t.Fatalf("filed = %v, want [c1]", st.filed)
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

		llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1985,"audience":"kids","category":"toys","confidence":100}`))
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

	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1985,"audience":"kids","category":"toys","confidence":100}`))
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

	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1985,"audience":"kids","category":"toys","confidence":100}`))
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
