package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/store"
)

type incomingBody struct {
	// The conveyor: being-prepared and needs-a-decision in ONE list (§10 V51e).
	Clips []struct {
		// Hash is the clip's IDENTITY, and it is decoded here rather than matched by path because
		// V54's decision tests deliberately give a clip a hash that is not its path (§10 V54).
		Hash          string `json:"hash"`
		Path          string `json:"path"`
		Name          string `json:"name"`
		SuggestedEra  int    `json:"suggestedEra"`
		Reason        string `json:"reason"`
		NeedsDecision bool   `json:"needsDecision"`
		Pipeline      *struct {
			Stage      string `json:"stage"`
			Status     string `json:"status"`
			Progress   int    `json:"progress"`
			DurationMs int64  `json:"durationMs"`
			Thumbnail  string `json:"thumbnail"`
			Stages     []struct {
				Stage  string `json:"stage"`
				Status string `json:"status"`
				Note   string `json:"note"`
			} `json:"stages"`
		} `json:"pipeline"`
	} `json:"clips"`
	Reels []struct {
		ProposalID     string `json:"proposalId"`
		ClipHash       string `json:"clipHash"`
		ClipName       string `json:"clipName"`
		Segments       int    `json:"segments"`
		NeedsAttention int    `json:"needsAttention"`
	} `json:"reels"`
	// Rejected is the refusals audit — what Loomarr decided WITHOUT the operator. An operator's own
	// dismissal must not appear here (§10 V54).
	Rejected []struct {
		Hash       string `json:"hash"`
		Reason     string `json:"reason"`
		Restorable bool   `json:"restorable"`
	} `json:"rejected"`
	StageOrder []string `json:"stageOrder"`
	Total      int      `json:"total"`
}

func getIncoming(t *testing.T, url, token string) (*http.Response, incomingBody) {
	t.Helper()
	res := sourceReq(t, http.MethodGet, url, "", token)
	var b incomingBody
	if res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&b); err != nil {
			t.Fatal(err)
		}
	}
	return res, b
}

// clipHashFor derives a clip's test identity from its path — DISTINCT from the path, never
// equal to it.
//
// ⚠ **This used to be `c.Hash = c.Path`, and that one line made every test in this package
// blind to hash/path confusion.** Identity has been the content HASH since V38c (§10), so a
// double that indexes by path stands in for a store that indexes by hash and cannot see the
// difference — every lookup that passed a path where a hash was wanted still hit. It shipped:
// `asSuggested` called `GetClip(ctx, path)` against a hash-keyed reader and silently confirmed
// nothing (fixed in #248). The comment above this helper explained why the shortcut was
// convenient and never asked what it stopped the tests from catching.
//
// sha256 of the path: deterministic (same fixture, same id, every run), 64 hex characters like
// the real thing, and provably not the path. Deliberately NOT the `hash-of-<path>` literal used
// elsewhere in this file — that spelling is reserved for a hash NO clip matches, which is how
// the missing-compilation fallback is tested. A test that needs a specific id still sets `Hash`
// explicitly; only the default is derived.
func clipHashFor(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}

