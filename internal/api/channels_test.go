package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeChannelSvc records reconcile + purge calls.
type fakeChannelSvc struct {
	reconciles int
	purges     int
	err        error
}

func (f *fakeChannelSvc) Reconcile(ctx context.Context, id string) error {
	f.reconciles++
	return f.err
}

func (f *fakeChannelSvc) Purge(ctx context.Context, id string) error {
	f.purges++
	return f.err
}

// fakeLiveTVSvc is a stateful Live TV service double.
type fakeLiveTVSvc struct {
	wired    bool
	connects int
}

func (f *fakeLiveTVSvc) Connect(ctx context.Context) (bool, bool, error) {
	f.connects++
	if f.wired {
		return false, false, nil // already wired → no-op
	}
	f.wired = true
	return true, true, nil
}
func (f *fakeLiveTVSvc) Wired(ctx context.Context) (bool, error) { return f.wired, nil }

func newServerWithScheduler(t *testing.T) (*httptest.Server, store.Store, *fakeChannelSvc, *fakeLiveTVSvc) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/api.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	chSvc := &fakeChannelSvc{}
	ltv := &fakeLiveTVSvc{}
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:    st,
		Auth:     api.NewTokenAuthorizer(adminToken),
		Log:      slog.New(slog.DiscardHandler),
		Channels: chSvc,
		LiveTV:   ltv,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st, chSvc, ltv
}

func TestCreateChannelAdmin(t *testing.T) {
	srv, _, chSvc, _ := newServerWithScheduler(t)
	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"ch1","name":"Cartoons","number":42,"strategy":"sequential"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create → %d, want 200", resp.StatusCode)
	}
	var body struct {
		ID, Name, Status string
		Number           int
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.ID != "ch1" || body.Number != 42 {
		t.Errorf("create body = %+v", body)
	}
	// Creation kicks an initial reconcile (§9 live-immediately).
	if chSvc.reconciles != 1 {
		t.Errorf("create should kick 1 reconcile, got %d", chSvc.reconciles)
	}
}

func TestCreateChannelRequiresAdmin(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	for _, tok := range []string{"", "wrong"} {
		resp := do(t, srv, http.MethodPost, "/v1/channels", tok,
			`{"id":"c","name":"n","number":1,"strategy":"sequential"}`)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("create with token %q → %d, want 403", tok, resp.StatusCode)
		}
	}
}

func TestCreateChannelDuplicateNumber(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	_ = do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"c1","name":"A","number":5,"strategy":"sequential"}`)
	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"c2","name":"B","number":5,"strategy":"sequential"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate number → %d, want 409", resp.StatusCode)
	}
}

func TestListAndGetChannel(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	_ = do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"c1","name":"A","number":5,"strategy":"sequential"}`)

	// List is visible to any authenticated user (admin token here).
	resp := do(t, srv, http.MethodGet, "/v1/channels", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list → %d", resp.StatusCode)
	}
	var list struct {
		Channels []struct{ ID string }
	}
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if len(list.Channels) != 1 || list.Channels[0].ID != "c1" {
		t.Errorf("list = %+v", list.Channels)
	}

	// Get one.
	resp = do(t, srv, http.MethodGet, "/v1/channels/c1", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("get → %d", resp.StatusCode)
	}
	// Missing → 404.
	resp = do(t, srv, http.MethodGet, "/v1/channels/nope", adminToken, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get missing → %d, want 404", resp.StatusCode)
	}
}

func TestReconcileChannelAdmin(t *testing.T) {
	srv, _, chSvc, _ := newServerWithScheduler(t)
	_ = do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"c1","name":"A","number":5,"strategy":"sequential"}`)
	before := chSvc.reconciles

	resp := do(t, srv, http.MethodPost, "/v1/channels/c1/reconcile", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reconcile → %d, want 200", resp.StatusCode)
	}
	if chSvc.reconciles != before+1 {
		t.Errorf("reconcile not invoked: %d → %d", before, chSvc.reconciles)
	}
}

func TestReconcileChannelRequiresAdmin(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	resp := do(t, srv, http.MethodPost, "/v1/channels/c1/reconcile", "", "")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("member reconcile → %d, want 403", resp.StatusCode)
	}
}

func TestDeleteChannelDetaches(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	_ = do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"c1","name":"A","number":5,"strategy":"sequential"}`)

	resp := do(t, srv, http.MethodDelete, "/v1/channels/c1", adminToken, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete → %d, want 204", resp.StatusCode)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	if string(ch.Status) != "detached" {
		t.Errorf("channel status after delete = %s, want detached", ch.Status)
	}
}

