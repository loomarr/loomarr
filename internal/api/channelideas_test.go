package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/ideas"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/proposalworkflow"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

type fakeIdeaLibrary struct {
	items []ideas.Item
	calls int
}

func (f *fakeIdeaLibrary) IdeaItems(context.Context) ([]ideas.Item, error) {
	f.calls++
	return f.items, nil
}

type ideasWire struct {
	Ideas []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Pitch        string `json:"pitch"`
		Requested    bool   `json:"requested"`
		RequestJobID string `json:"requestJobId"`
		Facet        string `json:"facet"`
		Value        string `json:"value"`
		Reason       struct {
			Kind         string `json:"kind"`
			Count        int    `json:"count"`
			HolidayID    string `json:"holidayId"`
			HolidayLabel string `json:"holidayLabel"`
			StartsAtMs   int64  `json:"startsAtMs"`
		} `json:"reason"`
		Keys       []string `json:"keys"`
		Movies     int      `json:"movies"`
		Series     int      `json:"series"`
		InLibrary  int      `json:"inLibrary"`
		ToDownload int      `json:"toDownload"`
	} `json:"ideas"`
}

func (w ideasWire) ids() []string {
	out := make([]string, 0, len(w.Ideas))
	for _, i := range w.Ideas {
		out = append(out, i.ID)
	}
	return out
}

// recordingNotifier counts the "a request is waiting" notices admins get.
type recordingNotifier struct {
	mu        sync.Mutex
	submitted []store.Proposal
}

func (n *recordingNotifier) ProposalSubmitted(_ context.Context, p store.Proposal) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.submitted = append(n.submitted, p)
}
func (n *recordingNotifier) ProposalApproved(context.Context, store.Proposal, string) {}
func (n *recordingNotifier) ProposalDeclined(context.Context, store.Proposal)         {}

func (n *recordingNotifier) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.submitted)
}

func newIdeasHarness(t *testing.T, lib *fakeIdeaLibrary, now time.Time) *apiHarness {
	t.Helper()
	return newIdeasHarnessNotifying(t, lib, now, &recordingNotifier{})
}

// newIdeasHarnessNotifying wires the REAL suggest service and durable workflow, with no LLM
// (nil suggester): an idea request must reach the real approval queue without the model.
func newIdeasHarnessNotifying(t *testing.T, lib *fakeIdeaLibrary, now time.Time, notify *recordingNotifier) *apiHarness {
	t.Helper()
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	ms.Accounts = map[string]testkit.Account{
		"boss": {Password: "pw", ID: "u-boss", IsAdmin: true},
		"kid":  {Password: "pw", ID: "u-kid", IsAdmin: false},
	}
	return startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		mediaServer := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
		mgr := auth.NewManager(defaults.Store, time.Hour, time.Now)
		seedImported(t, defaults.Store, "u-boss", "boss", store.RoleAdmin)
		seedImported(t, defaults.Store, "u-kid", "kid", store.RoleMember)
		var next atomic.Int64
		newID := func() string { return fmt.Sprintf("idea-req-%d", next.Add(1)) }
		workflow := proposalworkflow.New(defaults.Store, newID, func() time.Time { return now })
		service := suggest.NewService(defaults.Store, nil, suggest.Config{}, newID, func() time.Time { return now }, defaults.Log).
			WithDurableWorkflow(workflow).
			WithProposalNotifier(notify)
		return api.Router(defaults.Log, api.Options{
			Suggest:          service,
			ProposalWorkflow: workflow,
			Store:            defaults.Store,
			Auth:             api.NewSessionAuthorizer(mgr, "break-glass-token"),
			Log:              defaults.Log,
			Login:            auth.NewLoginService(mediaServer, defaults.Store, mgr, nil, time.Now),
			Sessions:         mgr,
			CookieSecure:     "false",
			IdeaLibrary:      lib,
			Now:              func() time.Time { return now },
		})
	})
}