func putClip(t *testing.T, st store.Store, c filler.Clip) {
	t.Helper()
	if c.Hash == "" {
		c.Hash = clipHashFor(c.Path)
	}
	if err := st.UpsertClip(context.Background(), store.Clip{Clip: c, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
}

// The two asks are different QUESTIONS, and the queue must not collapse them: an ungrounded era
// has a proposed answer to confirm, while an untagged commercial has nothing to confirm.
func TestFillerIncoming_QueuesTheTwoAskKindsSeparately(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	// ⚠ HELD, because V38 made waiting a STATE rather than something inferred from missing tags.
	// A clip that is not held is in the catalog and is nobody's decision, however it is tagged.
	putClip(t, st, filler.Clip{
		Path: "guess.mp4", Name: "guess.mp4", Kind: filler.Commercial, DurationMs: 30_000,
		Era: 0, Audience: filler.Kids, Category: "toys", SuggestedEra: 1988, Held: true,
	})
	putClip(t, st, filler.Clip{
		Path: "mystery.mp4", Name: "mystery.mp4", Kind: filler.Commercial, DurationMs: 25_000,
		Held: true,
	})
	// Filed and fully tagged: not an ask.
	putClip(t, st, filler.Clip{
		Path: "known.mp4", Name: "known.mp4", Kind: filler.Commercial, DurationMs: 30_000,
		Era: 1992, Audience: filler.Kids, Category: "cereal",
	})

	res, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if len(body.Clips) != 2 {
		t.Fatalf("clips = %d, want 2 — a fully tagged clip is not waiting on anyone", len(body.Clips))
	}
	// An ungrounded era sorts first: it is one click, where an untagged clip needs everything.
	if body.Clips[0].Path != "guess.mp4" || body.Clips[0].SuggestedEra != 1988 {
		t.Errorf("clips[0] = %+v, want the ungrounded-era clip first", body.Clips[0])
	}
	if body.Clips[0].Reason == body.Clips[1].Reason {
		t.Error("both asks carry the same reason — the two questions have been collapsed into one")
	}
}

// ⚠ Bumpers and station IDs do their bookend job with no era/audience/category, so queueing them
// would fill the review with work that changes nothing. Same rule the AI-tagging job applies.
func TestFillerIncoming_DoesNotQueueBumpers(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{Path: "bump.mp4", Name: "bump.mp4", Kind: filler.Bumper, DurationMs: 5_000})
	putClip(t, st, filler.Clip{Path: "ident.mp4", Name: "ident.mp4", Kind: filler.StationID, DurationMs: 4_000})

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Clips) != 0 {
		t.Errorf("queued %d bookend clips: %+v", len(body.Clips), body.Clips)
	}
}

