package filler_test

import (
	"context"
	"testing"
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
	tagger := filler.NewTagger(st, llmMock, func() time.Time { return time.Unix(1, 0) }, discardLog())

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
	tagger := filler.NewTagger(st, llmMock, func() time.Time { return time.Unix(1, 0) }, discardLog())

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
	tagger := filler.NewTagger(st, llmMock, func() time.Time { return time.Unix(1, 0) }, discardLog())
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
