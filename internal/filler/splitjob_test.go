package filler_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/testkit"
)

// fakeTools scripts MediaTools per test (§19 — unit tests never exec a binary).
type fakeTools struct {
	chapters      []filler.Chapter
	blacks        []filler.Interval
	silences      []filler.Interval
	transcripts   map[string][]filler.TranscriptSegment // "start:end" → utterances
	transcribeErr error
	grayFrames    map[string][][]byte // "path|start|end" → frames
	keyframes     map[string][][]byte // basename → JPEG frames (V44 vision/heuristic input)

	chapterCalls     int
	blackSilenceCall int
	cutCalls         []string
}

func key3(path string, start, end int64) string { return fmt.Sprintf("%s|%d|%d", path, start, end) }

func (f *fakeTools) Chapters(context.Context, string) ([]filler.Chapter, error) {
	f.chapterCalls++
	return f.chapters, nil
}

func (f *fakeTools) BlackSilence(context.Context, string) ([]filler.Interval, []filler.Interval, error) {
	f.blackSilenceCall++
	return f.blacks, f.silences, nil
}

func (f *fakeTools) Transcribe(_ context.Context, _ string, start, end int64) ([]filler.TranscriptSegment, error) {
	if f.transcribeErr != nil {
		return nil, f.transcribeErr
	}
	return f.transcripts[fmt.Sprintf("%d:%d", start, end)], nil
}

func (f *fakeTools) GrayFrames(_ context.Context, path string, start, end int64) ([][]byte, error) {
	// The splitter passes drop-dir-joined paths; tests key on the basename.
	frames, ok := f.grayFrames[key3(filepath.Base(path), start, end)]
	if !ok {
		return nil, fmt.Errorf("no frames for %s", key3(path, start, end))
	}
	return frames, nil
}

func (f *fakeTools) Keyframes(_ context.Context, path string, _ int) ([][]byte, error) {
	// Scripted per basename (the vision/heuristic tiers pass drop-dir-joined paths),
	// so a unit test never shells ffmpeg for real JPEGs.
	return f.keyframes[filepath.Base(path)], nil
}

func (f *fakeTools) Cut(_ context.Context, _ string, start, end int64, out string) error {
	f.cutCalls = append(f.cutCalls, fmt.Sprintf("%d-%d→%s", start, end, filepath.Base(out)))
	return os.WriteFile(out, []byte("cut"), 0o644)
}

// splitMemStore is an in-memory SplitStore.
type splitMemStore struct {
	clips     map[string]filler.StoreClip
	proposals map[string]filler.SplitProposal
}

func newSplitMemStore() *splitMemStore {
	return &splitMemStore{clips: map[string]filler.StoreClip{}, proposals: map[string]filler.SplitProposal{}}
}

func (m *splitMemStore) GetClip(_ context.Context, id string) (filler.StoreClip, bool, error) {
	c, ok := m.clips[id]
	return c, ok, nil
}
func (m *splitMemStore) ListClips(context.Context) ([]filler.StoreClip, error) {
	var out []filler.StoreClip
	for _, c := range m.clips {
		out = append(out, c)
	}
	return out, nil
}
func (m *splitMemStore) UpsertClip(_ context.Context, c filler.StoreClip) error {
	m.clips[c.Path] = c
	return nil
}
func (m *splitMemStore) DeleteClip(_ context.Context, id string) error {
	delete(m.clips, id)
	return nil
}
func (m *splitMemStore) UpsertSplitProposal(_ context.Context, p filler.SplitProposal) error {
	for id, existing := range m.proposals {
		if existing.ClipPath == p.ClipPath && id != p.ID {
			delete(m.proposals, id) // one proposal per clip, like the store's UNIQUE
		}
	}
	m.proposals[p.ID] = p
	return nil
}
func (m *splitMemStore) GetSplitProposal(_ context.Context, id string) (filler.SplitProposal, error) {
	p, ok := m.proposals[id]
	if !ok {
		return filler.SplitProposal{}, fmt.Errorf("not found")
	}
	return p, nil
}
func (m *splitMemStore) DeleteSplitProposal(_ context.Context, id string) error {
	delete(m.proposals, id)
	return nil
}

func seedCompilation(st *splitMemStore, path string, durationMs int64) {
	c := filler.StoreClip{}
	c.Path = path
	c.Name = filepath.Base(path)
	c.Kind = filler.Commercial
	c.DurationMs = durationMs
	c.Source = "archive"
	c.License = "https://creativecommons.org/licenses/by/4.0/"
	c.Quality = "480p"
	st.clips[path] = c
}

func newSplitter(st *splitMemStore, tools filler.MediaTools, provider *testkit.LLM, dropDir string) *filler.Splitter {
	var p llm.Provider
	if provider != nil {
		p = provider
	}
	n := 0
	return filler.NewSplitter(st, tools, p, dropDir,
		func() string { n++; return fmt.Sprintf("sp_%d", n) },
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, nil)
}