// A reel of twelve clean segments and a reel with three problems are different amounts of work,
// and the queue should say which is which before it is opened.
func TestFillerIncoming_CountsSegmentsNeedingAttention(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	if err := st.UpsertSplitProposal(context.Background(), filler.SplitProposal{
		ID: "sp_1", ClipHash: "hash-of-comps/1987.mp4", CreatedAt: time.Now().UTC(),
		Segments: []filler.SplitSegment{
			{Index: 0, StartMs: 0, EndMs: 30_000, Name: "clean"},
			{Index: 1, StartMs: 30_000, EndMs: 61_000, Name: "dup", DupOf: "old/ad.mp4"},
			{Index: 2, StartMs: 61_000, EndMs: 149_000, Name: "stuck", Unsplittable: true},
		},
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Reels) != 1 {
		t.Fatalf("reels = %d, want 1", len(body.Reels))
	}
	if body.Reels[0].Segments != 3 {
		t.Errorf("segments = %d, want 3", body.Reels[0].Segments)
	}
	if body.Reels[0].NeedsAttention != 2 {
		t.Errorf("needsAttention = %d, want 2 (a duplicate and an unsplittable stretch)", body.Reels[0].NeedsAttention)
	}
	// ⚠ No clip backs this proposal, so the name falls back to the identity rather than rendering
	// blank. A reel whose compilation has been deleted is a real state an operator must still be
	// able to see and dismiss.
	if body.Reels[0].ClipName != "hash-of-comps/1987.mp4" {
		t.Errorf("clipName = %q, want the hash as the fallback for a missing clip", body.Reels[0].ClipName)
	}
}

// The reel row is titled with the compilation's NAME. ⚠ Its identity is a 64-character hash, which
// is not a row title — and the field it replaced (`clipPath`) only looked acceptable because the
// fixtures used friendly filenames no real catalog contains: a filed clip's path is
// `a3/f9/<hash>.mp4`.
func TestFillerIncoming_ReelCarriesTheCompilationName(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{
		Hash: "a3f9deadbeef", Path: "a3/f9/a3f9deadbeef.mp4",
		Name: "1987 Saturday morning block", Kind: filler.Commercial, DurationMs: 149_000,
	})
	if err := st.UpsertSplitProposal(context.Background(), filler.SplitProposal{
		ID: "sp_1", ClipHash: "a3f9deadbeef", CreatedAt: time.Now().UTC(),
		Segments: []filler.SplitSegment{{Index: 0, StartMs: 0, EndMs: 30_000, Name: "clean"}},
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Reels) != 1 {
		t.Fatalf("reels = %d, want 1", len(body.Reels))
	}
	if body.Reels[0].ClipName != "1987 Saturday morning block" {
		t.Errorf("clipName = %q, want the catalog name", body.Reels[0].ClipName)
	}
	if body.Reels[0].ClipHash != "a3f9deadbeef" {
		t.Errorf("clipHash = %q — the identity must still be carried alongside the name",
			body.Reels[0].ClipHash)
	}
}

// Both halves in ONE read. A client fetching them separately renders a half-empty queue whenever
// one call is slower — which is exactly when the queue matters.
func TestFillerIncoming_TotalCoversBothHalves(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{Path: "mystery.mp4", Name: "mystery.mp4", Kind: filler.Commercial, DurationMs: 25_000, Held: true})
	if err := st.UpsertSplitProposal(context.Background(), filler.SplitProposal{
		ID: "sp_1", ClipHash: "hash-of-comps/a.mp4", CreatedAt: time.Now().UTC(),
		Segments: []filler.SplitSegment{{Index: 0, StartMs: 0, EndMs: 30_000, Name: "a"}},
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if body.Total != 2 {
		t.Errorf("total = %d, want 2 — it must cover asks AND reels", body.Total)
	}
}

// Empty is an array, never null: a null makes every consumer guard before iterating, and "nothing
// needs you" is a real answer the tab renders as its all-clear state.
func TestFillerIncoming_EmptyHalvesAreArrays(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	res := sourceReq(t, http.MethodGet, srv.URL+"/v1/filler/incoming", "", adminToken)
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"clips", "reels"} {
		if got := string(raw[k]); got != "[]" {
			t.Errorf("%s = %s, want []", k, got)
		}
	}
}

// A pipeline row describes a CLIP, not just a position (§10 V51e).
//
// ⚠ The pipeline half of this response shipped in V51b with no API-level test of its contents at
// all, which is the same shape as the defect V51a found in `clips.confidence`: a field can be
// declared, typed, serialised and completely empty, and the only symptom is a UI that renders
// nothing where something should be. The row is drawn as a clip card — thumbnail, name,
// duration — so all three are asserted against a clip whose values are deliberately distinct
// from each other and from its hash.
func TestFillerIncoming_PipelineRowCarriesTheClipItDescribes(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{
		Hash: "hash-cola", Path: "1985/cola.mp4", Name: "Coca-Cola 1985",
		DurationMs: 31_000, Thumbnail: "1985/cola.jpg", Kind: filler.Commercial,
	})
	if err := st.UpsertClipPipeline(context.Background(), filler.ClipPipeline{
		ClipHash: "hash-cola", Stage: filler.StageTag, Status: filler.StatusRunning,
		// -1 is the "this rung cannot measure itself" sentinel, and it must survive the wire as
		// -1 rather than being flattened to 0 by an `omitempty` someone adds later.
		Progress: -1, Disposition: filler.DispositionRunning,
		Stages: []filler.StageRecord{
			{Stage: filler.StageProbe, Status: filler.StatusDone, At: time.Now().UTC()},
			{Stage: filler.StageTranscribe, Status: filler.StatusSkipped, Note: "the description already says enough", At: time.Now().UTC()},
		},
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Clips) != 1 {
		t.Fatalf("conveyor has %d rows, want 1", len(body.Clips))
	}
	got := body.Clips[0]
	if got.Name != "Coca-Cola 1985" {
		t.Errorf("name = %q, want the clip's own", got.Name)
	}
	// ⚠ A clip the machine is still working on does NOT need a decision. This is the assertion the
	// whole merge exists for: before it, the same clip appeared in an `asks` list demanding one.
	if got.NeedsDecision {
		t.Error("a clip mid-pipeline is marked as needing a decision — it is waiting on the machine")
	}
	if got.Pipeline == nil {
		t.Fatal("no pipeline block on a clip that has a pipeline row")
	}
	if got.Pipeline.Stage != "tag" || got.Pipeline.Status != "running" || got.Pipeline.Progress != -1 {
		t.Errorf("pipeline = {stage %q, status %q, progress %d}, want {tag, running, -1}",
			got.Pipeline.Stage, got.Pipeline.Status, got.Pipeline.Progress)
	}
	// ⚠ The VISITED ladder, including the skip and its reason. A stage that silently did not
	// happen reads as broken, so the note is the half that makes the skip legible.
	if len(got.Pipeline.Stages) != 2 || got.Pipeline.Stages[1].Status != "skipped" || got.Pipeline.Stages[1].Note == "" {
		t.Errorf("stages = %+v, want probe/done then transcribe/skipped with its note", got.Pipeline.Stages)
	}
}

// ⚠ THE regression this phase exists to prevent: one clip, one row, one place on the belt.
//
// Before the merge a held clip mid-pipeline satisfied BOTH the `asks` query (held and untagged)
// and the `pipeline` query (non-terminal), so it rendered twice — once demanding a decision, once
// captioned "nothing here needs you". On a fresh scan that was 84 of 85 clips, because the runner
// enrols everything at `probe/queued` and a freshly scanned clip is held and untagged by
// definition. Nothing in the old shape could catch it: both lists were individually correct.
func TestFillerIncoming_AClipAppearsExactlyOnce(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{
		Hash: "hash-dup", Path: "dup.mp4", Name: "Held and enrolled",
		Kind: filler.Commercial, DurationMs: 30_000, Held: true,
	})
	if err := st.UpsertClipPipeline(context.Background(), filler.ClipPipeline{
		ClipHash: "hash-dup", Stage: filler.StageProbe, Status: filler.StatusQueued,
		Disposition: filler.DispositionRunning, UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Clips) != 1 {
		t.Fatalf("clips = %d, want 1 — held AND enrolled is one clip, not two rows", len(body.Clips))
	}
	if body.Clips[0].NeedsDecision {
		t.Error("held-and-enrolled reported as needing a decision; the machine has not finished")
	}
	// ⚠ And the badge agrees with the list. `total` counted `len(asks)` before, which included
	// clips the machine still owned — a number that disagreed with the rows underneath it.
	if body.Total != 0 {
		t.Errorf("total = %d, want 0 — nothing is waiting on a person yet", body.Total)
	}
}

// The other end of the belt: once the pipeline hands a clip over, it becomes the operator's.
func TestFillerIncoming_ReviewDispositionIsWhatAsksForADecision(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{
		Hash: "hash-review", Path: "review.mp4", Name: "Machine gave up",
		Kind: filler.Commercial, DurationMs: 30_000, Held: true,
	})
	if err := st.UpsertClipPipeline(context.Background(), filler.ClipPipeline{
		ClipHash: "hash-review", Stage: filler.StageScore, Status: filler.StatusDone,
		Disposition: filler.DispositionReview, UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Clips) != 1 {
		t.Fatalf("clips = %d, want 1", len(body.Clips))
	}
	if !body.Clips[0].NeedsDecision {
		t.Error("a clip at `review` is not marked as needing a decision — review IS the handoff")
	}
	if body.Total != 1 {
		t.Errorf("total = %d, want 1", body.Total)
	}
}

// ⚠ A clip catalogued BEFORE V51b has no pipeline row at all. Treating "no row" as "still being
// prepared" would strand it in a section that says nothing needs the operator, permanently.
func TestFillerIncoming_AClipWithNoPipelineRowStillAsks(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{
		Path: "legacy.mp4", Name: "legacy.mp4", Kind: filler.Commercial,
		DurationMs: 30_000, Held: true,
	})

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Clips) != 1 || !body.Clips[0].NeedsDecision {
		t.Fatalf("clips = %+v, want one clip needing a decision via the legacy fallback", body.Clips)
	}
	if body.Clips[0].Pipeline != nil {
		t.Error("invented a pipeline block for a clip that has no row")
	}
}

