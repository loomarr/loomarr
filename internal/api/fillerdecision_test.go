package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// The operator's decision has to reach the PIPELINE ROW, not only `clips` (§10 V54).
//
// ⚠ These assert through `GET /v1/filler/incoming` rather than by reading the row back, and that
// is deliberate. The defect was never "a column holds the wrong string" — it was "the clip I just
// decided on is still sitting in my queue", and the queue is the only thing that can say so. A
// test that read the disposition directly would have gone green on a fix that left the belt's
// fallback loop re-resolving the clip anyway.

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
	srv, st, _ := newFillerServer(t)
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

	_, body := getIncoming(t, srv.URL+"/v1/filler/incoming", adminToken)
	for _, c := range body.Clips {
		if c.Hash == hash {
			t.Error("a dismissed clip is still on the conveyor — the fallback loop re-resolved it")
		}
	}
	// ⚠ And NOT on the refusals list either: that is the audit of what Loomarr decided WITHOUT the
	// operator, and a dismissal is what the operator decided themselves.
	for _, r := range body.Rejected {
		if r.Hash == hash {
			t.Error("an operator dismissal appeared under 'Loomarr didn't use these' — that list is the machine's")
		}
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
	srv, st, _ := newFillerServer(t)
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
