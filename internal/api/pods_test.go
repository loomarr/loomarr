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
	// ⚠ SEPARATE from draftAsked, so a test can assert the pod and the meter were built from
	// the SAME selection — which is the property V51f added CoverageDraft for. One shared slice
	// would make "both got the draft" indistinguishable from "one got it twice".
	draftCoverageAsked []filler.Selection
	atAsked            []int64 // the break starts PreviewAt received
	pod                filler.Pod
	coverage           filler.CoverageReport
	pool               filler.PoolReport
	// fits is what ClipFit returns, keyed by channel id; fitAsked records the clip paths it
	// was asked about, so a test can prove the route resolves the id it was given.
	fits     map[string]filler.Fit
	fitAsked []string
	err      error
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

// Coverage records the channel so a test can prove the route resolves the id it was given
// rather than reporting on some other channel's catalog.
func (f *fakePods) Coverage(_ context.Context, channelID string) (filler.CoverageReport, error) {
	f.asked = append(f.asked, channelID)
	return f.coverage, f.err
}

// CoverageDraft records the SELECTION as well as the channel, so a test can prove the draft
// handler passed the draft it was given rather than re-reading the saved policy.
//
// ⚠ It honours ctx. A double that ignores cancellation cannot catch a handler doing work through
// a dead context, which is a class this repo has already been bitten by twice.
func (f *fakePods) CoverageDraft(ctx context.Context, channelID string, sel filler.Selection) (filler.CoverageReport, error) {
	if err := ctx.Err(); err != nil {
		return filler.CoverageReport{}, err
	}
	f.asked = append(f.asked, channelID)
	f.draftCoverageAsked = append(f.draftCoverageAsked, sel)
	return f.coverage, f.err
}

// ClipFit records the clip path so a test can prove the route asks about the clip in the URL.
func (f *fakePods) ClipFit(_ context.Context, clipPath string) (map[string]filler.Fit, error) {
	f.fitAsked = append(f.fitAsked, clipPath)
	return f.fits, f.err
}

func (f *fakePods) Pool(context.Context) (filler.PoolReport, error) {
	return f.pool, f.err
}

