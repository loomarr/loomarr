package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
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
			Stage       string `json:"stage"`
			Status      string `json:"status"`
			Progress    int    `json:"progress"`
			DurationMs  int64  `json:"durationMs"`
			Thumbnail   string `json:"thumbnail"`
			Lifecycle   string `json:"lifecycle"`
			FailureCode string `json:"failureCode"`
			Recovery    string `json:"recovery"`
			RetryStage  string `json:"retryStage"`
			Stages      []struct {
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
		Hash        string `json:"hash"`
		Reason      string `json:"reason"`
		Restorable  bool   `json:"restorable"`
		FailureCode string `json:"failureCode"`
		Recovery    string `json:"recovery"`
		RetryStage  string `json:"retryStage"`
	} `json:"rejected"`
	Overview struct {
		Runnable, InProgress, Scheduled, NeedsDecision, Admitted, Rejected, Dismissed int
	} `json:"overview"`
	StageOrder     []string `json:"stageOrder"`
	ClipsTotal     int      `json:"clipsTotal"`
	DecisionsTotal int      `json:"decisionsTotal"`
	ReelsTotal     int      `json:"reelsTotal"`
	Total          int      `json:"total"`
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
			{Index: 3, StartMs: 149_000, EndMs: 179_000, Name: "unknown", Looked: true},
		},
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Reels) != 1 {
		t.Fatalf("reels = %d, want 1", len(body.Reels))
	}
	if body.Reels[0].Segments != 4 {
		t.Errorf("segments = %d, want 4", body.Reels[0].Segments)
	}
	if body.Reels[0].NeedsAttention != 3 {
		t.Errorf("needsAttention = %d, want 3 (legacy duplicate, unsplittable, and examined-but-unclassified)", body.Reels[0].NeedsAttention)
	}
	// ⚠ No clip backs this proposal, so the name falls back to the identity rather than rendering
	// blank. A reel whose compilation has been deleted is a real state an operator must still be
	// able to see and dismiss.
	if body.Reels[0].ClipName != "hash-of-comps/1987.mp4" {
		t.Errorf("clipName = %q, want the hash as the fallback for a missing clip", body.Reels[0].ClipName)
	}
}

