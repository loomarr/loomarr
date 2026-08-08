package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

const adminToken = "test-admin-token"

// memberToken authenticates as a MEMBER in tests.
//
// ⚠ **Its absence is why the anonymous-read gap survived review.** The production
// `tokenAuthorizer` resolves admin-or-anonymous only — by design, since API_TOKEN is a
// break-glass admin credential (§11). So member-vs-anonymous was structurally untestable in
// the default harness, and four tests asserting "a member can read this" were passing NO
// token at all: they proved an ANONYMOUS caller could read it, under names that said
// otherwise.
//
// Deliberately a test-only authorizer rather than a member branch on the real one: adding a
// second credential to shipped auth code so a test can express itself is how production
// grows a path that only tests use.
const memberToken = "test-member-token"

// testAuthorizer resolves two fixed bearer tokens to two roles. It exists so a test can say
// "member" and mean it.
type testAuthorizer struct{}

func (testAuthorizer) Authorize(r *http.Request) api.Role {
	switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
	case adminToken:
		return api.RoleAdmin
	case memberToken:
		return api.RoleMember
	default:
		return api.RoleAnonymous
	}
}

func newServer(t *testing.T) (*httptest.Server, store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/api.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:        st,
		Auth:         testAuthorizer{},
		Log:          slog.New(slog.DiscardHandler),
		BackupSQLite: store.SQLiteBackuper(st),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st
}

func do(t *testing.T, srv *httptest.Server, method, path, token, body string) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, srv.URL+path, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// OpenAPI is 3.1 and its State enum equals the code enum (§7.1).
func TestOpenAPISpec(t *testing.T) {
	spec, err := api.ExportOpenAPI(slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	s := string(spec)
	if !strings.Contains(s, "openapi: 3.1") {
		t.Error("spec is not OpenAPI 3.1")
	}
	for _, st := range []string{"wanted", "requested", "downloading", "available", "unavailable"} {
		if !strings.Contains(s, "- "+st) {
			t.Errorf("spec State enum missing %q (must equal the code enum, §7.1)", st)
		}
	}
}

// Enqueue requires admin; a valid admin token creates the title as `wanted`.
func TestEnqueueTitleAdmin(t *testing.T) {
	srv, _ := newServer(t)
	resp := do(t, srv, http.MethodPost, "/v1/titles", adminToken,
		`{"mediaType":"movie","tmdbId":1111867,"name":"In Flames"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enqueue → %d, want 200", resp.StatusCode)
	}
	var body struct{ Key, State string }
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Key != "movie:tmdb:1111867" || body.State != "wanted" {
		t.Errorf("enqueue body = %+v, want key movie:tmdb:1111867 state wanted", body)
	}
}

// Enqueue is idempotent (§4 inv. 3): a second identical POST returns the current
// record, not a duplicate or error.
func TestEnqueueIdempotent(t *testing.T) {
	srv, st := newServer(t)
	for i := 0; i < 2; i++ {
		resp := do(t, srv, http.MethodPost, "/v1/titles", adminToken, `{"mediaType":"movie","tmdbId":5}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("enqueue %d → %d", i, resp.StatusCode)
		}
	}
	// Exactly one wanted row.
	recs, _ := st.ListTitlesByState(context.Background(), "wanted")
	if len(recs) != 1 {
		t.Errorf("idempotent enqueue produced %d rows, want 1", len(recs))
	}
}

// POST/DELETE /v1/titles require admin (§7) — anonymous and wrong-token → 403.
func TestTitlesMutationRequiresAdmin(t *testing.T) {
	srv, _ := newServer(t)
	for _, tok := range []string{"", "wrong"} {
		resp := do(t, srv, http.MethodPost, "/v1/titles", tok, `{"mediaType":"movie","tmdbId":1}`)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("POST with token %q → %d, want 401", tok, resp.StatusCode)
		}
	}
}

// list requires the state filter (§7); missing → 400.
func TestListRequiresState(t *testing.T) {
	srv, _ := newServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/titles", adminToken, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("list without state → %d, want 400", resp.StatusCode)
	}
}

// GET a missing title → 404.
func TestGetMissingTitle(t *testing.T) {
	srv, _ := newServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/titles/movie:tmdb:404", adminToken, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get missing → %d, want 404", resp.StatusCode)
	}
}

// DELETE gives up a title (→ unavailable), admin only.
func TestDeleteTitle(t *testing.T) {
	srv, st := newServer(t)
	_ = do(t, srv, http.MethodPost, "/v1/titles", adminToken, `{"mediaType":"movie","tmdbId":7}`)
	resp := do(t, srv, http.MethodDelete, "/v1/titles/movie:tmdb:7", adminToken, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete → %d, want 204", resp.StatusCode)
	}
	rec, _ := st.GetTitle(context.Background(), "movie:tmdb:7")
	if rec.State != "unavailable" {
		t.Errorf("deleted title state = %s, want unavailable (audit-preserving give-up)", rec.State)
	}
}

// SQLite backend serves a backup snapshot; admin only.
func TestBackupSQLite(t *testing.T) {
	srv, _ := newServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/backup", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("backup → %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("backup content-type = %q", ct)
	}
	b, _ := io.ReadAll(resp.Body)
	if len(b) == 0 || string(b[:15]) != "SQLite format 3" {
		t.Error("backup body is not a SQLite database snapshot")
	}
}

// Backup requires admin.
func TestBackupRequiresAdmin(t *testing.T) {
	srv, _ := newServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/backup", "", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("backup without admin → %d, want 401", resp.StatusCode)
	}
}

// The API reference is offline: no external (CDN) asset references (§7.1).
func TestDocsOffline(t *testing.T) {
	srv, _ := newServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/reference", "", "")
	b, _ := io.ReadAll(resp.Body)
	page := string(b)
	if strings.Contains(page, "cdn.") || strings.Contains(page, "unpkg") || strings.Contains(page, "jsdelivr") {
		t.Error("/v1/reference references a CDN — violates the offline rule (§7.1)")
	}
	// ⚠ Its two spec links are hardcoded in a const while the real paths come from
	// cfg.OpenAPIPath, so nothing but this connects them. They pointed at the pre-/v1
	// /openapi.json until it was spotted by hand — a reference page whose only links 404.
	for _, link := range []string{"/v1/openapi.json", "/v1/openapi.yaml"} {
		if !strings.Contains(page, link) {
			t.Errorf("the reference page does not link %s; its links have drifted from cfg.OpenAPIPath", link)
			continue
		}
		if r := do(t, srv, http.MethodGet, link, "", ""); r.StatusCode != http.StatusOK {
			t.Errorf("the reference page links %s, which answers %d", link, r.StatusCode)
		}
	}
}
