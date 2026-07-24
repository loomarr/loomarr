package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeIconSvc records the channel it was asked about, so a test can prove the handler
// passes the id through rather than suggesting icons for some other channel.
type fakeIconSvc struct {
	asked       []string
	suggestions []api.IconSuggestion
	err         error
}

func (f *fakeIconSvc) IconSuggestions(_ context.Context, channelID string) ([]api.IconSuggestion, error) {
	f.asked = append(f.asked, channelID)
	return f.suggestions, f.err
}

func newIconsServer(t *testing.T) (*httptest.Server, store.Store, *fakeIconSvc) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/icons.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.UpsertChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{ID: "ch-1", Name: "Star Trek", Number: 42, Status: "live"},
	}); err != nil {
		t.Fatal(err)
	}
	fi := &fakeIconSvc{}
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st,
		Auth:  api.NewTokenAuthorizer(adminToken),
		Log:   slog.New(slog.DiscardHandler),
		Icons: fi,
	}))
	t.Cleanup(srv.Close)
	return srv, st, fi
}

// The endpoint renders the candidate posters the service resolved from the channel's
// own lineup — e.g. a Star Trek channel offering its five series' posters (§icon P2).
func TestChannelIconSuggestions_RendersCandidates(t *testing.T) {
	srv, _, fi := newIconsServer(t)
	fi.suggestions = []api.IconSuggestion{
		{Title: "Star Trek: The Next Generation", URL: "https://image.tmdb.org/t/p/w500/tng.jpg"},
		{Title: "Star Trek: Deep Space Nine", URL: "https://image.tmdb.org/t/p/w500/ds9.jpg"},
	}

	resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1/icon-suggestions", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("icon-suggestions → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Suggestions []api.IconSuggestion `json:"suggestions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Suggestions) != 2 {
		t.Fatalf("suggestions = %d, want 2", len(body.Suggestions))
	}
	if body.Suggestions[0].Title != "Star Trek: The Next Generation" || body.Suggestions[0].URL != "https://image.tmdb.org/t/p/w500/tng.jpg" {
		t.Errorf("suggestion[0] = %+v", body.Suggestions[0])
	}
	if len(fi.asked) != 1 || fi.asked[0] != "ch-1" {
		t.Errorf("asked = %v, want exactly [ch-1]", fi.asked)
	}
}

// An empty candidate set must render as [] rather than null, so the FE renders an
// empty state instead of guarding a case that never means failure.
func TestChannelIconSuggestions_EmptyIsEmptyArray(t *testing.T) {
	srv, _, fi := newIconsServer(t)
	fi.suggestions = nil

	resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1/icon-suggestions", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("icon-suggestions → %d, want 200", resp.StatusCode)
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if got := string(raw["suggestions"]); got != "[]" {
		t.Errorf("suggestions = %s, want [] (never null)", got)
	}
}

// Read-only: any authenticated user may fetch icon suggestions, matching get-channel.
func TestChannelIconSuggestions_VisibleToAnyAuthenticatedUser(t *testing.T) {
	srv, _, _ := newIconsServer(t)
	if resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1/icon-suggestions", "", ""); resp.StatusCode != http.StatusOK {
		t.Errorf("member request → %d, want 200 (read-only)", resp.StatusCode)
	}
}

func TestChannelIconSuggestions_UnknownChannelIs404(t *testing.T) {
	srv, _, _ := newIconsServer(t)
	if resp := do(t, srv, http.MethodGet, "/v1/channels/nope/icon-suggestions", adminToken, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown channel → %d, want 404", resp.StatusCode)
	}
}

// With no icon service wired (TMDB unconfigured), the route reports unavailable
// rather than 500 — the same nil-service contract every optional TMDB-gated feature
// follows (search, suggest, …).
func TestChannelIconSuggestions_501WhenNoService(t *testing.T) {
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/icons2.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.UpsertChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{ID: "ch-1", Name: "Star Trek", Number: 42, Status: "live"},
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st,
		Auth:  api.NewTokenAuthorizer(adminToken),
		Log:   slog.New(slog.DiscardHandler),
		// Icons intentionally omitted (nil).
	}))
	t.Cleanup(srv.Close)

	resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1/icon-suggestions", adminToken, "")
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("icon-suggestions with no service → %d, want 501", resp.StatusCode)
	}
}