func newPodsServer(t *testing.T) (*httptest.Server, store.Store, *fakePods) {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/p.db")
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

// ⚠ **The pod and the coverage meter must come from ONE selection (§10 V51f).** Before this the
// meter read the SAVED policy while the timeline directly beneath it rendered the DRAFT, so during
// an edit the page showed a meter for one selection above a pod for another — with nothing on
// screen saying they disagreed. Asserting the two adapter calls received the SAME selection is
// what makes that unrepresentable, rather than asserting a number the handler could compute twice.
func TestPreviewDraftPods_MeterAndPodDescribeTheSameSelection(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	fp.coverage = filler.CoverageReport{
		Level: filler.MatchAudience,
		Total: 7,
		Criteria: []filler.CriterionCoverage{
			{Criterion: filler.CriterionAudience, Clips: 0},
			{Criterion: filler.CriterionEra, Clips: 7},
		},
	}

	body := `{"filler":{"audience":"kids","era":{"from":1990,"to":1999}}}`
	resp := do(t, srv, http.MethodPost, "/v1/channels/ch-1/pods/preview", adminToken, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("draft preview → %d, want 200", resp.StatusCode)
	}

	if len(fp.draftCoverageAsked) != 1 {
		t.Fatalf("CoverageDraft called %d times, want 1 — the meter is not being computed for the draft", len(fp.draftCoverageAsked))
	}
	if len(fp.draftAsked) != 1 {
		t.Fatalf("PreviewDraft called %d times, want 1", len(fp.draftAsked))
	}
	pod, meter := fp.draftAsked[0], fp.draftCoverageAsked[0]
	if pod.Audience != meter.Audience || pod.Era != meter.Era {
		t.Errorf("pod was built from %+v but the meter from %+v — they must describe one selection", pod, meter)
	}
	// ⚠ Both bounds reach the domain: the era range is the V51f fix, and a handler that dropped
	// `to` here would look identical to the pre-fix behaviour.
	if meter.Era != (filler.EraRange{From: 1990, To: 1999}) {
		t.Errorf("era reached the domain as %+v, want 1990-1999", meter.Era)
	}

	var got struct {
		Coverage struct {
			Level    string `json:"level"`
			Total    int    `json:"total"`
			Criteria []struct {
				Criterion string `json:"criterion"`
				Clips     int    `json:"clips"`
			} `json:"criteria"`
		} `json:"coverage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode draft preview body: %v", err)
	}
	if got.Coverage.Level != "audience" || got.Coverage.Total != 7 {
		t.Errorf("coverage = %+v, want the draft's report on the wire", got.Coverage)
	}
	if len(got.Coverage.Criteria) != 2 || got.Coverage.Criteria[0].Criterion != "audience" {
		t.Errorf("criteria = %+v, want the per-setting breakdown", got.Coverage.Criteria)
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

// --- per-clip channel fit (§10, V35 item 1.7) ---

func decodeFit(t *testing.T, resp *http.Response) []api.ChannelFitDTO {
	t.Helper()
	var body struct {
		Channels []api.ChannelFitDTO `json:"channels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Channels
}

// The route resolves the clip in the URL, and answers per channel.
func TestClipFit_AnswersForEveryChannel(t *testing.T) {
	srv, st, fp := newPodsServer(t)
	if err := st.UpsertChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{ID: "ch-2", Name: "Late Night", Number: 7, Status: "live"},
	}); err != nil {
		t.Fatal(err)
	}
	fp.fits = map[string]filler.Fit{
		"ch-1": {Level: filler.MatchExact},
		"ch-2": {Level: filler.MatchBumperCard, Reason: filler.FitAudience},
	}

	resp := do(t, srv, http.MethodGet, "/v1/filler/fit?clip=ads%2Fcereal.mp4", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	rows := decodeFit(t, resp)

	if len(fp.fitAsked) != 1 || fp.fitAsked[0] != "ads/cereal.mp4" {
		t.Errorf("service asked about %v, want the clip in the URL", fp.fitAsked)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want one per channel", len(rows))
	}
	byID := map[string]api.ChannelFitDTO{}
	for _, r := range rows {
		byID[r.ChannelID] = r
	}
	if byID["ch-1"].Level != "exact" || byID["ch-1"].Reason != "" {
		t.Errorf("ch-1 = %+v, want an exact match with no reason", byID["ch-1"])
	}
	// ⚠ A rejected row must carry WHY. "Won't be picked" with no reason is what sends an
	// operator hunting through channel settings for a rule they cannot see.
	if byID["ch-2"].Reason != "audience" {
		t.Errorf("ch-2 reason = %q, want the rejecting predicate", byID["ch-2"].Reason)
	}
	// The row carries enough to render without a second request.
	if byID["ch-2"].Name != "Late Night" || byID["ch-2"].Number != 7 {
		t.Errorf("ch-2 = %+v, want the channel's name and number", byID["ch-2"])
	}
}

// ⚠ Rows come out in the STORE's order (number-sorted), never Go's randomised map order — a
// picker whose rows shuffle on every render is unusable.
func TestClipFit_RowsAreOrderedNotShuffled(t *testing.T) {
	srv, st, fp := newPodsServer(t)
	for _, ch := range []struct {
		id  string
		num int
	}{{"ch-2", 2}, {"ch-3", 3}, {"ch-4", 4}, {"ch-5", 5}} {
		if err := st.UpsertChannel(context.Background(), store.Channel{
			Channel: schedule.Channel{ID: ch.id, Name: ch.id, Number: ch.num, Status: "live"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	fp.fits = map[string]filler.Fit{}
	for _, id := range []string{"ch-1", "ch-2", "ch-3", "ch-4", "ch-5"} {
		fp.fits[id] = filler.Fit{Level: filler.MatchExact}
	}

	// Repeated, because map iteration order varies per RUN — one pass could agree by luck.
	var first []string
	for range 5 {
		var got []string
		for _, r := range decodeFit(t, do(t, srv, http.MethodGet, "/v1/filler/fit?clip=a.mp4", adminToken, "")) {
			got = append(got, r.ChannelID)
		}
		if first == nil {
			first = got
			continue
		}
		if len(got) != len(first) {
			t.Fatalf("row count changed between requests: %d then %d", len(first), len(got))
		}
		for i := range got {
			if got[i] != first[i] {
				t.Fatalf("row order changed between requests: %v then %v", first, got)
			}
		}
	}
}

// ⚠ Both flags are reported AS STORED, including the contradictory both-true state — assembly
// resolves it (excluded wins), and normalising here would show a state the database does not
// hold, so an operator un-ticking "excluded" would see a pin they never made.
func TestClipFit_ReportsPinAndExcludeAsStored(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	fp.fits = map[string]filler.Fit{
		"ch-1": {Level: filler.MatchBumperCard, Reason: filler.FitExcluded, Pinned: true, Excluded: true},
	}

	rows := decodeFit(t, do(t, srv, http.MethodGet, "/v1/filler/fit?clip=a.mp4", adminToken, ""))
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if !rows[0].Pinned || !rows[0].Excluded {
		t.Errorf("row = %+v, want both flags as stored", rows[0])
	}
	if rows[0].Reason != "excluded" {
		t.Errorf("reason = %q, want excluded to win", rows[0].Reason)
	}
}

func TestClipFit_UnknownClipIs404(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	fp.err = store.ErrNotFound

	if resp := do(t, srv, http.MethodGet, "/v1/filler/fit?clip=gone.mp4", adminToken, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// Read-only, so a member may call it — the same posture as the coverage meter. A member sees
// which channels a clip serves; they cannot change it (the write is admin-only).
func TestClipFit_IsReadableByAMember(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	fp.fits = map[string]filler.Fit{"ch-1": {Level: filler.MatchExact}}

	if resp := do(t, srv, http.MethodGet, "/v1/filler/fit?clip=a.mp4", memberToken, ""); resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 — a read-only note is member-visible", resp.StatusCode)
	}
}