// --- edit (PATCH), pause/resume, purge ---

func mkChannel(t *testing.T, srv *httptest.Server, id, name string, number int) {
	t.Helper()
	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"`+id+`","name":"`+name+`","number":`+itoa(number)+`,"strategy":"sequential"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("seed channel %s → %d", id, resp.StatusCode)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestUpdateChannel_RenameAndRenumber(t *testing.T) {
	srv, st, chSvc, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "Old Name", 5)
	before := chSvc.reconciles

	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken,
		`{"name":"New Name","number":7,"group":"Kids"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch → %d, want 200", resp.StatusCode)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	if ch.Name != "New Name" || ch.Number != 7 || ch.Group != "Kids" {
		t.Errorf("after patch = name=%q number=%d group=%q", ch.Name, ch.Number, ch.Group)
	}
	// An edit auto-reconciles (the seamless "no rebuild" model).
	if chSvc.reconciles != before+1 {
		t.Errorf("patch should auto-reconcile once: %d → %d", before, chSvc.reconciles)
	}
}

func TestUpdateChannel_RenumberCollision409(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "A", 5)
	mkChannel(t, srv, "c2", "B", 6)

	// Renumber c2 onto c1's number → 409 (not a 500 from the unique index).
	resp := do(t, srv, http.MethodPatch, "/v1/channels/c2", adminToken, `{"number":5}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("renumber collision → %d, want 409", resp.StatusCode)
	}
}

func TestUpdateChannel_RenumberToSelfOK(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "A", 5)

	// Re-setting a channel to its OWN number must not false-positive as a collision.
	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken, `{"number":5}`)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("renumber to self → %d, want 200", resp.StatusCode)
	}
}

func TestUpdateChannel_PolicyMergePreservesApplied(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "A", 5)

	// Seed a reconcile-owned relaxation on the channel (as a reconcile would).
	ctx := context.Background()
	ch, _ := st.GetChannel(ctx, "c1")
	ch.Policy.Applied = []schedule.AppliedRelaxation{{Kind: "episodeNoRepeat", From: "168h", To: "84h"}}
	if err := st.UpsertChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}

	// A policy edit that tries to also set `applied` must be ignored — the stored
	// `applied` is reconcile-owned and preserved.
	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken,
		`{"policy":{"ordering":"shuffle","applied":[{"kind":"hacked","from":"x","to":"y"}]}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("policy patch → %d, want 200", resp.StatusCode)
	}
	ch, _ = st.GetChannel(ctx, "c1")
	if ch.Policy.Ordering != "shuffle" {
		t.Errorf("policy.ordering = %q, want shuffle (the edit)", ch.Policy.Ordering)
	}
	if len(ch.Policy.Applied) != 1 || ch.Policy.Applied[0].Kind != "episodeNoRepeat" {
		t.Errorf("policy.applied = %+v, want the reconcile-owned value preserved (not client-set)", ch.Policy.Applied)
	}
}

func TestUpdateChannel_InvalidPolicyRejected(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "A", 5)

	// An off-ladder audience ceiling is a §4 safety violation → 422.
	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken,
		`{"policy":{"audience":{"ceiling":"NOT-A-RATING"}}}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("invalid policy → %d, want 422", resp.StatusCode)
	}
}