// The ladder the UI draws is the ladder the runner walks (§10 V51e).
//
// ⚠ This is a DRIFT guard, not a shape check, which is why it compares against
// `filler.StageOrder` itself rather than a literal list of eight ids. A literal here would be the
// second copy of the sequence the field exists to avoid: adding a rung to the pipeline would leave
// this test red for the wrong reason, and the obvious fix — editing the literal — is exactly the
// edit that hides the bug. The strip renders one pip per element, so an omission or a reordering
// is a pipeline the operator is shown that the machine does not run.
func TestFillerIncoming_ServesTheWholeStageLadderInRunOrder(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.StageOrder) != len(filler.StageOrder) {
		t.Fatalf("stageOrder = %v (%d stages), want %d", body.StageOrder, len(body.StageOrder), len(filler.StageOrder))
	}
	for i, want := range filler.StageOrder {
		if body.StageOrder[i] != string(want) {
			t.Errorf("stageOrder[%d] = %q, want %q", i, body.StageOrder[i], want)
		}
	}
}

// §19 negative: the queue names filesystem paths and drives what gets filed.
func TestFillerIncoming_RequiresAdmin(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	if res, _ := getIncoming(t, srv.URL+"/v1/filler/incoming", ""); res.StatusCode == http.StatusOK {
		t.Error("served the incoming queue with no credential")
	}
}
