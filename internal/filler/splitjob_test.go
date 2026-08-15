package filler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	boundarySpans    [][2]int64
	boundaryFn       func(context.Context, int64, int64) ([]filler.Interval, error)
	cutCalls         []string
	grayCalls        []string
	grayHook         func(string, int64, int64)
}

func key3(path string, start, end int64) string { return fmt.Sprintf("%s|%d|%d", path, start, end) }

func (f *fakeTools) Chapters(context.Context, string) ([]filler.Chapter, error) {
	f.chapterCalls++
	return f.chapters, nil
}

func (f *fakeTools) Boundaries(ctx context.Context, _ string, startMs, endMs int64) ([]filler.Interval, []filler.Interval, error) {
	f.blackSilenceCall++
	f.boundarySpans = append(f.boundarySpans, [2]int64{startMs, endMs})
	if f.boundaryFn != nil {
		gaps, err := f.boundaryFn(ctx, startMs, endMs)
		return gaps, nil, err
	}
	return append([]filler.Interval(nil), f.blacks...), append([]filler.Interval(nil), f.silences...), nil
}

func (f *fakeTools) Transcribe(_ context.Context, _ string, start, end int64) ([]filler.TranscriptSegment, error) {
	if f.transcribeErr != nil {
		return nil, f.transcribeErr
	}
	return f.transcripts[fmt.Sprintf("%d:%d", start, end)], nil
}

func (f *fakeTools) GrayFrames(_ context.Context, path string, start, end int64) ([][]byte, error) {
	// The splitter passes drop-dir-joined paths; tests key on the basename.
	if f.grayHook != nil {
		f.grayHook(filepath.Base(path), start, end)
	}
	f.grayCalls = append(f.grayCalls, key3(filepath.Base(path), start, end))
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
	clips        map[string]filler.StoreClip
	proposals    map[string]filler.SplitProposal
	fingerprints map[string][]uint64
	// roundTripProposals makes the fake cross the same JSON durability boundary as the SQL store.
	// It is opt-in because most splitter tests exercise domain behavior, while checkpoint tests
	// specifically need private in-memory fields to disappear between passes like they do live.
	roundTripProposals bool
	// Captures whether cache population incorrectly reused an expired pipeline context.
	fingerprintWriteCtxErr error
	fingerprintReadErr     error
	fingerprintWriteErr    error
}

func newSplitMemStore() *splitMemStore {
	return &splitMemStore{
		clips: map[string]filler.StoreClip{}, proposals: map[string]filler.SplitProposal{},
		fingerprints: map[string][]uint64{},
	}
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
func (m *splitMemStore) ListClipFingerprints(_ context.Context, algorithm string) (map[string][]uint64, error) {
	if m.fingerprintReadErr != nil {
		return nil, m.fingerprintReadErr
	}
	out := make(map[string][]uint64)
	for key, frames := range m.fingerprints {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) == 2 && parts[1] == algorithm {
			out[parts[0]] = append([]uint64(nil), frames...)
		}
	}
	return out, nil
}
func (m *splitMemStore) UpsertClipFingerprint(ctx context.Context, clipHash, algorithm string, frames []uint64) error {
	m.fingerprintWriteCtxErr = ctx.Err()
	if m.fingerprintWriteErr != nil {
		return m.fingerprintWriteErr
	}
	m.fingerprints[clipHash+"|"+algorithm] = append([]uint64(nil), frames...)
	return nil
}
func (m *splitMemStore) UpsertClip(_ context.Context, c filler.StoreClip) error {
	if old, ok := m.clips[c.Hash]; ok {
		// Match production's lifecycle preservation on conflict.
		c.RemovedAt = old.RemovedAt
		c.ParentHash = old.ParentHash
	}
	m.clips[c.Hash] = c
	return nil
}
func (m *splitMemStore) GetClipTags(_ context.Context, hash string, leavesOnly bool) ([]string, error) {
	c, ok := m.clips[hash]
	if !ok {
		return nil, fmt.Errorf("clip not found: %s", hash)
	}
	if leavesOnly {
		return append([]string(nil), c.AssertedTags...), nil
	}
	return append([]string(nil), c.Tags...), nil
}
func (m *splitMemStore) SetClipTags(_ context.Context, hash string, leaves []string) error {
	c, ok := m.clips[hash]
	if !ok {
		return fmt.Errorf("clip not found: %s", hash)
	}
	forest := taxonomy.New(taxonomy.SeedForest())
	c.AssertedTags = unionStrings(c.AssertedTags, leaves)
	c.Tags = nil
	for _, leaf := range c.AssertedTags {
		c.Tags = unionStrings(c.Tags, []string{leaf})
		c.Tags = unionStrings(c.Tags, forest.Ancestors(leaf))
	}
	c.Category = forest.PrimaryProductLeaf(c.AssertedTags)
	m.clips[hash] = c
	return nil
}

