package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// fakePods records the channel it was asked about, so a test can prove the handler
// passes the id through rather than previewing some other channel.
type fakePods struct {
	asked      []string
	draftAsked []filler.Selection // the draft selections PreviewDraft received
	atAsked    []int64            // the break starts PreviewAt received
	pod        filler.Pod
	err        error
}

func (f *fakePods) Preview(_ context.Context, channelID string) (filler.Pod, error) {
	f.asked = append(f.asked, channelID)
	return f.pod, f.err
}

func (f *fakePods) PreviewDraft(_ context.Context, channelID string, sel filler.Selection) (filler.Pod, error) {
	f.asked = append(f.asked, channelID)
	f.draftAsked = append(f.draftAsked, sel)
	return f.pod, f.err
}

// PreviewAt records the BREAK START it was asked for, so a test can prove the guide resolves
// each break individually rather than reusing one channel-wide pod.
func (f *fakePods) PreviewAt(_ context.Context, channelID string, breakStartMs int64) (filler.Pod, error) {
	f.asked = append(f.asked, channelID)
	f.atAsked = append(f.atAsked, breakStartMs)
	return f.pod, f.err
}

func newPodsServer(t *testing.T) (*httptest.Server, store.Store, *fakePods) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/p.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.UpsertChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{ID: "ch-1", Name: "Cartoons", Number: 42, Status: "live"},
	}); err != nil {
		t.Fatal(err)
	}
	fp := &fakePods{}
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st,
		Auth:  testAuthorizer{},
		Log:   slog.New(slog.DiscardHandler),
		Pods:  fp,
	}))
	t.Cleanup(srv.Close)
	return srv, st, fp
}

