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
	"github.com/mantonx/loomarr/internal/taxonomy"
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

func (f *fakeTools) KeyframesIn(ctx context.Context, path string, _, _ int64, n int) ([][]byte, error) {
	return f.Keyframes(ctx, path, n)
}

func (f *fakeTools) Keyframes(_ context.Context, path string, _ int) ([][]byte, error) {
	// Scripted per basename (the vision/heuristic tiers pass drop-dir-joined paths),
	// so a unit test never shells ffmpeg for real JPEGs.
	return f.keyframes[filepath.Base(path)], nil
}

// ⚠ The written bytes are DERIVED FROM THE SPAN, and that is load-bearing rather than decorative.
// A segment's identity is the hash of its contents (§10 V38c), so a fake that wrote the same bytes
// for every cut would make every segment of a reel hash identically — they would collapse into one
// catalog row, correctly, and the test could not tell that outcome apart from the empty-hash bug
// V51a fixes. Real cuts of different spans differ; the fake has to as well.
func (f *fakeTools) Cut(_ context.Context, _ string, start, end int64, out string) error {
	f.cutCalls = append(f.cutCalls, fmt.Sprintf("%d-%d→%s", start, end, filepath.Base(out)))
	return os.WriteFile(out, []byte(fmt.Sprintf("cut %d-%d", start, end)), 0o644)
}

// splitMemStore is an in-memory SplitStore.
type splitMemStore struct {
	clips     map[string]filler.StoreClip
	proposals map[string]filler.SplitProposal
}

func newSplitMemStore() *splitMemStore {
	return &splitMemStore{clips: map[string]filler.StoreClip{}, proposals: map[string]filler.SplitProposal{}}
}

// ⚠ **Keyed by HASH, because `store.UpsertClip` is `ON CONFLICT(hash)` and `store.GetClip` is
// `WHERE hash = ?`.** This map was keyed on `Path`, and that single mismatch hid two shipped bugs
// at once: `Confirm` looked the compilation up by path (so no split could ever be committed), and
// every segment was upserted with an empty hash (so a 41-segment reel collapsed into one row).
// Both were invisible here because a path-keyed fixture answers a question production never asks.
// Keep this keyed exactly as the real store is — a fixture that indexes differently from the
// thing it stands in for cannot see key confusion by construction.
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
	m.clips[c.Hash] = c
	return nil
}
func (m *splitMemStore) DeleteClip(_ context.Context, id string) error {
	delete(m.clips, id)
	return nil
}

