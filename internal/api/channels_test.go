package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/binder"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeChannelSvc records reconcile + purge calls and returns canned cycle-preview output.
type fakeChannelSvc struct {
	reconciles int
	purges     int
	err        error

	// cycle-preview stubs (§8.1)
	cycleCalls  int
	cycleAt     time.Time                      // the `at` the handler passed in
	cycleSlots  []schedule.Slot                // returned slots
	cycleActive schedule.ActiveRuleAttribution // returned attribution
	cycleWindow time.Duration                  // returned window
	cycleErr    error                          // returned error (nil ⇒ f.err path unused)

	// programming/preview draft capture (P6): what the last CyclePreviewDraft received.
	draftLineup []schedule.LineupEntry
	draftPolicy *schedule.ChannelPolicy
}

func (f *fakeChannelSvc) Reconcile(ctx context.Context, id string) error {
	f.reconciles++
	return f.err
}

func (f *fakeChannelSvc) Purge(ctx context.Context, id string) error {
	f.purges++
	return f.err
}

func (f *fakeChannelSvc) CyclePreview(ctx context.Context, id string, at time.Time) (
	time.Time, []schedule.Slot, schedule.ActiveRuleAttribution, time.Duration, error,
) {
	f.cycleCalls++
	f.cycleAt = at
	if f.cycleErr != nil {
		return time.Time{}, nil, schedule.ActiveRuleAttribution{}, 0, f.cycleErr
	}
	// Echo "now" substitution the real engine does: a zero `at` resolves to a fixed
	// deterministic instant so the handler's echoed `at` is assertable.
	resolved := at
	if resolved.IsZero() {
		resolved = time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	}
	return resolved, f.cycleSlots, f.cycleActive, f.cycleWindow, nil
}