// The preview renders the assembled pool, including the match level — the answer to
// "why are my commercials wrong" (§10 fallback ladder).
func TestPreviewPods_RendersPoolAndMatchLevel(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	fp.pod = filler.Pod{
		Entries: []filler.PodEntry{
			{TunarrProgramID: "p1", Name: "Bumper", Kind: filler.Bumper, DurationMs: 5000},
			{TunarrProgramID: "p2", Name: "Frosted Flakes", Kind: filler.Commercial, DurationMs: 30000},
		},
		TotalMs:    35000,
		MatchLevel: filler.MatchExact,
	}

	resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1/pods", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Entries []struct {
			TunarrProgramID string `json:"tunarrProgramId"`
			Name            string `json:"name"`
			Kind            string `json:"kind"`
			DurationMs      int64  `json:"durationMs"`
		} `json:"entries"`
		TotalMs    int64  `json:"totalMs"`
		MatchLevel string `json:"matchLevel"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(body.Entries))
	}
	// Play order is meaningful: a pod is bookended by bumpers (§10 pod structure), so
	// the API must not reorder what the assembler produced.
	if body.Entries[0].Name != "Bumper" || body.Entries[1].Name != "Frosted Flakes" {
		t.Errorf("play order not preserved: %+v", body.Entries)
	}
	if body.Entries[1].TunarrProgramID != "p2" {
		t.Error("entry lost its Tunarr program id — the FE cannot deep-link the clip without it")
	}
	if body.TotalMs != 35000 {
		t.Errorf("totalMs = %d, want 35000", body.TotalMs)
	}
	if body.MatchLevel != string(filler.MatchExact) {
		t.Errorf("matchLevel = %q, want %q", body.MatchLevel, filler.MatchExact)
	}
	if len(fp.asked) != 1 || fp.asked[0] != "ch-1" {
		t.Errorf("previewed %v, want exactly [ch-1]", fp.asked)
	}
}

// An empty catalog is a normal state ("no clips yet"), not an error — and the JSON must
// carry [] rather than null so the FE renders an empty state instead of guarding a case
// that never means failure.
func TestPreviewPods_EmptyCatalogIsEmptyArray(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	fp.pod = filler.Pod{}

	resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1/pods", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview → %d, want 200", resp.StatusCode)
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if got := string(raw["entries"]); got != "[]" {
		t.Errorf("entries = %s, want [] (never null)", got)
	}
}

// Preview is a READ. §12 puts it on the Filler view for anyone diagnosing a channel, and
// it exposes nothing an authenticated user cannot already see via /v1/filler.
func TestPreviewPods_VisibleToAnyAuthenticatedUser(t *testing.T) {
	srv, _, _ := newPodsServer(t)
	if resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1/pods", memberToken, ""); resp.StatusCode != http.StatusOK {
		t.Errorf("member preview → %d, want 200 (preview is read-only)", resp.StatusCode)
	}
}

func TestPreviewPods_UnknownChannelIs404(t *testing.T) {
	srv, _, _ := newPodsServer(t)
	if resp := do(t, srv, http.MethodGet, "/v1/channels/nope/pods", adminToken, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown channel → %d, want 404", resp.StatusCode)
	}
}

// --- draft preview (POST …/pods/preview) — the sandbox ---

// The draft preview assembles a proposed (unsaved) selection and passes it through to the
// adapter, so the channel page can show exactly what a filler change would air before Apply.
func TestPreviewDraftPods_AssemblesTheDraftSelection(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	fp.pod = filler.Pod{Entries: []filler.PodEntry{
		{TunarrProgramID: "p1", Name: "Pinned Ad", Kind: filler.Commercial, DurationMs: 30000},
	}, TotalMs: 30000, MatchLevel: filler.MatchExact}

	body := `{"filler":{"audience":"kids","categories":["toys"],"pinned":["p1"]}}`
	resp := do(t, srv, http.MethodPost, "/v1/channels/ch-1/pods/preview", adminToken, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("draft preview → %d, want 200", resp.StatusCode)
	}
	if len(fp.draftAsked) != 1 {
		t.Fatalf("PreviewDraft called %d times, want 1", len(fp.draftAsked))
	}
	got := fp.draftAsked[0]
	if got.Audience != "kids" || len(got.Categories) != 1 || got.Categories[0] != "toys" {
		t.Errorf("draft selection not passed through: %+v", got)
	}
	if len(got.Pinned) != 1 || got.Pinned[0] != "p1" {
		t.Errorf("pinned not passed through: %+v", got.Pinned)
	}
}

// The draft preview is an authoring tool (it precedes an Apply that writes policy), so it
// is admin-only — a member gets 403, not a sandbox.
func TestPreviewDraftPods_RequiresAdmin(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	resp := do(t, srv, http.MethodPost, "/v1/channels/ch-1/pods/preview", "", `{"filler":{}}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("member draft preview → %d, want 401", resp.StatusCode)
	}
	if len(fp.draftAsked) != 0 {
		t.Error("a forbidden draft preview still reached the assembler")
	}
}

// A nonsense selection is rejected (422) with the same validation a policy write uses —
// the sandbox must not assemble an invalid draft.
func TestPreviewDraftPods_InvalidSelection422(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	resp := do(t, srv, http.MethodPost, "/v1/channels/ch-1/pods/preview", adminToken,
		`{"filler":{"audience":"nonsense"}}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid draft → %d, want 422", resp.StatusCode)
	}
	if len(fp.draftAsked) != 0 {
		t.Error("an invalid draft was assembled instead of rejected")
	}
}

func TestPreviewDraftPods_UnknownChannelIs404(t *testing.T) {
	srv, _, _ := newPodsServer(t)
	resp := do(t, srv, http.MethodPost, "/v1/channels/nope/pods/preview", adminToken, `{"filler":{}}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown channel draft → %d, want 404", resp.StatusCode)
	}
}

// /v1/channels/{id}/pods must not shadow /v1/channels/now-next. Both are GETs under
// /v1/channels, and a greedy {id} match would turn the now-next read into a pod preview
// for a channel called "now-next" — pinned because it is the kind of routing bug that
// only shows up in the browser.
func TestPreviewPods_DoesNotShadowNowNext(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/channels/now-next", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("now-next → %d, want 200", resp.StatusCode)
	}
	if len(fp.asked) != 0 {
		t.Errorf("now-next was routed to the pod preview (asked %v)", fp.asked)
	}
}