// SetClipComposite marks the parent composite by HASH (§10 V45) — a direct lookup now that the
// map is keyed the way the real store is. It used to scan for a matching `Hash` because the key
// was the path; that workaround was the fixture quietly admitting it indexed clips differently
// from production.
func (m *splitMemStore) SetClipComposite(_ context.Context, hash string, composite bool, _ time.Time) error {
	c, ok := m.clips[hash]
	if !ok {
		return fmt.Errorf("composite target not found: %s", hash)
	}
	c.IsComposite = composite
	m.clips[hash] = c
	return nil
}
func (m *splitMemStore) UpsertSplitProposal(_ context.Context, p filler.SplitProposal) error {
	for id, existing := range m.proposals {
		if existing.ClipHash == p.ClipHash && id != p.ID {
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

// ⚠ REFUSES to insert, exactly as the real store does. A fake that happily created the row would
// hide the resurrection race this method exists to prevent — the class this repo has already been
// bitten by twice (a double that never refuses cannot catch a write-through-a-dead-handle bug).
func (m *splitMemStore) UpdateSplitProposalSegments(_ context.Context, id string, segs []filler.SplitSegment) error {
	p, ok := m.proposals[id]
	if !ok {
		return fmt.Errorf("%w: %s", filler.ErrProposalGone, id)
	}
	p.Segments = segs
	m.proposals[id] = p
	return nil
}

// ListTaxa serves the REAL seed forest (§10 V45a), like tagMemStore — the splitter grounds each
// segment's tags against it, so a segment tagged `toys` resolves and an off-vocabulary slug is dropped
// exactly as a directly-tagged clip is. An empty graph would ground nothing and prove nothing.
func (m *splitMemStore) ListTaxa(_ context.Context) ([]taxonomy.Taxon, error) {
	return taxonomy.SeedForest(), nil
}

// seedCompilation files a compilation and RETURNS ITS HASH — the identity every caller then hands
// to `Propose`.
//
// ⚠ Returning it is the point. Callers used to pass the PATH to `Propose`, which is an identity
// parameter, and a path-keyed fixture happily answered — so the suite asserted against a lookup
// production does not perform. Handing back the hash means no test re-derives the identity, and
// none can express "look this clip up by its location" even by accident.
func seedCompilation(st *splitMemStore, path string, durationMs int64) string {
	c := filler.StoreClip{}
	// ⚠ Hash is the IDENTITY (§10 V38c) SetClipComposite/ParentHash key on, and it must be NON-EMPTY
	// and DISTINCT from the path. Leaving it "" made SetClipComposite(clip.Hash=="") match whichever
	// empty-hash clip the map iteration reached first — the compilation OR a freshly-cut segment — an
	// intermittent "compilation not marked composite" flake ([[loomarr-fixture-collapsed-keys]]).
	c.Hash = "hash-of-" + path
	c.Path = path
	c.Name = filepath.Base(path)
	c.Kind = filler.Commercial
	c.DurationMs = durationMs
	c.Source = "archive"
	c.License = "https://creativecommons.org/licenses/by/4.0/"
	c.Quality = "480p"
	st.clips[c.Hash] = c
	return c.Hash
}

func newSplitter(st *splitMemStore, tools filler.MediaTools, provider *testkit.LLM, dropDir string) *filler.Splitter {
	var p llm.Provider
	if provider != nil {
		p = provider
	}
	n := 0
	// ⚠ The REAL default (10s), not 0. The suite should exercise the number production runs with:
	// a splitter built with no floor would pass tests that the live 10s floor then fails, which is
	// exactly how the sub-floor problem stayed invisible until it was measured on a real reel.
	return filler.NewSplitter(st, tools, p, dropDir,
		func() time.Duration { return 10 * time.Second },
		func() string { n++; return fmt.Sprintf("sp_%d", n) },
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, nil)
}

// Chapters split for free: no black/silence pass runs, and titles become names.
func TestPropose_ChaptersShortCircuitDetection(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/1987.mp4", 61_000)
	tools := &fakeTools{chapters: []filler.Chapter{
		{StartMs: 0, EndMs: 30000, Title: "McDonald's"},
		{StartMs: 30000, EndMs: 61000, Title: "Lego"},
	}}
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":0,"audience":"kids","tags":["fast_food"]}`),
		testkit.FinalResponse(`{"era":0,"audience":"kids","tags":["toys"]}`),
	)
	drop := t.TempDir()
	sp := newSplitter(st, tools, llmMock, drop)

	p, err := sp.Propose(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	if tools.blackSilenceCall != 0 {
		t.Error("chapters present, but the coarse detector still ran")
	}
	// ⚠ Names come from the CHAPTERS, not from a model. The `Category` assertion that stood here
	// was checking `classify`, which V51g removed from `Propose` — 51 LLM turns inside a 120s pass,
	// on segments whose only input was a generated name. Tagging happens on each spawned segment's
	// own `tag` rung now, and the grounding rule it must obey is tested at `Classify` itself
	// (`TestClassify_UngroundedEraBecomesSuggestion`, `TestClassify_EraGroundedBySourceText`).
	if len(p.Segments) != 2 || p.Segments[0].Name != "McDonald's" || p.Segments[1].Name != "Lego" {
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
	hash := seedCompilation(st, "comps/1987.mp4", 90_000)
	tools := &fakeTools{
		blacks:   []filler.Interval{{StartMs: 900, EndMs: 1100}, {StartMs: 29800, EndMs: 30200}},
		silences: []filler.Interval{{StartMs: 59900, EndMs: 60100}},
	}
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"era":1987,"audience":"general","tags":["cars"]}`),
		testkit.FinalResponse(`{"era":0,"audience":"general","tags":["tech"]}`),
		testkit.FinalResponse(`{"era":0,"audience":"general","tags":["cereal"]}`),
	)
	sp := newSplitter(st, tools, llmMock, t.TempDir())

	p, err := sp.Propose(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	// The 1s head sliver is dropped; three boundaries → three kept segments.
	if len(p.Segments) != 3 {
		t.Fatalf("segments = %+v, want 3", p.Segments)
	}
	// ⚠ Name only. The `Era != 1987` half asserted `classify`, which no longer runs here (V51g) —
	// the era is settled on the segment's own `tag` rung, after `transcribe`, where there is
	// actually text to ground it in. Here the name is derived from the parent, and that IS
	// `Propose`'s job.
	if p.Segments[0].Name == "" {
		t.Errorf("segment naming wrong: %+v", p.Segments[0])
	}
	// ⚠ And the proposal must carry NO tags at all — a half-classified segment would be worse
	// than an untagged one, because the review screen would present a guess as a finding.
	if p.Segments[0].Era != 0 || p.Segments[0].SuggestedEra != 0 || len(p.Segments[0].Tags) != 0 {
		t.Errorf("Propose classified a segment; that belongs to the tag rung: %+v", p.Segments[0])
	}
	if p.Segments[0].StartMs != 1000 || p.Segments[2].EndMs != 90_000 {
		t.Errorf("cut positions wrong: %+v", p.Segments)
	}
}

// ⚠ The rescue's reason to exist: a 149s block with NO A/V boundaries, holding
// three adverts that only the transcript can see (plan §6.4's measured case).
func TestPropose_RescueSplitsWhatDetectorsCouldNot(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/late-night.mp4", 149_000)
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
		testkit.FinalResponse(`{"era":0,"audience":"general","tags":["tech"]}`),
		testkit.FinalResponse(`{"era":1987,"audience":"general","tags":["tech"]}`),
		testkit.FinalResponse(`{"era":1950,"audience":"general","tags":["tech"]}`),
	)
	sp := newSplitter(st, tools, llmMock, t.TempDir())

	p, err := sp.Propose(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Segments) != 3 {
		t.Fatalf("rescue produced %+v, want 3 segments", p.Segments)
	}
	if p.Segments[1].Name != "Aqua Globes" || p.Segments[1].StartMs != 27_000 {
		t.Errorf("rescued boundary wrong: %+v", p.Segments[1])
	}
	// ⚠ The era-grounding assertions that stood here MOVED, they were not dropped (§8 is not
	// negotiable). They exercised `Classify` through `Propose`'s `classify` call, which V51g
	// removed; the rule itself is unchanged and is tested at its own seam —
	// `TestClassify_EraGroundedBySourceText` (a year present in the text becomes `Era`) and
	// `TestClassify_UngroundedEraBecomesSuggestion` (an invented one becomes `SuggestedEra`,
	// never a tag). Every spawned segment reaches that code on its own `tag` rung.
	//
	// What `Propose` owes is the CUT, and that is what is asserted above: three segments, the
	// rescued boundary at the right millisecond, named from the parent. Tags are the tag rung's.
	for i, seg := range p.Segments {
		if seg.Era != 0 || seg.SuggestedEra != 0 || len(seg.Tags) != 0 {
			t.Errorf("segment %d arrived classified; Propose cuts, it does not describe: %+v", i, seg)
		}
	}
}

// ⚠ The load-bearing single-advert case: a 121s infomercial for ONE product
// must come back as ONE segment — never manufactured into clips (§6.4).
func TestPropose_SingleLongAdvertIsNotManufactured(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/infomercial.mp4", 121_000)
	tools := &fakeTools{
		transcripts: map[string][]filler.TranscriptSegment{
			"0:121000": {{StartMs: 0, EndMs: 121000, Text: "The amazing knife slices, dices, and juliennes"}},
		},
	}
	llmMock := testkit.NewLLM(
		testkit.FinalResponse(`{"adverts":[{"start":"00:00","end":"02:01","product":"amazing knife"}]}`),
		testkit.FinalResponse(`{"era":0,"audience":"general","tags":["tech"]}`),
	)
	sp := newSplitter(st, tools, llmMock, t.TempDir())

	p, err := sp.Propose(context.Background(), hash)
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
	hash := seedCompilation(st, "comps/late-night.mp4", 149_000)
	tools := &fakeTools{transcribeErr: fmt.Errorf("whisper not configured")}
	sp := newSplitter(st, tools, testkit.NewLLM(), t.TempDir())

	p, err := sp.Propose(context.Background(), hash)
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
	hash := seedCompilation(st, "comps/1987.mp4", 160_000)
	tools := &fakeTools{blacks: []filler.Interval{{StartMs: 29800, EndMs: 30200}}}
	sp := newSplitter(st, tools, nil, t.TempDir())

	p, err := sp.Propose(context.Background(), hash)
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
	hash := seedCompilation(st, "comps/1987.mp4", 60_000)
	existing := filler.StoreClip{}
	// ⚠ A distinct, non-empty hash. Left empty, this clip and any other unhashed fixture share
	// the map key "" — the collapsed-key trap that has now hidden four separate bugs in this file.
	existing.Hash = "hash-of-old/mcdonalds.mp4"
	existing.Path = "old/mcdonalds.mp4"
	existing.DurationMs = 30_000
	st.clips[existing.Hash] = existing

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

	p, err := sp.Propose(context.Background(), hash)
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
	hash := seedCompilation(st, "comps/1987.mp4", 61_000)
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
	// ⚠ Capture the proposal id from Propose's return, NOT by ranging st.proposals — Go randomises map
	// order, so if the store ever holds >1 proposal the range picked an arbitrary one and Confirm ran
	// against the wrong id (an intermittent "compilation not marked composite" flake, now fixed
	// deterministically). See [[loomarr-splitjob-test-map-order-flake]].
	prop, err := sp.Propose(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	propID := prop.ID

	// The operator's EDITED list: era suggestion accepted on the second segment,
	// and a third segment they added by hand.
	edited := []filler.SplitSegment{
		{StartMs: 0, EndMs: 30000, Name: "McDonald's", Era: 1987, Audience: filler.Kids, Category: "fast_food"},
		{StartMs: 30000, EndMs: 61000, Name: "Lego", Era: 1987, Audience: filler.Kids, Category: "toys"},
	}
	if _, err := sp.Confirm(context.Background(), propID, edited); err != nil {
		t.Fatal(err)
	}

	if len(tools.cutCalls) != 2 {
		t.Fatalf("cuts = %v", tools.cutCalls)
	}
	// ⚠ V45: the compilation is KEPT and marked a composite (NOT deleted — the reversal of V34). Its
	// row survives, flagged is_composite so pod assembly excludes it, and its file stays for re-split.
	comp, found, _ := st.GetClip(context.Background(), hash)
	if !found {
		t.Fatal("compilation row was deleted on confirm — V45 keeps the parent as a composite")
	}
	if !comp.IsComposite {
		t.Error("compilation survived but was not marked is_composite — it would still be airable")
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("compilation file was removed on confirm — V45 keeps it for re-splitting")
	}
	// …the proposal is consumed…
	if _, err := st.GetSplitProposal(context.Background(), propID); err == nil {
		t.Error("proposal survived confirm")
	}
	// …and the segments are real clips with tags, durations, provenance, AND lineage back to the
	// composite. Skip the kept composite itself when counting segments.
	var segments []filler.StoreClip
	for path, c := range st.clips {
		if c.IsComposite {
			continue // the kept parent, not a segment
		}
		if c.DurationMs <= 0 || c.Kind != filler.Commercial || c.License == "" || c.Source != "archive" {
			t.Errorf("clip %s missing duration/kind/provenance: %+v", path, c.Clip)
		}
		if c.ParentHash != comp.Hash {
			t.Errorf("segment %s parent_hash = %q, want the composite's hash %q — lineage is the point of V45", c.Name, c.ParentHash, comp.Hash)
		}
		segments = append(segments, c)
	}
	if len(segments) != 2 {
		t.Fatalf("segments = %+v", segments)
	}
	// The cut files exist at cataloged paths (segments only; the composite keeps its own file).
	for _, seg := range segments {
		if _, err := os.Stat(filepath.Join(drop, seg.Path)); err != nil {
			t.Errorf("cataloged segment %s has no file: %v", seg.Path, err)
		}
	}
}

// Confirm REJECTS a gutted compilation and overlapping cuts (§10 — the write
// path is where the review gate's teeth are).
func TestConfirm_ValidatesTheEdit(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/1987.mp4", 61_000)
	tools := &fakeTools{}
	sp := newSplitter(st, tools, nil, t.TempDir())
	if _, err := sp.Propose(context.Background(), hash); err != nil {
		t.Fatal(err)
	}
	var propID string
	for id := range st.proposals {
		propID = id
	}

	if _, err := sp.Confirm(context.Background(), propID, nil); err == nil {
		t.Error("zero-segment confirm accepted — the compilation would be gutted")
	}
	overlap := []filler.SplitSegment{
		{StartMs: 0, EndMs: 31000, Name: "a"},
		{StartMs: 30000, EndMs: 61000, Name: "b"},
	}
	if _, err := sp.Confirm(context.Background(), propID, overlap); err == nil {
		t.Error("overlapping confirm accepted")
	}
	// Nothing was written on the failures.
	if len(st.clips) != 1 || len(tools.cutCalls) != 0 {
		t.Errorf("a rejected confirm still wrote: clips=%+v cuts=%v", st.clips, tools.cutCalls)
	}
}
