package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/ideas"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
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
		ID     string `json:"id"`
		Facet  string `json:"facet"`
		Value  string `json:"value"`
		Reason struct {
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

func newIdeasHarness(t *testing.T, lib *fakeIdeaLibrary, now time.Time) *apiHarness {
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
		return api.Router(defaults.Log, api.Options{
			Store:        defaults.Store,
			Auth:         api.NewSessionAuthorizer(mgr, "break-glass-token"),
			Log:          defaults.Log,
			Login:        auth.NewLoginService(mediaServer, defaults.Store, mgr, nil, time.Now),
			Sessions:     mgr,
			CookieSecure: "false",
			IdeaLibrary:  lib,
			Now:          func() time.Time { return now },
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