func TestUpdateChannel_PauseAndResume(t *testing.T) {
	srv, st, chSvc, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "A", 5)
	ctx := context.Background()

	// Pause: status → paused, and NO reconcile is kicked (a paused channel is off the sweep).
	before := chSvc.reconciles
	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken, `{"status":"paused"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pause → %d, want 200", resp.StatusCode)
	}
	ch, _ := st.GetChannel(ctx, "c1")
	if ch.Status != schedule.StatusPaused {
		t.Errorf("status after pause = %s, want paused", ch.Status)
	}
	if chSvc.reconciles != before {
		t.Errorf("pause must NOT reconcile: %d → %d", before, chSvc.reconciles)
	}

	// Resume: status → building, and it reconciles again.
	resp = do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken, `{"status":"building"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resume → %d, want 200", resp.StatusCode)
	}
	ch, _ = st.GetChannel(ctx, "c1")
	if ch.Status != schedule.StatusBuilding {
		t.Errorf("status after resume = %s, want building", ch.Status)
	}
	if chSvc.reconciles != before+1 {
		t.Errorf("resume should reconcile once: %d → %d", before, chSvc.reconciles)
	}
}

func TestUpdateChannel_RejectsBadStatus(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "A", 5)
	// A client may only pause/resume — never force live/detached/drifted.
	for _, s := range []string{"live", "detached", "drifted"} {
		resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken, `{"status":"`+s+`"}`)
		// Huma rejects an off-enum value at the schema layer (422) before the handler.
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("status=%q → %d, want 422", s, resp.StatusCode)
		}
	}
}

func TestUpdateChannel_RequiresAdmin(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "A", 5)
	for _, tok := range []string{"", "wrong"} {
		resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", tok, `{"name":"X"}`)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("patch with token %q → %d, want 403", tok, resp.StatusCode)
		}
	}
}