func ideasCall(t *testing.T, srv *httptest.Server, method, path string, session *http.Cookie) (int, ideasWire) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, nil)
	if session != nil {
		req.AddCookie(session)
		req.Header.Set("X-Loomarr-Csrf", "1")
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out ideasWire
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// Home's Channel ideas (#1665) over HTTP, with no LLM anywhere: library facets that no channel
// plays and a holiday ahead, each with a typed reason and its numbers. A hide is the person's own
// and undoable.
func TestChannelIdeasFromLibraryAndCalendar(t *testing.T) {
	t.Parallel()
	lib := &fakeIdeaLibrary{}
	for i := 1; i <= 7; i++ {
		lib.items = append(lib.items, ideas.Item{Keys: []provision.Key{provision.Key(fmt.Sprintf("movie:tmdb:%d", i))},
			MediaType: provision.Movie, Name: fmt.Sprintf("Film %d", i), Genres: []string{"Comedy"}})
	}
	for i := 20; i <= 22; i++ {
		lib.items = append(lib.items, ideas.Item{Keys: []provision.Key{provision.Key(fmt.Sprintf("movie:tmdb:%d", i))},
			MediaType: provision.Movie, Name: fmt.Sprintf("Haunted night %d", i)})
	}
	now := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
	h := newIdeasHarness(t, lib, now)
	if _, err := h.Store.SaveChannel(context.Background(), store.Channel{Channel: schedule.Channel{
		ID: "ch-1", Name: "Comedy one", Number: 1, Strategy: schedule.Shuffle, Status: schedule.StatusLive,
	}, Lineup: []schedule.LineupEntry{{Key: "movie:tmdb:1"}}}); err != nil {
		t.Fatal(err)
	}
	kid := login(t, h.Server, "kid", "pw")
	boss := login(t, h.Server, "boss", "pw")

	code, got := ideasCall(t, h.Server, http.MethodGet, "/v1/discovery/ideas", kid)
	if code != http.StatusOK || fmt.Sprint(got.ids()) != "[holiday:halloween genre:comedy]" {
		t.Fatalf("member ideas = %d %v, want the holiday then comedy", code, got.ids())
	}
	holiday, comedy := got.Ideas[0], got.Ideas[1]
	if holiday.Reason.Kind != "holiday" || holiday.Reason.HolidayID != "halloween" || holiday.Reason.HolidayLabel != "Halloween" ||
		holiday.Reason.Count != 3 || holiday.Reason.StartsAtMs != time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Errorf("holiday reason = %+v", holiday.Reason)
	}
	if comedy.Reason.Kind != "unaired" || comedy.Reason.Count != 6 || len(comedy.Keys) != 6 ||
		comedy.Movies != 6 || comedy.InLibrary != 6 || comedy.ToDownload != 0 || comedy.Value != "Comedy" {
		t.Errorf("comedy idea = %+v, want 6 unaired films, all in the library", comedy)
	}

	if code, _ := ideasCall(t, h.Server, http.MethodPut, "/v1/me/hidden-ideas/genre:comedy", kid); code != http.StatusNoContent {
		t.Fatalf("hide = %d, want 204", code)
	}
	if _, got := ideasCall(t, h.Server, http.MethodGet, "/v1/discovery/ideas", kid); fmt.Sprint(got.ids()) != "[holiday:halloween]" {
		t.Errorf("after hide the member sees %v, want only the holiday", got.ids())
	}
	if _, got := ideasCall(t, h.Server, http.MethodGet, "/v1/discovery/ideas", boss); fmt.Sprint(got.ids()) != "[holiday:halloween genre:comedy]" {
		t.Errorf("another person sees %v, want their own unhidden ideas", got.ids())
	}
	if code, _ := ideasCall(t, h.Server, http.MethodDelete, "/v1/me/hidden-ideas/genre:comedy", kid); code != http.StatusNoContent {
		t.Fatalf("undo = %d, want 204", code)
	}
	if _, got := ideasCall(t, h.Server, http.MethodGet, "/v1/discovery/ideas", kid); fmt.Sprint(got.ids()) != "[holiday:halloween genre:comedy]" {
		t.Errorf("after undo the member sees %v", got.ids())
	}
}

