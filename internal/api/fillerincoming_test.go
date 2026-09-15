package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

type calmIncomingBody struct {
	Preparing struct {
		Rows []struct {
			ClipHash, Name, StatusLabel string
			Technical                   struct {
				Stages []struct{ Label, Status string } `json:"stages"`
			} `json:"technical"`
		} `json:"rows"`
		Total      int    `json:"total"`
		NextCursor string `json:"nextCursor"`
	} `json:"preparing"`
	NeedsHelp struct {
		Rows []struct {
			ID, Kind, Question, ActionLabel, ActionHref string
		} `json:"rows"`
		Total      int    `json:"total"`
		NextCursor string `json:"nextCursor"`
	} `json:"needsHelp"`
	RecentlyReady struct {
		Rows []struct {
			ClipHash, Name, StatusLabel string
		} `json:"rows"`
		Total      int    `json:"total"`
		NextCursor string `json:"nextCursor"`
	} `json:"recentlyReady"`
}

func readIncoming(t *testing.T, serverURL, path, token string) (*http.Response, calmIncomingBody) {
	t.Helper()
	res := sourceReq(t, http.MethodGet, serverURL+path, "", token)
	var body calmIncomingBody
	if res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
	}
	return res, body
}

func clipHashFor(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}

func putClip(t *testing.T, st store.Store, clip filler.Clip) {
	t.Helper()
	if clip.Hash == "" {
		clip.Hash = clipHashFor(clip.Path)
	}
	if err := st.UpsertClip(context.Background(), store.Clip{Clip: clip, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
}

func TestFillerIncoming_SeparatesMachineWorkAndReadyClipsWithoutInventingHumanWork(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	for _, clip := range []filler.Clip{
		{Hash: "preparing", Path: "preparing.mp4", Name: "Preparing clip", Held: true},
		{Hash: "ready", Path: "ready.mp4", Name: "Ready clip"},
		{Hash: "old-ready", Path: "old-ready.mp4", Name: "Old Ready clip"},
		{Hash: "audit-review", Path: "audit-review.mp4", Name: "Old audit review", Held: true},
	} {
		putClip(t, st, clip)
	}
	for _, row := range []filler.ClipPipeline{
		{ClipHash: "preparing", Stage: filler.StageTranscode, Status: filler.StatusRunning, Disposition: filler.DispositionRunning, UpdatedAt: now,
			Stages: []filler.StageRecord{{Stage: filler.StageProbe, Status: filler.StatusDone, At: now.Add(-time.Minute)}}},
		{ClipHash: "ready", Stage: filler.StageScore, Status: filler.StatusDone, Disposition: filler.DispositionReady, UpdatedAt: now.Add(-time.Hour)},
		{ClipHash: "old-ready", Stage: filler.StageScore, Status: filler.StatusDone, Disposition: filler.DispositionReady, UpdatedAt: now.Add(-8 * 24 * time.Hour)},
		{ClipHash: "audit-review", Stage: filler.StageScore, Status: filler.StatusDone, Disposition: filler.DispositionReview, UpdatedAt: now.Add(-2 * time.Hour)},
	} {
		if err := st.UpsertClipPipeline(t.Context(), row); err != nil {
			t.Fatal(err)
		}
	}

	res, body := readIncoming(t, srv.URL, "/v1/filler/incoming", adminToken)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if body.Preparing.Total != 1 || len(body.Preparing.Rows) != 1 ||
		body.Preparing.Rows[0].ClipHash != "preparing" || body.Preparing.Rows[0].StatusLabel != "Checking video" {
		t.Fatalf("preparing = %+v, want the one machine-owned clip with a friendly status", body.Preparing)
	}
	if stages := body.Preparing.Rows[0].Technical.Stages; len(stages) != 1 || stages[0].Label != "Checking video" || stages[0].Status != "Finished" {
		t.Fatalf("technical stages = %+v, want friendly history behind details", stages)
	}
	if body.RecentlyReady.Total != 1 || len(body.RecentlyReady.Rows) != 1 ||
		body.RecentlyReady.Rows[0].ClipHash != "ready" || body.RecentlyReady.Rows[0].StatusLabel != "Ready" {
		t.Fatalf("recently ready = %+v, want only the recent Ready clip", body.RecentlyReady)
	}
	if body.NeedsHelp.Total != 0 || len(body.NeedsHelp.Rows) != 0 {
		t.Fatalf("needs help = %+v; an audit-only review must not become household work", body.NeedsHelp)
	}
}

func TestFillerIncoming_PagesPreparingWithTheSamePredicateAsItsTotal(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	for i := range 22 {
		hash := fmt.Sprintf("preparing-%02d", i)
		putClip(t, st, filler.Clip{Hash: hash, Path: hash + ".mp4", Name: fmt.Sprintf("Clip %02d", i), Held: true})
		if err := st.UpsertClipPipeline(t.Context(), filler.ClipPipeline{
			ClipHash: hash, Stage: filler.StageTranscode, Status: filler.StatusQueued,
			Disposition: filler.DispositionRunning, UpdatedAt: now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}

	_, first := readIncoming(t, srv.URL, "/v1/filler/incoming", adminToken)
	if first.Preparing.Total != 22 || len(first.Preparing.Rows) != 20 || first.Preparing.NextCursor == "" {
		t.Fatalf("first preparing page = total %d, rows %d, cursor %q", first.Preparing.Total, len(first.Preparing.Rows), first.Preparing.NextCursor)
	}
	if first.Preparing.Rows[0].ClipHash != "preparing-21" {
		t.Fatalf("first preparing row = %q, want newest clip", first.Preparing.Rows[0].ClipHash)
	}
	_, second := readIncoming(t, srv.URL, "/v1/filler/incoming?preparingCursor="+url.QueryEscape(first.Preparing.NextCursor), adminToken)
	if second.Preparing.Total != 22 || len(second.Preparing.Rows) != 2 || second.Preparing.NextCursor != "" {
		t.Fatalf("second preparing page = total %d, rows %d, cursor %q", second.Preparing.Total, len(second.Preparing.Rows), second.Preparing.NextCursor)
	}
	seen := make(map[string]bool, len(first.Preparing.Rows))
	for _, row := range first.Preparing.Rows {
		seen[row.ClipHash] = true
	}
	for _, row := range second.Preparing.Rows {
		if seen[row.ClipHash] {
			t.Fatalf("clip %q appears on both pages", row.ClipHash)
		}
	}
}

func TestFillerIncoming_ProjectsAndPagesOnlyReadySplitsAsDurableHelp(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	for i := range 22 {
		hash := fmt.Sprintf("compilation-%02d", i)
		putClip(t, st, filler.Clip{Hash: hash, Path: hash + ".mp4", Name: fmt.Sprintf("Reel %02d", i), Held: true, IsComposite: true})
		if err := st.UpsertSplitProposal(t.Context(), filler.SplitProposal{
			ID: fmt.Sprintf("split-%02d", i), ClipHash: hash, CreatedAt: now.Add(time.Duration(i) * time.Second),
			Segments: []filler.SplitSegment{{Index: 0, StartMs: 0, EndMs: 30_000, Name: "first clip"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	_, first := readIncoming(t, srv.URL, "/v1/filler/incoming", adminToken)
	if first.NeedsHelp.Total != 22 || len(first.NeedsHelp.Rows) != 20 || first.NeedsHelp.NextCursor == "" {
		t.Fatalf("first Needs-help page = total %d, rows %d, cursor %q", first.NeedsHelp.Total, len(first.NeedsHelp.Rows), first.NeedsHelp.NextCursor)
	}
	got := first.NeedsHelp.Rows[0]
	if got.ID != "split-21" || got.Kind != "split_boundary" || got.Question != "Where should this compilation be split?" ||
		got.ActionLabel != "Review clips" || got.ActionHref != "/filler/splits/split-21" {
		t.Fatalf("task = %+v, want the newest server-authored split destination", got)
	}
	_, second := readIncoming(t, srv.URL, "/v1/filler/incoming?needsHelpCursor="+url.QueryEscape(first.NeedsHelp.NextCursor), adminToken)
	if second.NeedsHelp.Total != 22 || len(second.NeedsHelp.Rows) != 2 || second.NeedsHelp.NextCursor != "" {
		t.Fatalf("second Needs-help page = total %d, rows %d, cursor %q", second.NeedsHelp.Total, len(second.NeedsHelp.Rows), second.NeedsHelp.NextCursor)
	}
}

func TestFillerIncoming_RejectsInvalidCursorAndRequiresAdmin(t *testing.T) {
	srv, _, _ := newFillerServer(t)
	if res, _ := readIncoming(t, srv.URL, "/v1/filler/incoming?preparingCursor=not-a-cursor", adminToken); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid cursor status = %d, want 422", res.StatusCode)
	}
	if res, _ := readIncoming(t, srv.URL, "/v1/filler/incoming", ""); res.StatusCode == http.StatusOK {
		t.Fatal("anonymous request reached Incoming")
	}
}