// CyclePreviewDraft records the draft it was handed (so a test can assert the handler passed
// the draft lineup/policy through) and otherwise mirrors CyclePreview's echo.
func (f *fakeChannelSvc) CyclePreviewDraft(ctx context.Context, id string, at time.Time,
	draftLineup []schedule.LineupEntry, draftPolicy *schedule.ChannelPolicy) (
	time.Time, []schedule.Slot, schedule.ActiveRuleAttribution, time.Duration, error,
) {
	f.draftLineup = draftLineup
	f.draftPolicy = draftPolicy
	return f.CyclePreview(ctx, id, at)
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
func (f *fakeLiveTVSvc) Reconnect(ctx context.Context) (int, error) {
	f.connects++
	return 1, nil // one tuner reset
}

func newServerWithScheduler(t *testing.T) (*httptest.Server, store.Store, *fakeChannelSvc, *fakeLiveTVSvc) {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/api.db")
	t.Cleanup(func() { _ = st.Close() })
	chSvc := &fakeChannelSvc{}
	ltv := &fakeLiveTVSvc{}
	log := slog.New(slog.DiscardHandler)
	h := api.Router(log, api.Options{
		Store:    st,
		Auth:     testAuthorizer{},
		Log:      log,
		Channels: chSvc,
		LiveTV:   ltv,
		// chSvc satisfies binder.Reconciler (Reconcile(ctx, id) error), so createChannel's
		// lineupFromIntent/policyFromIntent (which now go through the binder) resolve real
		// approved proposals in these tests, same as production wiring.
		Binder: binder.New(st, chSvc, nil, log),
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

// A create with NO id is a hand-made channel: the server assigns a stable `ch_…` id (§7),
// so the "New channel" UI action needs no client-side id scheme. An explicit id is still
// honored (the proposal-approval path relies on that), which TestCreateChannelAdmin covers.
func TestCreateChannelServerAssignsIDWhenOmitted(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"name":"Hand-made","number":7,"strategy":"sequential"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create without id → %d, want 200", resp.StatusCode)
	}
	var body struct {
		ID, Name string
		Number   int
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if !strings.HasPrefix(body.ID, "ch_") {
		t.Errorf("server-assigned id = %q, want a ch_ prefix", body.ID)
	}
	// The channel is really persisted under that id (an empty, hand-made channel — no lineup).
	ch, err := st.GetChannel(context.Background(), body.ID)
	if err != nil {
		t.Fatalf("GetChannel(%q): %v", body.ID, err)
	}
	if ch.Name != "Hand-made" || len(ch.Lineup) != 0 {
		t.Errorf("hand-made channel = %+v (want empty lineup)", ch)
	}
}

func TestCreateChannelRequiresAdmin(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	for _, tok := range []string{"", "wrong"} {
		resp := do(t, srv, http.MethodPost, "/v1/channels", tok,
			`{"id":"c","name":"n","number":1,"strategy":"sequential"}`)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("create with token %q → %d, want 401", tok, resp.StatusCode)
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
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("member reconcile → %d, want 401", resp.StatusCode)
	}
}

// --- cycle preview (GET …/cycle?at=) — the §8.1 time-travel preview ---

// The cycle preview renders the resolved slots + the active-rule attribution + the window,
// and echoes the resolved moment. It passes `at` through to the engine verbatim.
func TestCyclePreview_RendersSlotsAndAttribution(t *testing.T) {
	srv, _, chSvc, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "Trek", 5)
	chSvc.cycleSlots = []schedule.Slot{
		{Kind: schedule.SlotProgram, Title: "Encounter at Farpoint", Key: "tv:655", PartIndex: 0},
		{Kind: schedule.SlotFiller}, // a break gap
		{Kind: schedule.SlotProgram, Title: "The Killing Game (1)", Key: "tv:74", PartIndex: 1},
		{Kind: schedule.SlotPending, Title: "Not yet aired", Key: "tv:99"},
	}
	chSvc.cycleActive = schedule.ActiveRuleAttribution{ID: "r1", Label: "Weekends · Marathon", Priority: 20, Matched: true}
	chSvc.cycleWindow = 24 * time.Hour

	at := "2026-07-25T09:00:00Z" // a Saturday
	resp := do(t, srv, http.MethodGet, "/v1/channels/c1/cycle?at="+at, adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cycle → %d, want 200", resp.StatusCode)
	}
	var body struct {
		At         string `json:"at"`
		ActiveRule struct {
			ID       string `json:"id"`
			Label    string `json:"label"`
			Priority int    `json:"priority"`
			Matched  bool   `json:"matched"`
		} `json:"activeRule"`
		WindowMs int64 `json:"windowMs"`
		Slots    []struct {
			Kind, Title, Key string
			Part             int
		} `json:"slots"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !chSvc.cycleAt.Equal(mustTime(t, at)) {
		t.Errorf("engine got at=%v, want %s (handler must pass `at` through)", chSvc.cycleAt, at)
	}
	if body.At != at {
		t.Errorf("echoed at = %q, want %q", body.At, at)
	}
	if !body.ActiveRule.Matched || body.ActiveRule.Label != "Weekends · Marathon" || body.ActiveRule.Priority != 20 {
		t.Errorf("attribution = %+v, want the marathon rule matched", body.ActiveRule)
	}
	if body.WindowMs != (24 * time.Hour).Milliseconds() {
		t.Errorf("windowMs = %d, want %d", body.WindowMs, (24 * time.Hour).Milliseconds())
	}
	if len(body.Slots) != 4 {
		t.Fatalf("slots = %d, want 4", len(body.Slots))
	}
	if body.Slots[0].Kind != "program" || body.Slots[0].Title != "Encounter at Farpoint" {
		t.Errorf("slot[0] = %+v, want the program", body.Slots[0])
	}
	if body.Slots[1].Kind != "break" || body.Slots[1].Title != "" {
		t.Errorf("slot[1] = %+v, want an empty-title break gap", body.Slots[1])
	}
	if body.Slots[2].Part != 1 {
		t.Errorf("slot[2] part = %d, want 1 (multi-part index preserved)", body.Slots[2].Part)
	}
	if body.Slots[3].Kind != "pending" {
		t.Errorf("slot[3] kind = %q, want pending", body.Slots[3].Kind)
	}
}

// No rule matching → the base-policy attribution (matched:false), so the UI can say
// "no rule is active — base policy" rather than mislabel a moment.
func TestCyclePreview_BasePolicyWhenNoRuleMatches(t *testing.T) {
	srv, _, chSvc, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "Trek", 5)
	chSvc.cycleActive = schedule.ActiveRuleAttribution{Label: "Base policy", Matched: false}

	resp := do(t, srv, http.MethodGet, "/v1/channels/c1/cycle", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cycle → %d, want 200", resp.StatusCode)
	}
	var body struct {
		At         string `json:"at"`
		ActiveRule struct {
			Label   string `json:"label"`
			Matched bool   `json:"matched"`
		} `json:"activeRule"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.ActiveRule.Matched || body.ActiveRule.Label != "Base policy" {
		t.Errorf("no-match attribution = %+v, want base policy", body.ActiveRule)
	}
	// A default (no `at`) request still echoes a concrete resolved moment.
	if body.At == "" {
		t.Error("default-time preview must echo the resolved 'now', not an empty at")
	}
	if chSvc.cycleCalls != 1 {
		t.Errorf("cycle called %d times, want 1", chSvc.cycleCalls)
	}
}

// The preview is a READ — visible to any authenticated user (matching pod preview §8.1).
func TestCyclePreview_VisibleToAnyAuthenticatedUser(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "Trek", 5)
	if resp := do(t, srv, http.MethodGet, "/v1/channels/c1/cycle", memberToken, ""); resp.StatusCode != http.StatusOK {
		t.Errorf("member cycle preview → %d, want 200 (read-only)", resp.StatusCode)
	}
}

// A malformed `at` is a 400 — the model/UI must send RFC3339, not a re-implementation.
func TestCyclePreview_BadTimeIs400(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "Trek", 5)
	if resp := do(t, srv, http.MethodGet, "/v1/channels/c1/cycle?at=saturday-9am", adminToken, ""); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad at → %d, want 400", resp.StatusCode)
	}
}

// An unknown channel is a 404 (the engine reports store.ErrNotFound, the handler maps it).
func TestCyclePreview_UnknownChannelIs404(t *testing.T) {
	srv, _, chSvc, _ := newServerWithScheduler(t)
	chSvc.cycleErr = store.ErrNotFound
	if resp := do(t, srv, http.MethodGet, "/v1/channels/nope/cycle", adminToken, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown channel → %d, want 404", resp.StatusCode)
	}
}

// --- programming/preview + vocabulary (P6) -------------------------------------------------

// The whole-definition draft preview passes the DRAFT lineup + policy through to the engine
// (so the preview reflects the unsaved edit) and renders slots + a never-null pods pool.
func TestProgrammingPreview_PassesDraftToEngine(t *testing.T) {
	srv, _, chSvc, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "Trek", 5)
	chSvc.cycleSlots = []schedule.Slot{{Kind: schedule.SlotProgram, Title: "Encounter at Farpoint", Key: "series:tvdb:655"}}
	chSvc.cycleActive = schedule.ActiveRuleAttribution{Label: "Base policy"}

	body := `{"lineup":[{"key":"series:tvdb:655","name":"TNG"}],"policy":{"ordering":"shuffle"}}`
	resp := do(t, srv, http.MethodPost, "/v1/channels/c1/programming/preview", adminToken, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview → %d, want 200", resp.StatusCode)
	}
	var out struct {
		Slots []struct{ Kind, Title string } `json:"slots"`
		Pods  struct {
			Entries []struct{ Name string } `json:"entries"`
		} `json:"pods"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if chSvc.draftPolicy == nil || chSvc.draftPolicy.Ordering != "shuffle" {
		t.Errorf("draft policy not passed to the engine: %+v", chSvc.draftPolicy)
	}
	if len(chSvc.draftLineup) != 1 || chSvc.draftLineup[0].Key != "series:tvdb:655" {
		t.Errorf("draft lineup not passed to the engine: %+v", chSvc.draftLineup)
	}
	if len(out.Slots) != 1 || out.Slots[0].Title != "Encounter at Farpoint" {
		t.Errorf("slots = %+v, want the previewed program", out.Slots)
	}
	if out.Pods.Entries == nil {
		t.Error("pods.entries must be a slice, never null (an empty pool is a normal state)")
	}
}

// An invalid draft policy is a 422 (validated exactly like a policy write, §4).
func TestProgrammingPreview_InvalidPolicyIs422(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	mkChannel(t, srv, "c1", "Trek", 5)
	body := `{"policy":{"audience":{"ceiling":"TV-BOGUS"}}}`
	if resp := do(t, srv, http.MethodPost, "/v1/channels/c1/programming/preview", adminToken, body); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("bad ceiling → %d, want 422", resp.StatusCode)
	}
}

// The vocabulary endpoint serves the closed WHEN/WHAT/HOW presets to any authenticated user.
func TestProgrammingVocabulary_ServesTheClosedPresets(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	resp := do(t, srv, http.MethodGet, "/v1/programming/vocabulary", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("vocabulary → %d, want 200", resp.StatusCode)
	}
	var v struct {
		When []struct{ Token string } `json:"when"`
		What []struct{ Token string } `json:"what"`
		How  []struct{ Token string } `json:"how"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	if len(v.When) == 0 || len(v.What) == 0 || len(v.How) == 0 {
		t.Fatalf("vocabulary empty: when=%d what=%d how=%d", len(v.When), len(v.What), len(v.How))
	}
	has := func(tokens []struct{ Token string }, want string) bool {
		for _, x := range tokens {
			if x.Token == want {
				return true
			}
		}
		return false
	}
	if !has(v.When, "weekend") || !has(v.How, "marathon") || !has(v.What, "kids") {
		t.Error("expected the weekend/marathon/kids tokens in the vocabulary")
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
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

// --- refine (POST /v1/channels/{id}/refine) ---

// newServerWithSchedulerAndSuggest wires BOTH the channel service and a fake suggest
// service, so refine (which needs the suggester) can be exercised end to end.
func newServerWithSchedulerAndSuggest(t *testing.T) (*httptest.Server, store.Store, *fakeSuggest) {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/refine.db")
	t.Cleanup(func() { _ = st.Close() })
	fs := &fakeSuggest{}
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:    st,
		Auth:     testAuthorizer{},
		Log:      slog.New(slog.DiscardHandler),
		Channels: &fakeChannelSvc{},
		Suggest:  fs,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st, fs
}

func TestRefineChannel_RequiresAdmin(t *testing.T) {
	srv, _, _ := newServerWithSchedulerAndSuggest(t)
	for _, tok := range []string{"", "wrong"} {
		resp := do(t, srv, http.MethodPost, "/v1/channels/c1/refine", tok, `{"change":"more action"}`)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("refine with token %q → %d, want 401", tok, resp.StatusCode)
		}
	}
}

func TestRefineChannel_HandMadeChannel422(t *testing.T) {
	srv, st, _ := newServerWithSchedulerAndSuggest(t)
	// A channel with NO IntentRef (hand-made) can't be refined — there's no job to re-run.
	ch := store.Channel{}
	ch.ID, ch.Name, ch.Number, ch.Strategy, ch.Status = "handmade", "Hand", 3, schedule.Sequential, schedule.StatusBuilding
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	resp := do(t, srv, http.MethodPost, "/v1/channels/handmade/refine", adminToken, `{"change":"more action"}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("refine hand-made channel → %d, want 422", resp.StatusCode)
	}
}

func TestRefineChannel_ReQueuesIntentRefJobWithLineupContext(t *testing.T) {
	srv, st, fs := newServerWithSchedulerAndSuggest(t)
	ctx := context.Background()

	// A channel bound to a suggestion job, with a current lineup.
	if err := st.CreateJob(ctx, store.Job{
		ID: "job-orig", Kind: "suggest", Status: "done",
		IntentJSON: `{"description":"90s action"}`, CreatedBy: "alex",
	}); err != nil {
		t.Fatal(err)
	}
	ch := store.Channel{
		Lineup: []schedule.LineupEntry{
			{Key: "movie:tmdb:603", Title: "The Matrix", Year: 1999},
			{Key: "movie:tmdb:165", Title: "Terminator 2", Year: 1991},
		},
	}
	ch.ID, ch.IntentRef, ch.Name, ch.Number = "c1", "job-orig", "90s Action", 42
	ch.Strategy, ch.Status = schedule.Sequential, schedule.StatusBuilding
	if err := st.UpsertChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}

	resp := do(t, srv, http.MethodPost, "/v1/channels/c1/refine", adminToken,
		`{"change":"add more Schwarzenegger, drop the slow ones"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refine → %d, want 200", resp.StatusCode)
	}
	var body struct{ JobID string }
	_ = json.NewDecoder(resp.Body).Decode(&body)
	// Returns the channel's OWN job id (re-queued in place), so the refined proposal binds
	// back to this channel — the whole point of "reuse the intentRef job".
	if body.JobID != "job-orig" {
		t.Errorf("refine returned jobId %q, want the channel's IntentRef job-orig", body.JobID)
	}
	if fs.refines != 1 || fs.lastJobID != "job-orig" {
		t.Errorf("Refine called %d times on job %q, want 1 on job-orig", fs.refines, fs.lastJobID)
	}
	// The refine intent carried the change + the current lineup as context + kept the
	// original description.
	if fs.last.RefineText != "add more Schwarzenegger, drop the slow ones" {
		t.Errorf("refine intent RefineText = %q", fs.last.RefineText)
	}
	if fs.last.Description != "90s action" {
		t.Errorf("refine intent kept original description? got %q, want '90s action'", fs.last.Description)
	}
	if len(fs.last.CurrentLineup) != 2 || fs.last.CurrentLineup[0].Name != "The Matrix" {
		t.Errorf("refine intent CurrentLineup = %+v, want the 2 seeded titles", fs.last.CurrentLineup)
	}
}

func TestRefineChannel_NotFound(t *testing.T) {
	srv, _, _ := newServerWithSchedulerAndSuggest(t)
	resp := do(t, srv, http.MethodPost, "/v1/channels/nope/refine", adminToken, `{"change":"x"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("refine missing channel → %d, want 404", resp.StatusCode)
	}
}

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
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("patch with token %q → %d, want 401", tok, resp.StatusCode)
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

// --- P3: manual lineup edits (add / remove / reorder via PATCH lineup) ---

// seedChannelWithLineup writes a channel carrying a lineup directly (the create endpoint
// needs an approved proposal for a real lineup; these tests exercise the edit path, so
// they seed the starting state in the store).
func seedChannelWithLineup(t *testing.T, st store.Store, id string, entries ...schedule.LineupEntry) {
	t.Helper()
	err := st.UpsertChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{
			ID: id, Name: "Seed", Number: 5, Strategy: schedule.Sequential,
			Status: schedule.StatusBuilding,
		},
		Lineup: entries,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func keysOf(entries []schedule.LineupEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = string(e.Key)
	}
	return out
}

// A reorder is a whole-list replace with the same keys in a new order. The lineup order
// must follow the payload (sequential/syndication play in order), and it auto-reconciles.
func TestUpdateChannel_LineupReorder(t *testing.T) {
	srv, st, chSvc, _ := newServerWithScheduler(t)
	seedChannelWithLineup(t, st, "c1",
		schedule.LineupEntry{Key: "movie:tmdb:603", Title: "The Matrix", Year: 1999},
		schedule.LineupEntry{Key: "movie:tmdb:165", Title: "Terminator 2", Year: 1991},
	)
	before := chSvc.reconciles

	// Swap the order.
	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken,
		`{"lineup":[{"key":"movie:tmdb:165","name":"Terminator 2","year":1991},`+
			`{"key":"movie:tmdb:603","name":"The Matrix","year":1999}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reorder → %d, want 200", resp.StatusCode)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	got := keysOf(ch.Lineup)
	want := []string{"movie:tmdb:165", "movie:tmdb:603"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("lineup order after reorder = %v, want %v", got, want)
	}
	if chSvc.reconciles != before+1 {
		t.Errorf("lineup edit should auto-reconcile once: %d → %d", before, chSvc.reconciles)
	}
}

// Adding a key and removing another, in one payload. A newly-added key that isn't in the
// library is accepted — it becomes a pending slot at reconcile (§9), the point of P3.
func TestUpdateChannel_LineupAddAndRemove(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	seedChannelWithLineup(t, st, "c1",
		schedule.LineupEntry{Key: "movie:tmdb:603", Title: "The Matrix", Year: 1999},
		schedule.LineupEntry{Key: "movie:tmdb:165", Title: "Terminator 2", Year: 1991},
	)

	// Keep The Matrix, drop Terminator 2, add Predator (not in this store — a pending add).
	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken,
		`{"lineup":[{"key":"movie:tmdb:603","name":"The Matrix","year":1999},`+
			`{"key":"movie:tmdb:106","name":"Predator","year":1987}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add/remove → %d, want 200", resp.StatusCode)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	got := keysOf(ch.Lineup)
	want := []string{"movie:tmdb:603", "movie:tmdb:106"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("lineup after add/remove = %v, want %v", got, want)
	}
}

// A non-nil empty array clears the lineup (distinct from omitting the field, which leaves
// it unchanged — the pointer-optional partial contract).
func TestUpdateChannel_LineupClearVsOmit(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	seedChannelWithLineup(t, st, "c1",
		schedule.LineupEntry{Key: "movie:tmdb:603", Title: "The Matrix", Year: 1999})

	// Omitting lineup leaves it untouched (renames only).
	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken, `{"name":"Renamed"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename → %d", resp.StatusCode)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	if len(ch.Lineup) != 1 {
		t.Fatalf("omitting lineup cleared it: %d entries, want 1", len(ch.Lineup))
	}

	// An explicit empty array clears it.
	resp = do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken, `{"lineup":[]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear → %d", resp.StatusCode)
	}
	ch, _ = st.GetChannel(context.Background(), "c1")
	if len(ch.Lineup) != 0 {
		t.Errorf("explicit [] should clear the lineup: %d entries, want 0", len(ch.Lineup))
	}
}

// A malformed key fails the WHOLE edit (422) — a junk entry must never land, and the
// existing lineup must be untouched on rejection.
func TestUpdateChannel_LineupMalformedKey422(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	seedChannelWithLineup(t, st, "c1",
		schedule.LineupEntry{Key: "movie:tmdb:603", Title: "The Matrix", Year: 1999})

	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken,
		`{"lineup":[{"key":"not-a-real-key","name":"Junk"}]}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("malformed key → %d, want 422", resp.StatusCode)
	}
	// The rejection left the original lineup intact (no partial write).
	ch, _ := st.GetChannel(context.Background(), "c1")
	if len(ch.Lineup) != 1 || ch.Lineup[0].Key != provision.Key("movie:tmdb:603") {
		t.Errorf("a rejected edit changed the lineup: %v", keysOf(ch.Lineup))
	}
}

// The read DTO is lossy (no season range / rating / runtime), so an edit that reuses a key
// must PRESERVE those fields from the existing entry — a reorder must not silently reset a
// series' season scope. This is the correctness crux of P3.
func TestUpdateChannel_LineupPreservesRichFieldsByKey(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	// A series scoped to seasons 1–3 with a rating + runtime the DTO can't carry.
	seedChannelWithLineup(t, st, "c1",
		schedule.LineupEntry{
			Key: "series:tvdb:81189", Title: "Breaking Bad", Year: 2008,
			SeasonMin: 1, SeasonMax: 3, OfficialRating: "TV-MA", RuntimeSec: 2880,
		},
		schedule.LineupEntry{Key: "movie:tmdb:603", Title: "The Matrix", Year: 1999})

	// Reorder (series now second) using ONLY the lossy DTO fields — season/rating/runtime
	// are not in the payload.
	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken,
		`{"lineup":[{"key":"movie:tmdb:603","name":"The Matrix","year":1999},`+
			`{"key":"series:tvdb:81189","name":"Breaking Bad","year":2008}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reorder → %d, want 200", resp.StatusCode)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	var bb *schedule.LineupEntry
	for i := range ch.Lineup {
		if ch.Lineup[i].Key == provision.Key("series:tvdb:81189") {
			bb = &ch.Lineup[i]
		}
	}
	if bb == nil {
		t.Fatal("series entry vanished after reorder")
	}
	if bb.SeasonMin != 1 || bb.SeasonMax != 3 {
		t.Errorf("season scope reset on reorder: got [%d,%d], want [1,3] — the lossy DTO dropped it",
			bb.SeasonMin, bb.SeasonMax)
	}
	if bb.OfficialRating != "TV-MA" {
		t.Errorf("rating reset on reorder: %q, want TV-MA", bb.OfficialRating)
	}
	if bb.RuntimeSec != 2880 {
		t.Errorf("runtime reset on reorder: %d, want 2880", bb.RuntimeSec)
	}
}

// A duplicate key in one payload is rejected (422) — a lineup is a set of distinct titles;
// two entries for the same key would double-schedule and confuse the backfill correlation.
func TestUpdateChannel_LineupRejectsDuplicateKey(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	seedChannelWithLineup(t, st, "c1",
		schedule.LineupEntry{Key: "movie:tmdb:603", Title: "The Matrix", Year: 1999})

	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken,
		`{"lineup":[{"key":"movie:tmdb:603","name":"The Matrix"},`+
			`{"key":"movie:tmdb:603","name":"The Matrix again"}]}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("duplicate key → %d, want 422", resp.StatusCode)
	}
}

// Keys are trimmed before validation + dedupe, so surrounding whitespace neither
// smuggles a "distinct" duplicate past the set nor breaks ParseKey.
func TestUpdateChannel_LineupTrimsKeys(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	seedChannelWithLineup(t, st, "c1")

	// A padded key must validate (trimmed) and land canonicalized.
	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken,
		`{"lineup":[{"key":"  movie:tmdb:603  ","name":"The Matrix"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("padded key → %d, want 200", resp.StatusCode)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	if len(ch.Lineup) != 1 || ch.Lineup[0].Key != provision.Key("movie:tmdb:603") {
		t.Errorf("padded key not trimmed to canonical form: %v", keysOf(ch.Lineup))
	}

	// The same key padded differently in two entries is still one key → duplicate → 422.
	resp = do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken,
		`{"lineup":[{"key":"movie:tmdb:603","name":"a"},{"key":" movie:tmdb:603 ","name":"b"}]}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("whitespace-disguised duplicate → %d, want 422", resp.StatusCode)
	}
}

// Lineup editing is admin-only (same gate as every other channel mutation).
func TestUpdateChannel_LineupRequiresAdmin(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	seedChannelWithLineup(t, st, "c1",
		schedule.LineupEntry{Key: "movie:tmdb:603", Title: "The Matrix", Year: 1999})

	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", "",
		`{"lineup":[]}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("member lineup edit → %d, want 401", resp.StatusCode)
	}
}

// The single-channel GET resolves each lineup entry's `state` from its provision Record,
// so the editor's "not here yet" badge is durable across reloads (§7). A key with NO
// record reads `pending` (a manually-added title nothing has requested yet) — distinct
// from `unavailable` (acquisition gave up).
func TestGetChannel_LineupEntryStateFromRecords(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	ctx := context.Background()
	seed := func(key provision.Key, state provision.State) {
		if err := st.UpsertTitle(ctx, provision.Record{Key: key, State: state}); err != nil {
			t.Fatal(err)
		}
	}
	seed("movie:tmdb:603", provision.Available)    // in the library → available
	seed("movie:tmdb:165", provision.Wanted)       // enqueued → acquiring
	seed("movie:tmdb:9426", provision.Downloading) // grabbed → acquiring
	seed("movie:tmdb:11", provision.Unavailable)   // gave up → unavailable
	// movie:tmdb:106 has NO record → pending.
	seedChannelWithLineup(t, st, "c1",
		schedule.LineupEntry{Key: "movie:tmdb:603", Title: "The Matrix"},
		schedule.LineupEntry{Key: "movie:tmdb:165", Title: "Terminator 2"},
		schedule.LineupEntry{Key: "movie:tmdb:9426", Title: "Point Break"},
		schedule.LineupEntry{Key: "movie:tmdb:11", Title: "Star Wars"},
		schedule.LineupEntry{Key: "movie:tmdb:106", Title: "Predator"},
	)

	resp := do(t, srv, http.MethodGet, "/v1/channels/c1", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Lineup []struct {
			Key   string `json:"key"`
			State string `json:"state"`
		} `json:"lineup"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string, len(body.Lineup))
	for _, e := range body.Lineup {
		got[e.Key] = e.State
	}
	want := map[string]string{
		"movie:tmdb:603":  "available",
		"movie:tmdb:165":  "acquiring",
		"movie:tmdb:9426": "acquiring",
		"movie:tmdb:11":   "unavailable",
		"movie:tmdb:106":  "pending", // no record — the manual-add case
	}
	for key, wantState := range want {
		if got[key] != wantState {
			t.Errorf("entry %s state = %q, want %q", key, got[key], wantState)
		}
	}
}

// The LIST endpoint omits per-entry state (it shows counts, not entries) — so it must not
// pay the per-key Record lookup. A listed channel's entries carry an empty state.
func TestListChannels_OmitsLineupEntryState(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	if err := st.UpsertTitle(context.Background(),
		provision.Record{Key: "movie:tmdb:603", State: provision.Available}); err != nil {
		t.Fatal(err)
	}
	seedChannelWithLineup(t, st, "c1",
		schedule.LineupEntry{Key: "movie:tmdb:603", Title: "The Matrix"})

	resp := do(t, srv, http.MethodGet, "/v1/channels", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Channels []struct {
			Lineup []struct {
				State string `json:"state"`
			} `json:"lineup"`
		} `json:"channels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Channels) != 1 || len(body.Channels[0].Lineup) != 1 {
		t.Fatalf("unexpected list shape: %+v", body)
	}
	// Even though the title IS available, the list leaves state unresolved (empty).
	if s := body.Channels[0].Lineup[0].State; s != "" {
		t.Errorf("list entry state = %q, want empty (list omits per-entry state)", s)
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

// Live TV wiring is no longer a standalone endpoint (config-design §6): it auto-runs on a
// Connections save (settings.autoWireAfterSave) and its status surfaces via the `livetv`
// setup check. The idempotent Connect/Wired behavior is exercised through the settings
// auto-wire path (see settings_test.go) and the connector's own tests; here we only assert
// the check reflects the wired state.
func TestSetupStatusReportsLiveTVCheck(t *testing.T) {
	srv, _, _, ltv := newServerWithScheduler(t)

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
	var check *struct {
		Name string
		OK   bool
		Hint string
	}
	for i := range body.Checks {
		if body.Checks[i].Name == "livetv" {
			check = &body.Checks[i]
		}
	}
	if check == nil {
		t.Fatal("status missing livetv check")
	}
	if check.OK || check.Hint == "" {
		t.Errorf("unwired livetv check = %+v, want not-OK with a hint", *check)
	}

	// Once wiring has happened (auto-wired on a Tunarr save), the check flips to OK.
	ltv.wired = true
	resp = do(t, srv, http.MethodGet, "/v1/setup/status", adminToken, "")
	body.Checks = nil
	_ = json.NewDecoder(resp.Body).Decode(&body)
	for _, c := range body.Checks {
		if c.Name == "livetv" && !c.OK {
			t.Error("livetv check should be OK once Tunarr is wired")
		}
	}
}

func TestSetupStatusRequiresAdmin(t *testing.T) {
	srv, _, _, _ := newServerWithScheduler(t)
	resp := do(t, srv, http.MethodGet, "/v1/setup/status", "", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("member status → %d, want 401", resp.StatusCode)
	}
}

// fakeGuide is a scripted api.GuideReader keyed by TUNARR id (as the real one is).
type fakeGuide struct {
	byTunarr map[string]api.ChannelNowNext
	upcoming map[string][]api.NowNextEntry // per-tunarr-id "what's on later"; nil ⇒ empty
}

func (f fakeGuide) NowNext(context.Context, time.Time) (map[string]api.ChannelNowNext, error) {
	return f.byTunarr, nil
}

func (f fakeGuide) Upcoming(_ context.Context, tunarrID string, _ time.Time, limit int) ([]api.NowNextEntry, error) {
	got := f.upcoming[tunarrID]
	if limit > 0 && len(got) > limit {
		got = got[:limit]
	}
	return got, nil
}

// GET /v1/channels/now-next must resolve to the now/next handler, NOT to
// GET /v1/channels/{id} with id="now-next". A literal segment beats a wildcard in Go
// 1.22's ServeMux, but that is a routing detail worth pinning rather than assuming: if
// precedence ever flipped, the page would 404 on a channel that does not exist.
func TestChannelsNowNext_RoutesAndMapsToLoomarrChannelIDs(t *testing.T) {
	st := openTestStore(t, t.TempDir()+"/nn.db")
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
		Auth:  testAuthorizer{},
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
