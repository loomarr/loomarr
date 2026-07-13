package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeChannelSvc records reconcile calls.
type fakeChannelSvc struct {
	reconciles int
	err        error
}

func (f *fakeChannelSvc) Reconcile(ctx context.Context, id string) error {
	f.reconciles++
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