func TestUpdateChannel_NotFound(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	resp := do(t, srv, http.MethodPatch, "/v1/channels/nope", adminToken, `{"name":"X"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("patch missing → %d, want 404", resp.StatusCode)
	}
}

func TestDeleteChannel_PurgeCallsEngine(t *testing.T) {
	srv, _, chSvc, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "A", 5)

	resp := do(t, srv, http.MethodDelete, "/v1/channels/c1?purge=true", adminToken, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("purge delete → %d, want 204", resp.StatusCode)
	}
	if chSvc.purges != 1 {
		t.Errorf("purge should call Engine.Purge once, got %d", chSvc.purges)
	}
}

// --- setup routes ---

func TestLiveTVConnectIdempotentOverAPI(t *testing.T) {
	srv, _, _, ltv := newServerWithScheduler(t)

	first := do(t, srv, http.MethodPost, "/v1/setup/livetv-connect", adminToken, "")
	if first.StatusCode != http.StatusOK {
		t.Fatalf("connect → %d", first.StatusCode)
	}
	var fb struct{ TunerAdded, ListingAdded, AlreadyWired bool }
	_ = json.NewDecoder(first.Body).Decode(&fb)
	if !fb.TunerAdded || fb.AlreadyWired {
		t.Errorf("first connect body = %+v, want added+not-already-wired", fb)
	}

	// Second call is a no-op (§6 second-call-no-op gate, over the wire).
	second := do(t, srv, http.MethodPost, "/v1/setup/livetv-connect", adminToken, "")
	var sb struct{ TunerAdded, ListingAdded, AlreadyWired bool }
	_ = json.NewDecoder(second.Body).Decode(&sb)
	if sb.TunerAdded || !sb.AlreadyWired {
		t.Errorf("second connect body = %+v, want no-op already-wired", sb)
	}
	if ltv.connects != 2 {
		t.Errorf("connect called %d times, want 2", ltv.connects)
	}
}

func TestLiveTVConnectRequiresAdmin(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	resp := do(t, srv, http.MethodPost, "/v1/setup/livetv-connect", "", "")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("member connect → %d, want 403", resp.StatusCode)
	}
}

func TestSetupStatusReportsLiveTVCheck(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)

	// Before wiring: livetv check present and not OK, with a hint.
	resp := do(t, srv, http.MethodGet, "/v1/setup/status", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status → %d", resp.StatusCode)
	}
	var body struct {
		Checks []struct {
			Name string
			OK   bool
			Hint string
		}
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	var ltv *struct {
		Name string
		OK   bool
		Hint string
	}
	for i := range body.Checks {
		if body.Checks[i].Name == "livetv" {
			ltv = &body.Checks[i]
		}
	}
	if ltv == nil {
		t.Fatal("status missing livetv check")
	}
	if ltv.OK || ltv.Hint == "" {
		t.Errorf("unwired livetv check = %+v, want not-OK with a hint", *ltv)
	}

	// After connecting, the check flips to OK.
	_ = do(t, srv, http.MethodPost, "/v1/setup/livetv-connect", adminToken, "")
	resp = do(t, srv, http.MethodGet, "/v1/setup/status", adminToken, "")
	body.Checks = nil
	_ = json.NewDecoder(resp.Body).Decode(&body)
	for _, c := range body.Checks {
		if c.Name == "livetv" && !c.OK {
			t.Error("livetv check should be OK after connect")
		}
	}
}

func TestSetupStatusRequiresAdmin(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	resp := do(t, srv, http.MethodGet, "/v1/setup/status", "", "")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("member status → %d, want 403", resp.StatusCode)
	}
}

// fakeGuide is a scripted api.GuideReader keyed by TUNARR id (as the real one is).
type fakeGuide struct{ byTunarr map[string]api.ChannelNowNext }

func (f fakeGuide) NowNext(context.Context, time.Time) (map[string]api.ChannelNowNext, error) {
	return f.byTunarr, nil
}

// GET /v1/channels/now-next must resolve to the now/next handler, NOT to
// GET /v1/channels/{id} with id="now-next". A literal segment beats a wildcard in Go
// 1.22's ServeMux, but that is a routing detail worth pinning rather than assuming: if
// precedence ever flipped, the page would 404 on a channel that does not exist.
func TestChannelsNowNext_RoutesAndMapsToLoomarrChannelIDs(t *testing.T) {
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/nn.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// A reconciled channel (has a Tunarr id) and one that has never reconciled.
	if err := st.UpsertChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{ID: "ch-live", Name: "Live", Number: 42, TunarrID: "tunarr-1", Status: "live"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{ID: "ch-new", Name: "New", Number: 43, Status: "building"},
	}); err != nil {
		t.Fatal(err)
	}

	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st,
		Auth:  api.NewTokenAuthorizer(adminToken),
		Log:   slog.New(slog.DiscardHandler),
		Guide: fakeGuide{byTunarr: map[string]api.ChannelNowNext{
			"tunarr-1": {Now: &api.NowNextEntry{Title: "On Now"}, Next: &api.NowNextEntry{Title: "Up Next", Gap: true}},
		}},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := do(t, srv, http.MethodGet, "/v1/channels/now-next", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("now-next → %d; a 404 means it routed to get-channel with id=now-next", resp.StatusCode)
	}
	var body struct {
		Channels []api.ChannelNowNext `json:"channels"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)

	if len(body.Channels) != 1 {
		t.Fatalf("got %d entries, want 1 (the never-reconciled channel has nothing airing)", len(body.Channels))
	}
	// The response is keyed by the LOOMARR channel id — the FE never sees Tunarr ids.
	if body.Channels[0].ChannelID != "ch-live" {
		t.Errorf("channelId = %q, want ch-live (the Tunarr id must be translated)", body.Channels[0].ChannelID)
	}
	if body.Channels[0].Now == nil || body.Channels[0].Now.Title != "On Now" {
		t.Errorf("now = %+v", body.Channels[0].Now)
	}
}

// With no Tunarr configured the page still loads: now/next is a nicety on a list view,
// so an absent guide reads as "nothing airing", never an error.
func TestChannelsNowNext_NoGuideConfiguredIsEmptyNotError(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t) // wired without a Guide
	resp := do(t, srv, http.MethodGet, "/v1/channels/now-next", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("now-next without a guide → %d, want 200", resp.StatusCode)
	}
}
