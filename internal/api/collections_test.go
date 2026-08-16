package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
)

// fakeCollections is a CollectionService returning a fixed list, or an error.
type fakeCollections struct {
	colls []api.LibraryCollection
	err   error
}

func (f fakeCollections) Collections(context.Context) ([]api.LibraryCollection, error) {
	return f.colls, f.err
}

// collectionsServer wires a server with a collections service AND the live-config seam
// reporting a configured library — the handler gates on both, so a test that sets only the
// service would 501 and prove nothing about the happy path.
func collectionsServer(t *testing.T, svc api.CollectionService) *httptest.Server {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/coll.db")
	t.Cleanup(func() { _ = st.Close() })
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:             st,
		Auth:              testAuthorizer{},
		Log:               slog.New(slog.DiscardHandler),
		Collections:       svc,
		LiveConfig:        func(key string) string { return "http://media.local" },
		LibraryConfigured: func() bool { return true },
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestCollections_ListsForMember(t *testing.T) {
	srv := collectionsServer(t, fakeCollections{colls: []api.LibraryCollection{
		{ID: "bs-1", Name: "Halloween", ChildCount: 12},
	}})

	// A member may read this: it is read-only and exposes no more than the media server
	// already shows them (§7.2), the same posture as /v1/search.
	resp := do(t, srv, http.MethodGet, "/v1/library/collections", memberToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Collections []api.LibraryCollection `json:"collections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Collections) != 1 || body.Collections[0].ID != "bs-1" {
		t.Fatalf("collections = %+v, want the one BoxSet", body.Collections)
	}
	if body.Collections[0].Name != "Halloween" {
		t.Errorf("name = %q, want Halloween — the picker has nothing to show without it",
			body.Collections[0].Name)
	}
}

// ⚠ An operator with no collections must serialize as `[]`, never `null`. A null forces every
// client to treat two shapes as one, and the picker's empty state is the whole point of the
// distinction — "you have no collections yet" is a real answer, not a missing one.
func TestCollections_EmptyIsArrayNotNull(t *testing.T) {
	srv := collectionsServer(t, fakeCollections{colls: nil})

	resp := do(t, srv, http.MethodGet, "/v1/library/collections", memberToken, "")
	defer func() { _ = resp.Body.Close() }()
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if got := string(raw["collections"]); got != "[]" {
		t.Errorf("collections = %s, want [] (a nil slice serialized as null)", got)
	}
}

// §19: anonymous gets no library read.
//
// ⚠ This asserts the MIDDLEWARE, not this route's role marker — a token-less request is 401'd
// before the per-operation role is consulted, so it passes whatever marker the route carries.
// Verified by sabotage: switching to RoleAnonymous still returns 401. Kept because the
// property matters (this must never be an unauthenticated window onto the library), but the
// marker itself is pinned by the member test below, which is the one that can fail.
func TestCollections_AnonymousRefused(t *testing.T) {
	srv := collectionsServer(t, fakeCollections{colls: []api.LibraryCollection{{ID: "bs-1"}}})

	resp := do(t, srv, http.MethodGet, "/v1/library/collections", "", "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Errorf("anonymous status = %d, want 401/403", resp.StatusCode)
	}
}

// The route is deliberately MEMBER-readable, and this is the assertion that pins it: the role
// marker is the only thing standing between a member and a 403 here. Sabotage-verified —
// changing RoleMember to RoleAdmin fails this with 403, where the anonymous test above stays
// green either way.
//
// Member-readable is the §7.2 posture: read-only, showing no more than the media server
// already shows that user, and picking a collection still routes through submit→approve.
func TestCollections_MemberIsNotForbidden(t *testing.T) {
	srv := collectionsServer(t, fakeCollections{colls: []api.LibraryCollection{{ID: "bs-1"}}})

	resp := do(t, srv, http.MethodGet, "/v1/library/collections", memberToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusForbidden {
		t.Error("member got 403 — the route is admin-gated, but a read-only library list is " +
			"member-readable by §7.2 (same posture as /v1/search)")
	}
}

// A media server that errors is a 502 with an explanation, never a 500 and never an empty list
// — an empty list would read as "you have no collections", which is a different and false claim.
func TestCollections_UpstreamErrorIs502(t *testing.T) {
	srv := collectionsServer(t, fakeCollections{err: errors.New("dial tcp: connection refused")})

	resp := do(t, srv, http.MethodGet, "/v1/library/collections", memberToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

// No service wired (no library configured) ⇒ 501, the same nil-semantics every other optional
// service uses — not a 404, which would read as "this feature does not exist".
func TestCollections_UnwiredIs501(t *testing.T) {
	srv, _ := newServer(t) // no Collections option

	resp := do(t, srv, http.MethodGet, "/v1/library/collections", memberToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", resp.StatusCode)
	}
}
