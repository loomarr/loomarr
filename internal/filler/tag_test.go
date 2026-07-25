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
func (m *tagMemStore) UpdateClipTags(_ context.Context, id string, era int, audience, category string, aiTagged bool, _ time.Time) error {
	c := m.clips[id]
	c.Era = era
	c.Audience = filler.Audience(audience)
	c.Category = category
	c.AITagged = aiTagged
	m.updates[id] = c
	return nil
}

func untaggedClip(id, name string) filler.StoreClip {
	c := filler.StoreClip{}
	c.TunarrProgramID = id
	c.Name = name
	c.Kind = filler.Commercial
	return c
}

// A well-formed classification is written with all three tags.
func TestTagger_WritesValidClassification(t *testing.T) {
	st := newTagMemStore()
	st.clips["c1"] = untaggedClip("c1", "Frosted Flakes ad")
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
// field (era) still lands.
func TestTagger_DropsHallucinatedEnums(t *testing.T) {
	st := newTagMemStore()
	st.clips["c1"] = untaggedClip("c1", "some ad")
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

// Classify directly: enum validation.
func TestClassify_ValidatesEnums(t *testing.T) {
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":1994,"audience":"late_night","category":"cars"}`),
	)
	sug, err := filler.Classify(context.Background(), llmMock, "car ad", "")
	if err != nil {
		t.Fatal(err)
	}
	if !sug.Complete() || sug.Era != 1994 || sug.Audience != filler.LateNight || sug.Category != "cars" {
		t.Errorf("valid classification mis-parsed: %+v", sug)
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
	clip := untaggedClip("c1", "mystery-clip")
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
	if !strings.Contains(prompt, "mystery-clip") {
		t.Errorf("prompt lost the filename:\n%s", prompt)
	}
	// With no sidecar there is no description line at all — better than a misleading one.
	if strings.Contains(prompt, "Source description:") {
		t.Errorf("prompt has an empty description line:\n%s", prompt)
	}
}

func TestTagger_MatchesSidecarAcrossNameNormalization(t *testing.T) {
	st := newTagMemStore()
	// Tunarr's scan tidies the display name; the file on disk keeps its own spelling.
	st.clips["c1"] = untaggedClip("c1", "Toy Ad (1994)")

	drop := sidecarFS(map[string]string{
		"toy_ad_1994.info.json": `{"title": "Cereal Prize Spot", "description": "Saturday morning."}`,
	})
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1994,"audience":"kids","category":"cereal"}`))
	tagger := filler.NewTagger(st, llmMock, drop, func() time.Time { return time.Unix(1, 0) }, discardLog())

	if _, err := tagger.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(llmMock.Prompt(), "Cereal Prize Spot") {
		t.Errorf("normalized name match failed; prompt:\n%s", llmMock.Prompt())
	}
}

func TestTagger_MalformedSidecarDegradesToFilename(t *testing.T) {
	st := newTagMemStore()
	st.clips["c1"] = untaggedClip("c1", "broken")

	drop := sidecarFS(map[string]string{"broken.info.json": `{not json at all`})
	llmMock := testkit.NewLLM(testkit.FinalResponse(`{"era":1990,"audience":"general","category":"tech"}`))
	tagger := filler.NewTagger(st, llmMock, drop, func() time.Time { return time.Unix(1, 0) }, discardLog())

	res, err := tagger.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Tagged != 1 {
		t.Errorf("Tagged = %d, want 1 (a malformed sidecar must not fail the tag)", res.Tagged)
	}
}