// Chapters split for free: no black/silence pass runs, and titles become names.
func TestPropose_ChaptersShortCircuitDetection(t *testing.T) {
	st := newSplitMemStore()
	seedCompilation(st, "comps/1987.mp4", 61_000)
	tools := &fakeTools{chapters: []filler.Chapter{
		{StartMs: 0, EndMs: 30000, Title: "McDonald's"},
		{StartMs: 30000, EndMs: 61000, Title: "Lego"},
	}}
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":0,"audience":"kids","category":"fast_food"}`),
		testkit.FinalResponse(`{"era":0,"audience":"kids","category":"toys"}`),
	)
	drop := t.TempDir()
	sp := newSplitter(st, tools, llmMock, drop)

	p, err := sp.Propose(context.Background(), "comps/1987.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if tools.blackSilenceCall != 0 {
		t.Error("chapters present, but the coarse detector still ran")
	}
	if len(p.Segments) != 2 || p.Segments[0].Name != "McDonald's" || p.Segments[1].Category != "toys" {
		t.Errorf("proposal = %+v", p.Segments)
	}
	// Persisted — review happens later, possibly after a restart.
	if _, err := st.GetSplitProposal(context.Background(), p.ID); err != nil {
		t.Errorf("proposal not persisted: %v", err)
	}
	// The catalog is UNTOUCHED: propose never writes clips.
	if len(st.clips) != 1 {
		t.Errorf("propose wrote to the catalog: %+v", st.clips)
	}
}

// Coarse split: black/silence boundaries cut, slivers dropped, parts named.
func TestPropose_CoarseSplit(t *testing.T) {
	st := newSplitMemStore()
	seedCompilation(st, "comps/1987.mp4", 90_000)
	tools := &fakeTools{
		blacks:   []filler.Interval{{StartMs: 900, EndMs: 1100}, {StartMs: 29800, EndMs: 30200}},
		silences: []filler.Interval{{StartMs: 59900, EndMs: 60100}},
	}
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":1987,"audience":"general","category":"cars"}`),
		testkit.FinalResponse(`{"era":0,"audience":"general","category":"tech"}`),
		testkit.FinalResponse(`{"era":0,"audience":"general","category":"cereal"}`),
	)
	sp := newSplitter(st, tools, llmMock, t.TempDir())

	p, err := sp.Propose(context.Background(), "comps/1987.mp4")
	if err != nil {
		t.Fatal(err)
	}
	// The 1s head sliver is dropped; three boundaries → three kept segments.
	if len(p.Segments) != 3 {
		t.Fatalf("segments = %+v, want 3", p.Segments)
	}
	if p.Segments[0].Name == "" || p.Segments[0].Era != 1987 {
		t.Errorf("segment naming/era wrong: %+v", p.Segments[0])
	}
	if p.Segments[0].StartMs != 1000 || p.Segments[2].EndMs != 90_000 {
		t.Errorf("cut positions wrong: %+v", p.Segments)
	}
}

// ⚠ The rescue's reason to exist: a 149s block with NO A/V boundaries, holding
// three adverts that only the transcript can see (plan §6.4's measured case).
func TestPropose_RescueSplitsWhatDetectorsCouldNot(t *testing.T) {
	st := newSplitMemStore()
	seedCompilation(st, "comps/late-night.mp4", 149_000)
	tools := &fakeTools{ // no chapters, no black, no silence — the measured shape
		transcripts: map[string][]filler.TranscriptSegment{
			"0:149000": {
				{StartMs: 0, EndMs: 27000, Text: "The Swiffer sweeper picks up dust"},
				{StartMs: 27400, EndMs: 54000, Text: "Aqua Globes water your plants since 1987"},
				{StartMs: 54000, EndMs: 149000, Text: "Call now for the amazing knife set"},
			},
		},
	}
	llmMock := testkit.NewLLM(
		// 1: the rescue — three adverts, cut at 27.4s exactly (the measured cut).
		testkit.FinalResponse(`{"adverts":[
			{"start":"00:00","end":"00:27","product":"Swiffer"},
			{"start":"00:27","end":"00:54","product":"Aqua Globes"},
			{"start":"00:54","end":"02:29","product":"amazing knife set"}]}`),
		// 2-4: classify each rescued segment. The Aqua Globes era is grounded
		// ("since 1987" IS in the transcript); the knife era is INVENTED.
		testkit.FinalResponse(`{"era":0,"audience":"general","category":"tech"}`),
		testkit.FinalResponse(`{"era":1987,"audience":"general","category":"tech"}`),
		testkit.FinalResponse(`{"era":1950,"audience":"general","category":"tech"}`),
	)
	sp := newSplitter(st, tools, llmMock, t.TempDir())

	p, err := sp.Propose(context.Background(), "comps/late-night.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Segments) != 3 {
		t.Fatalf("rescue produced %+v, want 3 segments", p.Segments)
	}
	if p.Segments[1].Name != "Aqua Globes" || p.Segments[1].StartMs != 27_000 {
		t.Errorf("rescued boundary wrong: %+v", p.Segments[1])
	}
	// Era grounding INSIDE the pipeline: the grounded year lands as a tag…
	if p.Segments[1].Era != 1987 {
		t.Errorf("grounded era not tagged: %+v", p.Segments[1])
	}
	// …and the invented one is a suggestion, never a tag (§8).
	if p.Segments[2].Era != 0 || p.Segments[2].SuggestedEra != 1950 {
		t.Errorf("invented era mishandled: %+v", p.Segments[2])
	}
}

