package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// The operator's decision has to reach the pipeline row, not only `clips` (§10 V54). Incoming no
// longer exposes review/refusal audit, so the durable row is the direct regression seam here.

// seedForDecision puts a held clip with a pipeline row waiting on a person.
func seedForDecision(t *testing.T, st interface {
	UpsertClipPipeline(context.Context, filler.ClipPipeline) error
}, put func(), hash string, d filler.Disposition) {
	t.Helper()
	put()
	if err := st.UpsertClipPipeline(context.Background(), filler.ClipPipeline{
		ClipHash: hash, Stage: filler.StageScore, Status: filler.StatusDone,
		Disposition: d, UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

// "Don't use it" wrote `removed_at` and nothing else. `GetClip` carries no `removed_at` predicate,
// so the belt's fallback loop re-resolved the clip and put it straight back on the queue.
func TestBulkRemoveFiller_DismissalTakesTheClipOffTheBelt(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st := harness.Server, harness.Store
	const hash = "hash-dismissed"
	seedForDecision(t, st, func() {
		putClip(t, st, filler.Clip{
			Hash: hash, Path: "1985/dismissed.mp4", Name: "Dismissed", Kind: filler.Commercial,
			DurationMs: 30_000, Held: true,
		})
	}, hash, filler.DispositionReview)

	if res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/bulk/remove",
		`{"hashes":["`+hash+`"]}`, adminToken); res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}

	row, found, err := st.GetClipPipeline(context.Background(), hash)
	if err != nil || !found {
		t.Fatalf("pipeline row: found=%v err=%v", found, err)
	}
	if row.Disposition != filler.DispositionDismissed {
		t.Errorf("disposition = %q, want dismissed", row.Disposition)
	}
}

// Restore is ONE endpoint and has to undo BOTH halves — the tombstone and the refusal — for a
// dismissal exactly as it already did for a machine rejection.
func TestBulkRemoveFiller_RestoreUndoesADismissal(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st := harness.Server, harness.Store
	const hash = "hash-restored"
	seedForDecision(t, st, func() {
		putClip(t, st, filler.Clip{
			Hash: hash, Path: "1985/restored.mp4", Name: "Restored", Kind: filler.Commercial,
			DurationMs: 30_000, Held: true,
		})
	}, hash, filler.DispositionDismissed)

	if res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/bulk/remove",
		`{"hashes":["`+hash+`"],"restore":true}`, adminToken); res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}

	row, found, err := st.GetClipPipeline(context.Background(), hash)
	if err != nil || !found {
		t.Fatalf("pipeline row: found=%v err=%v", found, err)
	}
	if row.Disposition != filler.DispositionReview {
		t.Errorf("disposition = %q, want review — a restored clip is waiting on a person again", row.Disposition)
	}
}