func requestIdea(t *testing.T, srv *httptest.Server, ideaID string, session *http.Cookie) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/discovery/ideas/"+ideaID+"/request", nil)
	if session != nil {
		req.AddCookie(session)
		req.Header.Set("X-Loomarr-Csrf", "1")
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out struct {
		JobID string `json:"jobId"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out.JobID
}

// A member turns an idea into a channel request (#1720) with the LLM nowhere in the harness: the
// idea's name, pitch and library titles become a submitted proposal in the real approval queue,
// admins hear about it, and the idea then reads as requested. Asking again returns the same
// request. Admins don't request ideas (H4).
func TestChannelIdeaRequestQueuesAMemberRequestWithoutTheLLM(t *testing.T) {
	t.Parallel()
	lib := &fakeIdeaLibrary{}
	for i := 1; i <= 7; i++ {
		lib.items = append(lib.items, ideas.Item{Keys: []provision.Key{provision.Key(fmt.Sprintf("movie:tmdb:%d", i))},
			// One per decade, so no decade idea competes; the newest is Film 7.
			MediaType: provision.Movie, Name: fmt.Sprintf("Film %d", i), Year: 1950 + 10*i, Genres: []string{"Comedy"}})
	}
	// Late spring: no holiday within six weeks, so comedy is the only idea.
	now := time.Date(2026, time.May, 1, 12, 0, 0, 0, time.UTC)
	notify := &recordingNotifier{}
	h := newIdeasHarnessNotifying(t, lib, now, notify)
	if _, err := h.Store.SaveChannel(context.Background(), store.Channel{Channel: schedule.Channel{
		ID: "ch-1", Name: "Comedy one", Number: 1, Strategy: schedule.Shuffle, Status: schedule.StatusLive,
	}, Lineup: []schedule.LineupEntry{{Key: "movie:tmdb:1"}}}); err != nil {
		t.Fatal(err)
	}
	kid := login(t, h.Server, "kid", "pw")
	boss := login(t, h.Server, "boss", "pw")

	code, got := ideasCall(t, h.Server, http.MethodGet, "/v1/discovery/ideas", kid)
	if code != http.StatusOK || fmt.Sprint(got.ids()) != "[genre:comedy]" {
		t.Fatalf("ideas = %d %v", code, got.ids())
	}
	comedy := got.Ideas[0]
	if comedy.Name != "Comedy Movies" || comedy.Pitch != "Every comedy title in your library that no channel plays yet, on one channel." ||
		comedy.Requested || comedy.RequestJobID != "" {
		t.Fatalf("comedy card = %+v", comedy)
	}

	if code, _ := requestIdea(t, h.Server, "genre:comedy", boss); code != http.StatusForbidden {
		t.Fatalf("admin request = %d, want 403 (ideas are members-only, H4)", code)
	}
	code, jobID := requestIdea(t, h.Server, "genre:comedy", kid)
	if code != http.StatusAccepted || jobID == "" {
		t.Fatalf("member request = %d %q, want 202 with a job", code, jobID)
	}

	queue, err := h.Store.ListProposalsByStatus(context.Background(), "submitted")
	if err != nil || len(queue) != 1 {
		t.Fatalf("approval queue = %+v, %v", queue, err)
	}
	if queue[0].JobID != jobID || queue[0].CreatedBy != "u-kid" {
		t.Fatalf("queued request = %+v, want the member's own, on job %s", queue[0], jobID)
	}
	var payload suggest.Proposal
	if err := json.Unmarshal([]byte(queue[0].ProposalJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ChannelName != "Comedy Movies" || payload.FromIdea != "genre:comedy" ||
		payload.Intent.Description != comedy.Pitch || len(payload.Lineup) != 6 || len(payload.Acquisitions) != 0 {
		t.Fatalf("queued proposal = %+v", payload)
	}
	// Newest first, grounded by the key's own id, every title already in the library; the title on
	// a channel stays off.
	if first := payload.Lineup[0]; first.TMDBID != 7 || first.Name != "Film 7" || !first.InLibrary {
		t.Fatalf("first lineup item = %+v", first)
	}
	for _, item := range payload.Lineup {
		if item.TMDBID == 1 {
			t.Fatalf("the lineup carries a title a channel already plays: %+v", item)
		}
	}
	if notify.count() != 1 {
		t.Fatalf("admins were told about %d requests, want 1", notify.count())
	}

	if _, got := ideasCall(t, h.Server, http.MethodGet, "/v1/discovery/ideas", kid); len(got.Ideas) != 1 ||
		!got.Ideas[0].Requested || got.Ideas[0].RequestJobID != jobID {
		t.Fatalf("after requesting, the card = %+v, want requested on job %s", got.Ideas, jobID)
	}
	if code, again := requestIdea(t, h.Server, "genre:comedy", kid); code != http.StatusAccepted || again != jobID {
		t.Fatalf("second request = %d %q, want the same job %s", code, again, jobID)
	}
	if queue, _ := h.Store.ListProposalsByStatus(context.Background(), "submitted"); len(queue) != 1 || notify.count() != 1 {
		t.Fatalf("asking again queued %d requests and sent %d notices, want 1 and 1", len(queue), notify.count())
	}
	if code, _ := requestIdea(t, h.Server, "genre:western", kid); code != http.StatusNotFound {
		t.Fatalf("unknown idea = %d, want 404", code)
	}
}

// Signed out is refused; ideas belong to a person because hides do.
func TestChannelIdeasRequireSignIn(t *testing.T) {
	t.Parallel()
	h := newIdeasHarness(t, &fakeIdeaLibrary{}, time.Now())
	if code, _ := ideasCall(t, h.Server, http.MethodGet, "/v1/discovery/ideas", nil); code != http.StatusUnauthorized {
		t.Errorf("anonymous ideas = %d, want 401", code)
	}
	if code, _ := ideasCall(t, h.Server, http.MethodPut, "/v1/me/hidden-ideas/genre:comedy", nil); code != http.StatusUnauthorized {
		t.Errorf("anonymous hide = %d, want 401", code)
	}
}