// ⚠ The load-bearing single-advert case: a 121s infomercial for ONE product
// must come back as ONE segment — never manufactured into clips (§6.4).
func TestPropose_SingleLongAdvertIsNotManufactured(t *testing.T) {
	st := newSplitMemStore()
	seedCompilation(st, "comps/infomercial.mp4", 121_000)
	tools := &fakeTools{
		transcripts: map[string][]filler.TranscriptSegment{
			"0:121000": {{StartMs: 0, EndMs: 121000, Text: "The amazing knife slices, dices, and juliennes"}},
		},
	}
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"adverts":[{"start":"00:00","end":"02:01","product":"amazing knife"}]}`),
		testkit.FinalResponse(`{"era":0,"audience":"general","category":"tech"}`),
	)
	sp := newSplitter(st, tools, llmMock, t.TempDir())

	p, err := sp.Propose(context.Background(), "comps/infomercial.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Segments) != 1 {
		t.Fatalf("single advert manufactured into %d clips: %+v", len(p.Segments), p.Segments)
	}
	if p.Segments[0].Unsplittable {
		t.Error("a successfully-rescued single advert is not 'unsplittable'")
	}
}

// No whisper (or a whisper failure) ⇒ Unsplittable, NEVER a guessed cut (§15).
func TestPropose_WhisperFailureMarksUnsplittable(t *testing.T) {
	st := newSplitMemStore()
	seedCompilation(st, "comps/late-night.mp4", 149_000)
	tools := &fakeTools{transcribeErr: fmt.Errorf("whisper not configured")}
	sp := newSplitter(st, tools, testkit.NewLLM(), t.TempDir())

	p, err := sp.Propose(context.Background(), "comps/late-night.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Segments) != 1 || !p.Segments[0].Unsplittable {
		t.Fatalf("whisper failure = %+v, want one Unsplittable segment", p.Segments)
	}
}

// No LLM at all: coarse split still works, over-long segments say Unsplittable,
// and nothing is classified — the honest degradation.
func TestPropose_NoLLMDegradesHonestly(t *testing.T) {
	st := newSplitMemStore()
	seedCompilation(st, "comps/1987.mp4", 160_000)
	tools := &fakeTools{blacks: []filler.Interval{{StartMs: 29800, EndMs: 30200}}}
	sp := newSplitter(st, tools, nil, t.TempDir())

	p, err := sp.Propose(context.Background(), "comps/1987.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Segments) != 2 {
		t.Fatalf("segments = %+v", p.Segments)
	}
	if !p.Segments[1].Unsplittable {
		t.Errorf("the over-long segment must say Unsplittable with no LLM: %+v", p.Segments[1])
	}
	for _, s := range p.Segments {
		if s.Era != 0 || s.Category != "" || s.Audience != "" {
			t.Errorf("classified without a provider: %+v", s)
		}
	}
}

// Dedup flags a segment matching a catalog clip — and never matches the
// compilation itself.
func TestPropose_DedupFlagsExistingClips(t *testing.T) {
	st := newSplitMemStore()
	seedCompilation(st, "comps/1987.mp4", 60_000)
	existing := filler.StoreClip{}
	existing.Path = "old/mcdonalds.mp4"
	existing.DurationMs = 30_000
	st.clips[existing.Path] = existing

	dupFrame := make([]byte, 72)
	for i := range dupFrame {
		dupFrame[i] = byte(i) // a deterministic non-flat pattern
	}
	compFrame := make([]byte, 72)
	for i := range compFrame {
		compFrame[i] = byte(255 - i) // a DIFFERENT pattern — matches only the compilation
	}
	tools := &fakeTools{
		blacks: []filler.Interval{{StartMs: 29800, EndMs: 30200}},
		grayFrames: map[string][][]byte{
			key3("1987.mp4", 0, 30000):       {dupFrame},
			key3("1987.mp4", 30000, 60_000):  {compFrame}, // matches ONLY the compilation
			key3("1987.mp4", 0, 60_000):      {compFrame}, // the compilation, hashed whole
			key3("mcdonalds.mp4", 0, 30_000): {dupFrame},
		},
	}
	sp := newSplitter(st, tools, nil, t.TempDir())

	p, err := sp.Propose(context.Background(), "comps/1987.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if p.Segments[0].DupOf != "old/mcdonalds.mp4" {
		t.Errorf("duplicate not flagged: %+v", p.Segments[0])
	}
	// ⚠ The compilation itself is EXCLUDED from the candidate set — otherwise
	// every segment would "duplicate" the file it was cut from, and the flag
	// would cry wolf on exactly the clips that are new.
	if p.Segments[1].DupOf != "" {
		t.Errorf("segment flagged against its OWN compilation: %+v", p.Segments[1])
	}
}

// Confirm cuts, catalogs, and consumes — and leaves the compilation behind.
func TestConfirm_WritesReviewedSegments(t *testing.T) {
	st := newSplitMemStore()
	seedCompilation(st, "comps/1987.mp4", 61_000)
	drop := t.TempDir()
	if err := os.MkdirAll(filepath.Join(drop, "comps"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(drop, "comps", "1987.mp4")
	if err := os.WriteFile(src, []byte("compilation"), 0o644); err != nil {
		t.Fatal(err)
	}
	tools := &fakeTools{}
	sp := newSplitter(st, tools, nil, drop)
	if _, err := sp.Propose(context.Background(), "comps/1987.mp4"); err != nil {
		t.Fatal(err)
	}
	var propID string
	for id := range st.proposals {
		propID = id
	}

	// The operator's EDITED list: era suggestion accepted on the second segment,
	// and a third segment they added by hand.
	edited := []filler.SplitSegment{
		{StartMs: 0, EndMs: 30000, Name: "McDonald's", Era: 1987, Audience: filler.Kids, Category: "fast_food"},
		{StartMs: 30000, EndMs: 61000, Name: "Lego", Era: 1987, Audience: filler.Kids, Category: "toys"},
	}
	if err := sp.Confirm(context.Background(), propID, edited); err != nil {
		t.Fatal(err)
	}

	if len(tools.cutCalls) != 2 {
		t.Fatalf("cuts = %v", tools.cutCalls)
	}
	// The compilation row AND file are gone…
	if _, found, _ := st.GetClip(context.Background(), "comps/1987.mp4"); found {
		t.Error("compilation row survived confirm")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("compilation file survived confirm — the next sync would resurrect it")
	}
	// …the proposal is consumed…
	if _, err := st.GetSplitProposal(context.Background(), propID); err == nil {
		t.Error("proposal survived confirm")
	}
	// …and the segments are real clips with tags, durations, and provenance.
	var names []string
	for path, c := range st.clips {
		if c.DurationMs <= 0 || c.Kind != filler.Commercial || c.License == "" || c.Source != "archive" {
			t.Errorf("clip %s missing duration/kind/provenance: %+v", path, c.Clip)
		}
		names = append(names, c.Name)
	}
	if len(names) != 2 {
		t.Fatalf("catalog = %+v", st.clips)
	}
	// The cut files exist at cataloged paths.
	for path := range st.clips {
		if _, err := os.Stat(filepath.Join(drop, path)); err != nil {
			t.Errorf("cataloged clip %s has no file: %v", path, err)
		}
	}
}

// Confirm REJECTS a gutted compilation and overlapping cuts (§10 — the write
// path is where the review gate's teeth are).
func TestConfirm_ValidatesTheEdit(t *testing.T) {
	st := newSplitMemStore()
	seedCompilation(st, "comps/1987.mp4", 61_000)
	tools := &fakeTools{}
	sp := newSplitter(st, tools, nil, t.TempDir())
	if _, err := sp.Propose(context.Background(), "comps/1987.mp4"); err != nil {
		t.Fatal(err)
	}
	var propID string
	for id := range st.proposals {
		propID = id
	}

	if err := sp.Confirm(context.Background(), propID, nil); err == nil {
		t.Error("zero-segment confirm accepted — the compilation would be gutted")
	}
	overlap := []filler.SplitSegment{
		{StartMs: 0, EndMs: 31000, Name: "a"},
		{StartMs: 30000, EndMs: 61000, Name: "b"},
	}
	if err := sp.Confirm(context.Background(), propID, overlap); err == nil {
		t.Error("overlapping confirm accepted")
	}
	// Nothing was written on the failures.
	if len(st.clips) != 1 || len(tools.cutCalls) != 0 {
		t.Errorf("a rejected confirm still wrote: clips=%+v cuts=%v", st.clips, tools.cutCalls)
	}
}
