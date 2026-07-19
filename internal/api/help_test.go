package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

func newHelpServer(t *testing.T, ready api.ReadyFunc) *httptest.Server {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/h.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st,
		Auth:  api.NewTokenAuthorizer(adminToken),
		Log:   slog.New(slog.DiscardHandler),
		Ready: ready,
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Help ships inside the binary (§13), so it works air-gapped — no network, no CDN.
func TestHelp_ListsEmbeddedPages(t *testing.T) {
	srv := newHelpServer(t, nil)
	resp := do(t, srv, http.MethodGet, "/v1/docs", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list docs → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Docs []struct{ Slug, Title string } `json:"docs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Docs) == 0 {
		t.Fatal("no help pages listed — the Help section would render empty")
	}
	var found bool
	for _, d := range body.Docs {
		if d.Slug == "troubleshooting" {
			found = true
			if d.Title == "" {
				t.Error("troubleshooting has no title for the Help nav")
			}
		}
	}
	if !found {
		t.Errorf("troubleshooting is missing; every setup check deep-links into it. got %+v", body.Docs)
	}
}

// Markdown, not HTML: the FE renders it AND searches it client-side (§7.2), which needs
// the source rather than a pre-rendered blob.
func TestHelp_ServesMarkdownSource(t *testing.T) {
	srv := newHelpServer(t, nil)
	resp := do(t, srv, http.MethodGet, "/v1/docs/troubleshooting", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get doc → %d, want 200", resp.StatusCode)
	}
	var body struct{ Slug, Title, Markdown string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Markdown, "## ") {
		t.Error("payload has no markdown headings — is it pre-rendered HTML?")
	}
	if !strings.Contains(body.Markdown, "Tunarr") {
		t.Error("troubleshooting content looks wrong: no Tunarr section")
	}
}

func TestHelp_UnknownPageIs404(t *testing.T) {
	srv := newHelpServer(t, nil)
	if resp := do(t, srv, http.MethodGet, "/v1/docs/nope", adminToken, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown page → %d, want 404", resp.StatusCode)
	}
}

// Help is readable by members: it is documentation, and a member hitting a problem needs
// it at least as much as an admin does.
func TestHelp_VisibleToMembers(t *testing.T) {
	srv := newHelpServer(t, nil)
	if resp := do(t, srv, http.MethodGet, "/v1/docs", "", ""); resp.StatusCode != http.StatusOK {
		t.Errorf("member list docs → %d, want 200", resp.StatusCode)
	}
}

// The version endpoint carries readiness too, so the Settings health view has one typed
// call instead of an untyped fetch to /readyz.
func TestSystemVersion_ReportsVersionAndReadiness(t *testing.T) {
	srv := newHelpServer(t, func() (bool, string) { return false, "migrations pending" })
	resp := do(t, srv, http.MethodGet, "/v1/system/version", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("version → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Version string `json:"version"`
		Ready   bool   `json:"ready"`
		Detail  string `json:"detail"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	// An unstamped test build reports "dev" rather than an empty string — a bug report
	// that says "dev" is still more useful than one that says nothing.
	if body.Version == "" {
		t.Error("version is empty; a bug report needs something to quote")
	}
	if body.Ready {
		t.Error("ready = true although the readiness probe said otherwise")
	}
	if body.Detail != "migrations pending" {
		t.Errorf("detail = %q, want the probe's reason", body.Detail)
	}
}

// /healthz and /readyz stay OUTSIDE the authenticated API on purpose: their consumers are
// Docker HEALTHCHECK and orchestrators, which hold no session. Pinning this because
// "register them in huma so orval can type them" is a tempting change that would put an
// auth requirement in front of a container health probe.
func TestOpsProbesStayUnauthenticated(t *testing.T) {
	srv := newHelpServer(t, func() (bool, string) { return true, "ok" })
	for _, path := range []string{"/healthz", "/readyz"} {
		if resp := do(t, srv, http.MethodGet, path, "", ""); resp.StatusCode != http.StatusOK {
			t.Errorf("%s without credentials → %d, want 200", path, resp.StatusCode)
		}
	}
}
