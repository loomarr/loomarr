package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeSessions implements api.SessionManager. The API layer must not care how hashes are
// produced — it only surfaces them and passes them back for revocation.
type fakeSessions struct {
	list    []store.Session
	revoked []string
	err     error
}

func (f *fakeSessions) Revoke(context.Context, string) error { return nil }
func (f *fakeSessions) List(context.Context, string) ([]store.Session, error) {
	return f.list, f.err
}
func (f *fakeSessions) RevokeHash(_ context.Context, hash string) error {
	f.revoked = append(f.revoked, hash)
	return nil
}

func newSessionsServer(t *testing.T) (*httptest.Server, *fakeSessions, store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/s.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.UpsertUser(context.Background(), store.User{ID: "u1", Name: "Ada", Role: store.RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	fs := &fakeSessions{}
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:    st,
		Auth:     api.NewTokenAuthorizer(adminToken),
		Log:      slog.New(slog.DiscardHandler),
		Sessions: fs,
	}))
	t.Cleanup(srv.Close)
	return srv, fs, st
}

// §19 negative case: session routes expose who is signed in and can sign them out, so a
// member must be refused both.
func TestSessions_RequireAdmin(t *testing.T) {
	srv, _, _ := newSessionsServer(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/users/u1/sessions"},
		{http.MethodDelete, "/v1/sessions/abc123"},
	} {
		if resp := do(t, srv, tc.method, tc.path, "", ""); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s without admin → %d, want 403", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// The list carries a revocable handle and the timestamps an admin judges staleness by.
// It must NEVER carry anything that could authenticate — the id is the stored hash.
func TestSessions_ListShapesForReview(t *testing.T) {
	srv, fs, _ := newSessionsServer(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	fs.list = []store.Session{
		{TokenHash: "hash-a", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	}

	resp := do(t, srv, http.MethodGet, "/v1/users/u1/sessions", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Sessions []struct {
			ID        string `json:"id"`
			CreatedAt int64  `json:"createdAt"`
			ExpiresAt int64  `json:"expiresAt"`
			Current   bool   `json:"current"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(body.Sessions))
	}
	got := body.Sessions[0]
	if got.ID != "hash-a" {
		t.Errorf("id = %q, want the stored hash (the revocation handle)", got.ID)
	}
	if got.CreatedAt != now.UnixMilli() || got.ExpiresAt != now.Add(time.Hour).UnixMilli() {
		t.Errorf("timestamps = %d/%d, want %d/%d", got.CreatedAt, got.ExpiresAt, now.UnixMilli(), now.Add(time.Hour).UnixMilli())
	}
	// Authenticated by bearer token, not a cookie — so no row is the caller's own.
	if got.Current {
		t.Error("current = true for a token-authenticated caller that holds no session cookie")
	}
}

// A 404 on an unknown user, so an admin acting on a stale user list gets a real answer
// rather than a confusing empty session list.
func TestSessions_UnknownUserIs404(t *testing.T) {
	srv, _, _ := newSessionsServer(t)
	if resp := do(t, srv, http.MethodGet, "/v1/users/nope/sessions", adminToken, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown user → %d, want 404", resp.StatusCode)
	}
}

// Revocation is idempotent by design: the list an admin clicks from can go stale between
// render and click (expiry, or the user signing out), and erroring there would be noise
// about an outcome they already have.
func TestSessions_RevokeIsIdempotent(t *testing.T) {
	srv, fs, _ := newSessionsServer(t)
	for range 2 {
		resp := do(t, srv, http.MethodDelete, "/v1/sessions/hash-a", adminToken, "")
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
			t.Fatalf("revoke → %d, want 2xx", resp.StatusCode)
		}
	}
	if len(fs.revoked) != 2 || fs.revoked[0] != "hash-a" {
		t.Errorf("revoked = %v, want the hash passed through twice", fs.revoked)
	}
}

// GET /v1/users PANICKED in production while every unit test passed: quota accounting
// reads suggest.max_acquisitions, an INT setting, and it was routed through the string
// config seam — settings.String panics on a type mismatch. Tests never reached it because
// they leave the seam nil, so the read short-circuited to "".
//
// This wires an int seam that would have panicked under the old code, so the Users list
// is exercised on the path a real install actually takes.
func TestListUsers_ReadsTheIntConfigSeam(t *testing.T) {
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/u.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.UpsertUser(context.Background(), store.User{ID: "u1", Name: "Ada", Role: store.RoleAdmin}); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st,
		Auth:  api.NewTokenAuthorizer(adminToken),
		Log:   slog.New(slog.DiscardHandler),
		// The seam a real composition root wires. Its absence is what hid the bug.
		LiveConfigInt: func(key string) int {
			if key == "suggest.max_acquisitions" {
				return 7
			}
			return 0
		},
	}))
	t.Cleanup(srv.Close)

	resp := do(t, srv, http.MethodGet, "/v1/users", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list users → %d, want 200 (a 500 here is the panic)", resp.StatusCode)
	}
	var body struct {
		Users []struct {
			Quota          int `json:"quota"`
			EffectiveQuota int `json:"effectiveQuota"`
		} `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Users) != 1 {
		t.Fatalf("got %d users, want 1", len(body.Users))
	}
	// Quota 0 means "use the default", which is precisely the path that read the setting.
	if body.Users[0].EffectiveQuota != 7 {
		t.Errorf("effectiveQuota = %d, want the configured default 7", body.Users[0].EffectiveQuota)
	}
}
