package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

type calmIncomingBody struct {
	ReadyWindowSeconds int64 `json:"readyWindowSeconds"`
	Preparing          struct {
		Rows []struct {
			ClipHash, Name, StatusLabel string
			Processing                  struct {
				Attempts                   int
				NextTryAt, DiagnosticsHref string
				Stages                     []struct {
					Label, Outcome, OutcomeLabel, Note, At string
					Progress                               *int
				} `json:"stages"`
			} `json:"processing"`
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
	harness := newFillerHarness(t)
	srv, st := harness.Server, harness.Store
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
		{ClipHash: "old-ready", Stage: filler.StageScore, Status: filler.StatusDone, Disposition: filler.DispositionReady, UpdatedAt: now.Add(-25 * time.Hour)},
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
	if stages := body.Preparing.Rows[0].Processing.Stages; len(stages) != 2 ||
		stages[0].Label != "Inspecting the file" || stages[0].OutcomeLabel != "Finished" ||
		stages[1].Label != "Preparing playback" || stages[1].Outcome != "in_progress" {
		t.Fatalf("processing stages = %+v, want history followed by the active step", stages)
	}
	if body.RecentlyReady.Total != 1 || len(body.RecentlyReady.Rows) != 1 ||
		body.RecentlyReady.Rows[0].ClipHash != "ready" || body.RecentlyReady.Rows[0].StatusLabel != "Ready" {
		t.Fatalf("recently ready = %+v, want only the recent Ready clip", body.RecentlyReady)
	}
	if body.ReadyWindowSeconds != int64((24*time.Hour)/time.Second) {
		t.Fatalf("ready window = %ds, want 24h", body.ReadyWindowSeconds)
	}
	if body.NeedsHelp.Total != 0 || len(body.NeedsHelp.Rows) != 0 {
		t.Fatalf("needs help = %+v; an audit-only review must not become household work", body.NeedsHelp)
	}
}

func TestFillerIncoming_ProjectsSafeOrderedProcessingDetailsAndMeasuredCurrentStage(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st := harness.Server, harness.Store
	now := time.Now().UTC().Truncate(time.Second)
	putClip(t, st, filler.Clip{Hash: "retrying", Path: "retrying.mp4", Name: "Retrying clip", Held: true})
	if err := st.UpsertClipPipeline(t.Context(), filler.ClipPipeline{
		ClipHash: "retrying", Stage: filler.StageTranscode, Status: filler.StatusRunning, Progress: 47,
		Disposition: filler.DispositionRunning, Attempts: 2, UpdatedAt: now,
		Stages: []filler.StageRecord{
			{Stage: filler.StageProbe, Status: filler.StatusDone, At: now.Add(-2 * time.Minute)},
			{Stage: filler.StageTranscode, Status: filler.StatusFailed, Note: "open /Users/private/token: permission denied", At: now.Add(-time.Minute)},
			{Stage: filler.StageTranscode, Status: filler.StatusFailed, Note: "open /Users/private/token: permission denied", At: now.Add(-time.Minute)},
		},
	}); err != nil {
		t.Fatal(err)
	}

	res, body := readIncoming(t, srv.URL, "/v1/filler/incoming", adminToken)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK || len(body.Preparing.Rows) != 1 {
		t.Fatalf("Incoming = status %d, preparing %+v", res.StatusCode, body.Preparing)
	}
	processing := body.Preparing.Rows[0].Processing
	if processing.DiagnosticsHref != "/filler/manage#diagnostics" {
		t.Fatalf("diagnostics href = %q, want existing owner", processing.DiagnosticsHref)
	}
	if len(processing.Stages) != 4 {
		t.Fatalf("stages = %+v, want all three stored occurrences plus current", processing.Stages)
	}
	for _, index := range []int{1, 2} {
		stage := processing.Stages[index]
		if stage.Outcome != "retrying" || stage.OutcomeLabel != "Trying again" ||
			stage.Note != "This step did not finish. Loomarr will try again automatically." {
			t.Fatalf("retry stage %d = %+v, want safe server-owned explanation", index, stage)
		}
	}
	active := processing.Stages[3]
	if active.Outcome != "in_progress" || active.Progress == nil || *active.Progress != 47 {
		t.Fatalf("active stage = %+v, want measured current-stage progress", active)
	}
	encoded, err := json.Marshal(processing)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || incomingContainsAny(string(encoded), "/Users/private", "permission denied") {
		t.Fatalf("processing leaked raw failure evidence: %s", encoded)
	}
}

func TestFillerIncoming_ShowsNextTryOnlyWhileTheCurrentStepWaitsForRetry(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st := harness.Server, harness.Store
	now := time.Now().UTC().Truncate(time.Second)
	for _, clip := range []filler.Clip{
		{Hash: "active", Path: "active.mp4", Name: "Active clip", Held: true},
		{Hash: "retrying", Path: "retrying.mp4", Name: "Retrying clip", Held: true},
	} {
		putClip(t, st, clip)
	}
	for _, row := range []filler.ClipPipeline{
		{ClipHash: "active", Stage: filler.StageTranscode, Status: filler.StatusRunning,
			Disposition: filler.DispositionRunning, NextRun: now.Add(time.Hour), UpdatedAt: now},
		{ClipHash: "retrying", Stage: filler.StageVision, Status: filler.StatusFailed,
			Disposition: filler.DispositionRunning, NextRun: now.Add(time.Hour), UpdatedAt: now.Add(-time.Second)},
	} {
		if err := st.UpsertClipPipeline(t.Context(), row); err != nil {
			t.Fatal(err)
		}
	}

	res, body := readIncoming(t, srv.URL, "/v1/filler/incoming", adminToken)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK || len(body.Preparing.Rows) != 2 {
		t.Fatalf("Incoming = status %d, preparing %+v", res.StatusCode, body.Preparing)
	}
	byHash := make(map[string]struct{ NextTryAt string }, len(body.Preparing.Rows))
	for _, row := range body.Preparing.Rows {
		byHash[row.ClipHash] = struct{ NextTryAt string }{NextTryAt: row.Processing.NextTryAt}
	}
	if byHash["active"].NextTryAt != "" {
		t.Fatalf("active next try = %q, want no retry message while work is in progress", byHash["active"].NextTryAt)
	}
	if byHash["retrying"].NextTryAt != now.Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("retrying next try = %q, want scheduled retry", byHash["retrying"].NextTryAt)
	}
}

func incomingContainsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func TestFillerIncoming_UsesTheLiveReadyWindowForRowsAndTotals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		window time.Duration
	}{
		{name: "minimum", window: time.Hour},
		{name: "maximum", window: 30 * 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
			harness := newFillerIncomingHarness(t, tc.window, now)
			srv, st := harness.Server, harness.Store
			for _, clip := range []filler.Clip{
				{Hash: "at-cutoff", Path: "at-cutoff.mp4", Name: "At cutoff"},
				{Hash: "before-cutoff", Path: "before-cutoff.mp4", Name: "Before cutoff"},
			} {
				putClip(t, st, clip)
			}
			for _, row := range []filler.ClipPipeline{
				{ClipHash: "at-cutoff", Stage: filler.StageScore, Status: filler.StatusDone, Disposition: filler.DispositionReady, UpdatedAt: now.Add(-tc.window)},
				{ClipHash: "before-cutoff", Stage: filler.StageScore, Status: filler.StatusDone, Disposition: filler.DispositionReady, UpdatedAt: now.Add(-tc.window - time.Second)},
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
			if body.ReadyWindowSeconds != int64(tc.window/time.Second) {
				t.Fatalf("ready window = %ds, want %s", body.ReadyWindowSeconds, tc.window)
			}
			if body.RecentlyReady.Total != 1 || len(body.RecentlyReady.Rows) != 1 || body.RecentlyReady.Rows[0].ClipHash != "at-cutoff" {
				t.Fatalf("recently ready = %+v, want inclusive cutoff only", body.RecentlyReady)
			}
		})
	}
}

func TestFillerIncoming_PagesPreparingWithTheSamePredicateAsItsTotal(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st := harness.Server, harness.Store
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
	harness := newFillerHarness(t)
	srv, st := harness.Server, harness.Store
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
	harness := newFillerHarness(t)
	srv := harness.Server
	if res, _ := readIncoming(t, srv.URL, "/v1/filler/incoming?preparingCursor=not-a-cursor", adminToken); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid cursor status = %d, want 422", res.StatusCode)
	}
	if res, _ := readIncoming(t, srv.URL, "/v1/filler/incoming", ""); res.StatusCode == http.StatusOK {
		t.Fatal("anonymous request reached Incoming")
	}
}