func TestFillerIncoming_HidesBoundaryDetectionCheckpointFromReview(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	if err := st.UpsertSplitProposal(context.Background(), filler.SplitProposal{
		ID: "sp_detecting", ClipHash: "long-reel", CreatedAt: time.Now().UTC(),
		Detection: &filler.SplitDetectionProgress{ScannedThroughMs: 600_000},
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)
	if len(body.Reels) != 0 {
		t.Fatalf("detector checkpoint appeared as work owed by the operator: %+v", body.Reels)
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

func TestFillerIncoming_CapsRowsButKeepsTheFullTotals(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	for i := 0; i < incomingListLimitForTest+7; i++ {
		path := fmt.Sprintf("incoming-%03d.mp4", i)
		putClip(t, st, filler.Clip{
			Path: path, Name: path, Kind: filler.Commercial, DurationMs: 30_000, Held: true,
		})
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)
	if len(body.Clips) != incomingListLimitForTest {
		t.Fatalf("clips = %d, want bounded page of %d", len(body.Clips), incomingListLimitForTest)
	}
	if body.ClipsTotal != incomingListLimitForTest+7 {
		t.Errorf("clipsTotal = %d, want all %d conveyor clips", body.ClipsTotal, incomingListLimitForTest+7)
	}
	if body.Total != incomingListLimitForTest+7 {
		t.Errorf("total = %d, want all decisions rather than the page length", body.Total)
	}
	if body.DecisionsTotal != incomingListLimitForTest+7 {
		t.Errorf("decisionsTotal = %d, want all decisions", body.DecisionsTotal)
	}
}

const incomingListLimitForTest = 100

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
		//
		// ⚠ Seeded here because this test is about SERIALIZATION, and that is all it proves. It was
		// for a long time the only artefact mentioning -1, and it stayed green throughout the period
		// production could not emit one at all: `onProgress` dropped the sentinel on a blanket
		// `percent < 0` guard, so `tag` and `vision` rendered a bar frozen at zero on every run.
		// That the runner actually PRODUCES -1 is pinned in
		// internal/filler/pipelineprogress_test.go, which drives the real pipeline (V54 A6).
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
	if got.Pipeline.Lifecycle != "in_progress" || body.Overview.InProgress != 1 {
		t.Errorf("lifecycle/overview = %q / %+v, want in_progress and one active", got.Pipeline.Lifecycle, body.Overview)
	}
	// ⚠ The VISITED ladder, including the skip and its reason. A stage that silently did not
	// happen reads as broken, so the note is the half that makes the skip legible.
	if len(got.Pipeline.Stages) != 2 || got.Pipeline.Stages[1].Status != "skipped" || got.Pipeline.Stages[1].Note == "" {
		t.Errorf("stages = %+v, want probe/done then transcribe/skipped with its note", got.Pipeline.Stages)
	}
}

func TestFillerIncoming_ExecutionFailureCarriesSafeRecovery(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{Hash: "failed", Path: "failed.mp4", Name: "Failed", Kind: filler.Commercial})
	if err := st.UpsertClipPipeline(context.Background(), filler.ClipPipeline{
		ClipHash: "failed", Stage: filler.StageTranscode, Status: filler.StatusFailed,
		Attempts: filler.MaxAttempts, Disposition: filler.DispositionRejected,
		RejectReason: filler.ReasonUnplayable, RejectDetail: "ffmpeg exited 1",
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)
	if len(body.Rejected) != 1 {
		t.Fatalf("rejected = %+v, want one", body.Rejected)
	}
	got := body.Rejected[0]
	if got.FailureCode != "unplayable" || got.Recovery != "retry" || got.RetryStage != "transcode" || got.Restorable {
		t.Fatalf("recovery = %+v, want retry/transcode without content override", got)
	}
	if body.Overview.Rejected != 1 {
		t.Fatalf("overview = %+v, want one terminal rejection", body.Overview)
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
	now := time.Now().UTC()
	if err := st.UpsertClipPipeline(context.Background(), filler.ClipPipeline{
		ClipHash: "hash-review", Stage: filler.StageScore, Status: filler.StatusDone,
		Disposition: filler.DispositionReview, UpdatedAt: now,
		Stages: []filler.StageRecord{{
			Stage: filler.StageScore, Status: filler.StatusDone,
			Note: "The picture is unchanged for 12.0s; this may be intentional.", At: now,
		}},
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
	if body.Clips[0].Reason != "The picture is unchanged for 12.0s; this may be intentional." {
		t.Errorf("review reason = %q, want the stage's measured explanation", body.Clips[0].Reason)
	}
}

// ⚠ **A compilation appears ONCE, as a reel — never also as a taggable ask** (§10 V51e, V54).
//
// It appeared twice: as a "needs a decision" card carrying "Loomarr couldn't work out what this
// is", and as a reel below. Both halves were individually correct — the pipeline deliberately
// skips tag/vision for a composite ("a compilation is cut up rather than filed"), so `askReasonFor`
// truthfully reported it as unidentified — which is exactly why no existing test caught it. The
// tab was asking an operator to tag a container of adverts it never intended to file.
func TestFillerIncoming_ACompilationIsAReelNotAnAsk(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	putClip(t, st, filler.Clip{
		Hash: "hash-reel", Path: "comps/reel.mp4", Name: "WTTV-4 Commercial Breaks(5/18/1987)",
		Kind: filler.Commercial, DurationMs: 1_180_000, IsComposite: true,
	})
	// ⚠ Parked at split/review — the state ~50 live reels were in.
	if err := st.UpsertClipPipeline(ctx, filler.ClipPipeline{
		ClipHash: "hash-reel", Stage: filler.StageSplit, Status: filler.StatusDone,
		Disposition: filler.DispositionReview, UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSplitProposal(ctx, filler.SplitProposal{
		ID: "sp_reel", ClipHash: "hash-reel", CreatedAt: time.Now().UTC(),
		Segments: []filler.SplitSegment{{Index: 0, StartMs: 0, EndMs: 30_000}},
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Clips) != 0 {
		t.Errorf("clips = %d, want 0 — the reel below IS the handoff; %+v", len(body.Clips), body.Clips)
	}
	if len(body.Reels) != 1 {
		t.Fatalf("reels = %d, want 1", len(body.Reels))
	}
	if body.Reels[0].ClipName != "WTTV-4 Commercial Breaks(5/18/1987)" {
		t.Errorf("reel name = %q, want the compilation's name", body.Reels[0].ClipName)
	}
	// The badge counted the same reel twice, once per half.
	if body.Total != 1 {
		t.Errorf("total = %d, want 1 — a reel must not be counted as both an ask and a reel", body.Total)
	}
}

// ⚠ The other half, and the one a blunt "exclude every composite from the belt" fix would break:
// a compilation still BEING DETECTED has no proposal yet, and detection runs minutes per file. It
// must show as a preparing row — V51e exists so that "nothing is happening" is never the answer —
// but still not as a decision.
func TestFillerIncoming_ACompilationBeingDetectedStillShows(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{
		Hash: "hash-detecting", Path: "comps/detecting.mp4", Name: "TBS Commercial Breaks(12/15/1989)",
		Kind: filler.Commercial, DurationMs: 1_180_000, IsComposite: true,
	})
	if err := st.UpsertClipPipeline(context.Background(), filler.ClipPipeline{
		ClipHash: "hash-detecting", Stage: filler.StageSplit, Status: filler.StatusRunning,
		Disposition: filler.DispositionRunning, UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Clips) != 1 {
		t.Fatalf("clips = %d, want 1 — a compilation mid-detection must stay visible", len(body.Clips))
	}
	if body.Clips[0].NeedsDecision {
		t.Error("a compilation mid-detection is asking for nothing; it is being worked on")
	}
	if body.Total != 0 {
		t.Errorf("total = %d, want 0 — nothing here needs a human yet", body.Total)
	}
}

// A detection checkpoint is private pipeline state, not a reviewable reel. The list side already
// filters these drafts out of `reels` and leaves their compilation on the conveyor; the server-side
// total must apply the same readiness rule or the UI renders one preparing card beneath a zero
// count. This is the production shape between the first boundary-scan pass and its resume.
func TestFillerIncoming_ACompilationDetectionCheckpointCountsOnTheConveyor(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	putClip(t, st, filler.Clip{
		Hash: "hash-checkpoint", Path: "comps/checkpoint.mp4", Name: "Commercial Break Checkpoint",
		Kind: filler.Commercial, DurationMs: 1_180_000, IsComposite: true,
	})
	if err := st.UpsertClipPipeline(ctx, filler.ClipPipeline{
		ClipHash: "hash-checkpoint", Stage: filler.StageSplit, Status: filler.StatusQueued,
		Disposition: filler.DispositionRunning, UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSplitProposal(ctx, filler.SplitProposal{
		ID: "sp_checkpoint", ClipHash: "hash-checkpoint", CreatedAt: time.Now().UTC(),
		Detection: &filler.SplitDetectionProgress{ScannedThroughMs: 600_000},
	}); err != nil {
		t.Fatal(err)
	}

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)

	if len(body.Clips) != 1 {
		t.Fatalf("clips = %d, want one compilation still being detected", len(body.Clips))
	}
	if body.ClipsTotal != 1 {
		t.Errorf("clipsTotal = %d, want 1 — a draft proposal does not take its clip off the conveyor", body.ClipsTotal)
	}
	if len(body.Reels) != 0 || body.ReelsTotal != 0 {
		t.Errorf("reels = %d total %d, want no reviewable reel for a detection checkpoint",
			len(body.Reels), body.ReelsTotal)
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