func unionStrings(left, right []string) []string {
	out := append([]string(nil), left...)
	seen := make(map[string]bool, len(out)+len(right))
	for _, value := range out {
		seen[value] = true
	}
	for _, value := range right {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
func (m *splitMemStore) ReplaceSplitChildren(_ context.Context, parentHash string, keepHashes []string, at time.Time) (int, error) {
	keep := make(map[string]bool, len(keepHashes))
	for _, hash := range keepHashes {
		keep[hash] = true
	}
	retired := 0
	for hash, c := range m.clips {
		if c.ParentHash != parentHash {
			continue
		}
		if keep[hash] {
			c.RemovedAt = time.Time{}
		} else if c.RemovedAt.IsZero() {
			c.RemovedAt = at
			retired++
		}
		m.clips[hash] = c
	}
	return retired, nil
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
func (m *splitMemStore) SetClipsHeld(_ context.Context, paths []string, held, _ bool, _ time.Time) (int, error) {
	wanted := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		wanted[path] = struct{}{}
	}
	updated := 0
	for hash, c := range m.clips {
		if _, ok := wanted[c.Path]; !ok {
			continue
		}
		c.Held = held
		m.clips[hash] = c
		updated++
	}
	return updated, nil
}
func (m *splitMemStore) UpsertSplitProposal(_ context.Context, p filler.SplitProposal) error {
	for id, existing := range m.proposals {
		if existing.ClipHash == p.ClipHash && id != p.ID {
			delete(m.proposals, id) // one proposal per clip, like the store's UNIQUE
		}
	}
	if m.roundTripProposals {
		p = durableProposalCopy(p)
	}
	m.proposals[p.ID] = p
	return nil
}

func durableProposalCopy(p filler.SplitProposal) filler.SplitProposal {
	copy := p
	if p.Segments != nil {
		raw, _ := json.Marshal(p.Segments)
		_ = json.Unmarshal(raw, &copy.Segments)
	}
	if p.Detection != nil {
		raw, _ := json.Marshal(p.Detection)
		var detection filler.SplitDetectionProgress
		_ = json.Unmarshal(raw, &detection)
		copy.Detection = &detection
	}
	return copy
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
func (m *splitMemStore) ListSplitProposals(context.Context) ([]filler.SplitProposal, error) {
	out := make([]filler.SplitProposal, 0, len(m.proposals))
	for _, p := range m.proposals {
		out = append(out, p)
	}
	return out, nil
}

// ⚠ REFUSES to insert, exactly as the real store does. A fake that happily created the row would
// hide the resurrection race this method exists to prevent — the class this repo has already been
// bitten by twice (a double that never refuses cannot catch a write-through-a-dead-handle bug).
func (m *splitMemStore) UpdateSplitProposal(_ context.Context, p filler.SplitProposal) error {
	_, ok := m.proposals[p.ID]
	if !ok {
		return fmt.Errorf("%w: %s", filler.ErrProposalGone, p.ID)
	}
	m.proposals[p.ID] = p
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

func TestSplitStage_LongBoundaryScanResumesFromDurableChunkAfterRestartAndTimeout(t *testing.T) {
	st := newSplitMemStore()
	duration := int64((21 * time.Minute) / time.Millisecond)
	hash := seedCompilation(st, "comps/three-hour-shape.mp4", duration)
	tools := &fakeTools{boundaryFn: func(ctx context.Context, startMs, endMs int64) ([]filler.Interval, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var gaps []filler.Interval
		for cut := startMs + 30_000; cut < endMs; cut += 30_000 {
			gaps = append(gaps, filler.Interval{StartMs: cut - 100, EndMs: cut + 100})
		}
		return gaps, nil
	}}
	clip := st.clips[hash]
	clip.IsComposite = true
	st.clips[hash] = clip
	newStage := func() *filler.SplitStage {
		// A fresh splitter/stage on every call simulates an application restart. The only state
		// allowed to carry progress is the proposal in the store.
		return filler.NewSplitStage(newSplitter(st, tools, nil, t.TempDir()), st)
	}

	if _, err := newStage().Run(context.Background(), clip); !errors.Is(err, filler.ErrDeferred) {
		t.Fatalf("first chunk = %v, want deliberate deferral", err)
	}
	props, _ := st.ListSplitProposals(context.Background())
	if len(props) != 1 || props[0].Ready() || props[0].Detection.ScannedThroughMs != 600_000 {
		t.Fatalf("first checkpoint = %+v, want a private draft through 10:00", props)
	}
	proposalID := props[0].ID

	expired, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newStage().Run(expired, clip); !errors.Is(err, context.Canceled) {
		t.Fatalf("expired second chunk = %v, want cancellation", err)
	}
	props, _ = st.ListSplitProposals(context.Background())
	if props[0].Detection.ScannedThroughMs != 600_000 {
		t.Fatalf("timeout moved checkpoint to %d; an unfinished span must repeat", props[0].Detection.ScannedThroughMs)
	}

	for i := 0; i < 2; i++ {
		if _, err := newStage().Run(context.Background(), clip); !errors.Is(err, filler.ErrDeferred) {
			t.Fatalf("resume pass %d = %v, want deferral", i+1, err)
		}
	}
	out, err := newStage().Run(context.Background(), clip)
	if err != nil || out.Verdict != filler.VerdictReview {
		t.Fatalf("final pass = (%+v, %v), want completed proposal awaiting policy", out, err)
	}

	props, _ = st.ListSplitProposals(context.Background())
	if len(props) != 1 || !props[0].Ready() || props[0].ID != proposalID {
		t.Fatalf("completed proposal = %+v, want same durable id and a reviewable cut list", props)
	}
	wantSpans := [][2]int64{
		{0, 600_000},
		{600_000, 1_200_000}, // canceled attempt
		{600_000, 1_200_000}, // resumed after restart
		{1_200_000, duration},
	}
	if !reflect.DeepEqual(tools.boundarySpans, wantSpans) {
		t.Fatalf("boundary spans = %v, want %v", tools.boundarySpans, wantSpans)
	}
	if tools.chapterCalls != 1 {
		t.Errorf("chapters checked %d times; draft resume repeated triage", tools.chapterCalls)
	}
}

// A checkpoint crosses JSON before the next pass. The detector's private source bitmask cannot be
// the only copy of boundary evidence, or a restart turns corroborated black+silence cuts into
// confidence 0 and makes the default auto-split threshold impossible to clear.
func TestSplitStage_BoundaryConfidenceSurvivesDetectionCheckpoint(t *testing.T) {
	st := newSplitMemStore()
	st.roundTripProposals = true
	hash := seedCompilation(st, "comps/corroborated.mp4", 65_000)
	clip := st.clips[hash]
	clip.IsComposite = true
	st.clips[hash] = clip
	tools := &fakeTools{
		blacks: []filler.Interval{
			{StartMs: 20_000, EndMs: 21_000},
			{StartMs: 40_000, EndMs: 41_000},
		},
		silences: []filler.Interval{
			{StartMs: 20_000, EndMs: 21_000},
			{StartMs: 40_000, EndMs: 41_000},
		},
	}
	newStage := func() *filler.SplitStage {
		return filler.NewSplitStage(newSplitter(st, tools, nil, t.TempDir()), st)
	}

	if _, err := newStage().Run(context.Background(), clip); !errors.Is(err, filler.ErrDeferred) {
		t.Fatalf("checkpoint pass = %v, want deliberate deferral", err)
	}
	out, err := newStage().Run(context.Background(), clip)
	if err != nil || out.Verdict != filler.VerdictReview {
		t.Fatalf("resume pass = (%+v, %v), want completed proposal awaiting policy", out, err)
	}
	props, err := st.ListSplitProposals(context.Background())
	if err != nil || len(props) != 1 || !props[0].Ready() {
		t.Fatalf("completed proposals = %+v, %v", props, err)
	}
	for i, seg := range props[0].Segments {
		if seg.BoundaryConfidence != 90 || seg.StartEvidence == "" || seg.EndEvidence == "" {
			t.Errorf("segment %d boundary = %d (%q / %q), want persisted corroborated evidence at 90",
				i, seg.BoundaryConfidence, seg.StartEvidence, seg.EndEvidence)
		}
	}
}

func TestSplitStage_UntitledChapterEvidenceSurvivesDetectionCheckpoint(t *testing.T) {
	st := newSplitMemStore()
	st.roundTripProposals = true
	hash := seedCompilation(st, "comps/untitled-chapter.mp4", 60_000)
	clip := st.clips[hash]
	clip.IsComposite = true
	st.clips[hash] = clip
	tools := &fakeTools{chapters: []filler.Chapter{{StartMs: 0, EndMs: 60_000}}}
	newStage := func() *filler.SplitStage {
		return filler.NewSplitStage(newSplitter(st, tools, nil, t.TempDir()), st)
	}

	if _, err := newStage().Run(context.Background(), clip); !errors.Is(err, filler.ErrDeferred) {
		t.Fatalf("checkpoint pass = %v, want deliberate deferral", err)
	}
	if _, err := newStage().Run(context.Background(), clip); err != nil {
		t.Fatalf("resume pass: %v", err)
	}
	props, _ := st.ListSplitProposals(context.Background())
	if len(props) != 1 || len(props[0].Segments) != 1 {
		t.Fatalf("completed proposals = %+v", props)
	}
	seg := props[0].Segments[0]
	if seg.BoundaryConfidence != 100 || seg.StartEvidence != "chapter" || seg.EndEvidence != "chapter" {
		t.Errorf("chapter boundary = %d (%q / %q), want durable chapter evidence at 100",
			seg.BoundaryConfidence, seg.StartEvidence, seg.EndEvidence)
	}
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

func TestPropose_CatalogFingerprintCacheSurvivesSplitterRestart(t *testing.T) {
	st := newSplitMemStore()
	firstHash := seedCompilation(st, "comps/first.mp4", 30_000)
	existing := filler.StoreClip{}
	existing.Hash = "hash-of-catalog-ad"
	existing.Path = "old/catalog-ad.mp4"
	existing.DurationMs = 30_000
	st.clips[existing.Hash] = existing

	pattern := make([]byte, 72)
	for i := range pattern {
		pattern[i] = byte(i)
	}
	tools := &fakeTools{
		chapters: []filler.Chapter{{StartMs: 0, EndMs: 30_000, Title: "advert"}},
		grayFrames: map[string][][]byte{
			key3("catalog-ad.mp4", 0, 30_000): {pattern},
			key3("first.mp4", 0, 30_000):      {pattern},
			key3("second.mp4", 0, 30_000):     {pattern},
		},
	}

	first := newSplitter(st, tools, nil, t.TempDir())
	p, err := first.Propose(context.Background(), firstHash)
	if err != nil {
		t.Fatal(err)
	}
	delete(st.proposals, p.ID)
	delete(st.clips, firstHash)

	secondHash := seedCompilation(st, "comps/second.mp4", 30_000)
	// A new Splitter represents a process restart: only the store is shared.
	second := newSplitter(st, tools, nil, t.TempDir())
	if _, err := second.Propose(context.Background(), secondHash); err != nil {
		t.Fatal(err)
	}

	catalogCall := key3("catalog-ad.mp4", 0, 30_000)
	calls := 0
	for _, call := range tools.grayCalls {
		if call == catalogCall {
			calls++
		}
	}
	if calls != 1 {
		t.Errorf("catalog media decoded %d times across two splitters, want one persisted-cache fill; calls=%v", calls, tools.grayCalls)
	}
}

func TestPropose_CatalogFingerprintCompletedAtDeadlineIsStillPersisted(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/deadline.mp4", 30_000)
	existing := filler.StoreClip{}
	existing.Hash = "hash-of-deadline-candidate"
	existing.Path = "old/deadline-candidate.mp4"
	existing.DurationMs = 30_000
	st.clips[existing.Hash] = existing

	pattern := make([]byte, 72)
	for i := range pattern {
		pattern[i] = byte(i)
	}
	ctx, cancel := context.WithCancel(context.Background())
	canceled := false
	tools := &fakeTools{
		chapters: []filler.Chapter{{StartMs: 0, EndMs: 30_000, Title: "advert"}},
		grayFrames: map[string][][]byte{
			key3("deadline-candidate.mp4", 0, 30_000): {pattern},
			key3("deadline.mp4", 0, 30_000):           {pattern},
		},
		grayHook: func(path string, _, _ int64) {
			// Simulate the pass expiring after ffmpeg produced the catalog frames but before the
			// cache write. The write must detach from this context or the completed work is lost.
			if path == "deadline-candidate.mp4" && !canceled {
				canceled = true
				cancel()
			}
		},
	}
	defer cancel()

	if _, err := newSplitter(st, tools, nil, t.TempDir()).Propose(ctx, hash); err != nil {
		t.Fatal(err)
	}
	if len(st.fingerprints) != 1 {
		t.Fatalf("completed catalog decode left %d cache rows, want 1", len(st.fingerprints))
	}
	if st.fingerprintWriteCtxErr != nil {
		t.Errorf("cache write inherited expired pass: %v", st.fingerprintWriteCtxErr)
	}
}

func TestPropose_CatalogFingerprintCacheFailureFallsBackToMedia(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/cache-failure.mp4", 30_000)
	existing := filler.StoreClip{}
	existing.Hash = "hash-of-cache-failure-candidate"
	existing.Path = "old/cache-failure-candidate.mp4"
	existing.DurationMs = 30_000
	st.clips[existing.Hash] = existing
	st.fingerprintReadErr = errors.New("cache read unavailable")
	st.fingerprintWriteErr = errors.New("cache write unavailable")

	pattern := make([]byte, 72)
	for i := range pattern {
		pattern[i] = byte(i)
	}
	tools := &fakeTools{
		chapters: []filler.Chapter{{StartMs: 0, EndMs: 30_000, Title: "advert"}},
		grayFrames: map[string][][]byte{
			key3("cache-failure-candidate.mp4", 0, 30_000): {pattern},
			key3("cache-failure.mp4", 0, 30_000):           {pattern},
		},
	}
	p, err := newSplitter(st, tools, nil, t.TempDir()).Propose(context.Background(), hash)
	if err != nil {
		t.Fatalf("derived-cache outage failed split detection: %v", err)
	}
	if len(p.Segments) != 1 || p.Segments[0].DupOf != existing.Path {
		t.Errorf("media fallback did not preserve duplicate detection: %+v", p.Segments)
	}
}

func TestSplitStage_DiscardsDuplicateAndShortCandidatesBeforeAutoConfirm(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/1987.mp4", 70_000)
	parent := st.clips[hash]
	parent.Held = true
	st.clips[hash] = parent
	drop := t.TempDir()
	if err := os.MkdirAll(filepath.Join(drop, "comps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(drop, "comps/1987.mp4"), []byte("compilation"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := filler.SplitProposal{
		ID: "sp_existing", ClipHash: hash, CreatedAt: time.Now(),
		Segments: []filler.SplitSegment{
			{Index: 0, StartMs: 0, EndMs: 30_000, Name: "already have it", DupOf: "old/ad.mp4"},
			{Index: 1, StartMs: 30_000, EndMs: 34_000, Name: "boundary fragment"},
			{Index: 2, StartMs: 34_000, EndMs: 70_000, Name: "new advert", Category: "toys", Looked: true,
				BoundaryConfidence: 100, StartEvidence: "reel edge", EndEvidence: "reel edge"},
		},
	}
	if err := st.UpsertSplitProposal(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	tools := &fakeTools{}
	stage := filler.NewSplitStage(newSplitter(st, tools, nil, drop), st).WithAutoConfirm(
		filler.AutoSplitPolicy{
			Enabled:       func() bool { return true },
			MinConfidence: func() int { return 85 },
			MaxDuration:   func() time.Duration { return 2 * time.Minute },
		},
		func() time.Duration { return 10 * time.Second },
	)

	out, err := stage.Run(context.Background(), st.clips[hash])
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Spawned) != 1 || len(tools.cutCalls) != 1 {
		t.Fatalf("spawned = %v, cuts = %v; deterministic rejects became clips", out.Spawned, tools.cutCalls)
	}
	if got := tools.cutCalls[0]; !strings.HasPrefix(got, "34000-70000→") {
		t.Errorf("cut = %q, want only the usable 34s-70s span", got)
	}
	if !st.clips[hash].IsComposite {
		t.Error("parent was not retained as a composite")
	}
	if st.clips[hash].Held {
		t.Error("fully auto-confirmed composite remained held and invisible in the catalog")
	}
	if _, ok := st.proposals[p.ID]; ok {
		t.Error("completed proposal still waits for approval")
	}
}

func TestSplitStage_AllDiscardedFinishesWithoutAnEmptyReview(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/repeats.mp4", 30_000)
	parent := st.clips[hash]
	parent.Held = true
	st.clips[hash] = parent
	p := filler.SplitProposal{
		ID: "sp_duplicates", ClipHash: hash, CreatedAt: time.Now(),
		Segments: []filler.SplitSegment{{StartMs: 0, EndMs: 30_000, DupOf: "old/ad.mp4"}},
	}
	if err := st.UpsertSplitProposal(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	stage := filler.NewSplitStage(newSplitter(st, &fakeTools{}, nil, t.TempDir()), st).
		WithAutoConfirm(filler.AutoSplitPolicy{Enabled: func() bool { return true }}, func() time.Duration { return 10 * time.Second })

	out, err := stage.Run(context.Background(), st.clips[hash])
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != filler.VerdictContinue || len(out.Spawned) != 0 {
		t.Fatalf("result = %+v", out)
	}
	if !st.clips[hash].IsComposite {
		t.Error("all-duplicate parent can still air")
	}
	if st.clips[hash].Held {
		t.Error("resolved all-duplicate parent remained held and invisible in the catalog")
	}
	if _, ok := st.proposals[p.ID]; ok {
		t.Error("an empty proposal was left in the review queue")
	}
}

func TestSplitStage_PersistsWhyEverySegmentNeedsReview(t *testing.T) {
	st := newSplitMemStore()
	hash := seedCompilation(st, "comps/unclassified.mp4", 30_000)
	p := filler.SplitProposal{
		ID: "sp_unclassified", ClipHash: hash, CreatedAt: time.Now(),
		Segments: []filler.SplitSegment{{
			Index: 0, StartMs: 0, EndMs: 30_000, Name: "unclassified part", Looked: true,
			BoundaryConfidence: 90, StartEvidence: "reel edge", EndEvidence: "black + silence",
		}},
	}
	if err := st.UpsertSplitProposal(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	stage := filler.NewSplitStage(newSplitter(st, &fakeTools{}, nil, t.TempDir()), st).WithAutoConfirm(
		filler.AutoSplitPolicy{
			Enabled:       func() bool { return true },
			MinConfidence: func() int { return 85 },
			MaxDuration:   func() time.Duration { return 2 * time.Minute },
		},
		func() time.Duration { return 10 * time.Second },
	)

	out, err := stage.Run(context.Background(), st.clips[hash])
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != filler.VerdictReview {
		t.Fatalf("verdict = %v, want review", out.Verdict)
	}
	persisted := st.proposals[p.ID]
	if got := persisted.Segments[0].HoldReason; got != string(filler.RejectUntagged) {
		t.Fatalf("persisted hold reason = %q, want %q", got, filler.RejectUntagged)
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
	for _, seg := range segments {
		if len(seg.AssertedTags) != 1 || seg.AssertedTags[0] != seg.Category {
			t.Errorf("confirmed segment %q taxonomy = category %q / asserted %v", seg.Name, seg.Category, seg.AssertedTags)
		}
	}
	// The cut files exist at cataloged paths (segments only; the composite keeps its own file).
	for _, seg := range segments {
		if _, err := os.Stat(filepath.Join(drop, seg.Path)); err != nil {
			t.Errorf("cataloged segment %s has no file: %v", seg.Path, err)
		}
	}
}

func TestConfirm_ReSplitReplacesOldChildrenOnlyWhenComplete(t *testing.T) {
	st := newSplitMemStore()
	parentHash := seedCompilation(st, "comps/resplit.mp4", 60_000)
	drop := t.TempDir()
	if err := os.MkdirAll(filepath.Join(drop, "comps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(drop, "comps", "resplit.mp4"), []byte("reel"), 0o644); err != nil {
		t.Fatal(err)
	}
	sp := newSplitter(st, &fakeTools{}, nil, drop)

	oldProposal := filler.SplitProposal{
		ID: "old", ClipHash: parentHash, CreatedAt: time.Now(),
		Segments: []filler.SplitSegment{
			{StartMs: 0, EndMs: 30_000, Name: "old one"},
			{StartMs: 30_000, EndMs: 60_000, Name: "old two"},
		},
	}
	if err := st.UpsertSplitProposal(context.Background(), oldProposal); err != nil {
		t.Fatal(err)
	}
	oldHashes, err := sp.Confirm(context.Background(), oldProposal.ID, oldProposal.Segments)
	if err != nil {
		t.Fatal(err)
	}

	newProposal := filler.SplitProposal{
		ID: "new", ClipHash: parentHash, CreatedAt: time.Now().Add(time.Second),
		Segments: []filler.SplitSegment{
			{StartMs: 0, EndMs: 20_000, Name: "new one"},
			{StartMs: 20_000, EndMs: 40_000, Name: "new two"},
			{StartMs: 40_000, EndMs: 60_000, Name: "new three"},
		},
	}
	if err := st.UpsertSplitProposal(context.Background(), newProposal); err != nil {
		t.Fatal(err)
	}
	parent := st.clips[parentHash]
	parent.Held = true
	st.clips[parentHash] = parent
	first, err := sp.ConfirmSome(context.Background(), newProposal.ID, newProposal.Segments[:1], newProposal.Segments[1:])
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || st.clips[first[0]].RemovedAt.IsZero() {
		t.Fatalf("partial re-split child = %v / %+v, want a tombstone until the generation is complete", first, st.clips[first[0]])
	}
	if !st.clips[parentHash].Held {
		t.Error("partial confirmation filed the parent before its proposal was resolved")
	}
	for _, hash := range oldHashes {
		if !st.clips[hash].RemovedAt.IsZero() {
			t.Errorf("old generation %s retired before the replacement was complete", hash)
		}
	}
	persisted := st.proposals[newProposal.ID]
	if len(persisted.Spawned) != 1 || persisted.Spawned[0] != first[0] {
		t.Fatalf("proposal spawned state = %v, want first partial child", persisted.Spawned)
	}

	last, err := sp.Confirm(context.Background(), newProposal.ID, persisted.Segments)
	if err != nil {
		t.Fatal(err)
	}
	for _, hash := range oldHashes {
		if st.clips[hash].RemovedAt.IsZero() {
			t.Errorf("superseded old child %s remained airable after final confirm", hash)
		}
	}
	for _, hash := range append(first, last...) {
		if !st.clips[hash].RemovedAt.IsZero() {
			t.Errorf("replacement child %s stayed tombstoned after final confirm", hash)
		}
	}
	if _, ok := st.proposals[newProposal.ID]; ok {
		t.Error("completed re-split proposal survived final confirm")
	}
	if st.clips[parentHash].Held {
		t.Error("final re-split confirmation left the parent held")
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
