package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

// readBody drains a response so two of them can be compared verbatim.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newHelpServer(t *testing.T, ready api.ReadyFunc) *httptest.Server {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/h.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st,
		Auth:  testAuthorizer{},
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
	if resp := do(t, srv, http.MethodGet, "/v1/docs", memberToken, ""); resp.StatusCode != http.StatusOK {
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

// The About page's remaining rows (§16, V12). Every one is server-truth: a runtime or
// schema version the frontend guessed would describe the frontend, which is exactly the
// wrong answer when the two are out of step.
func TestSystemVersion_ReportsRuntimeAndSchema(t *testing.T) {
	srv := newHelpServer(t, nil)
	resp := do(t, srv, http.MethodGet, "/v1/system/version", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("version → %d, want 200", resp.StatusCode)
	}
	var body struct {
		GoVersion     string `json:"goVersion"`
		Platform      string `json:"platform"`
		StartedAt     string `json:"startedAt"`
		SchemaVersion int64  `json:"schemaVersion"`
		Backend       string `json:"backend"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(body.GoVersion, "go1.") {
		t.Errorf("goVersion = %q, want the Go runtime version", body.GoVersion)
	}
	if body.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		t.Errorf("platform = %q, want %s/%s", body.Platform, runtime.GOOS, runtime.GOARCH)
	}
	if body.Backend != "sqlite" {
		t.Errorf("backend = %q, want sqlite", body.Backend)
	}

	// ⚠ A REAL applied version, not a placeholder. The helper opens a freshly migrated
	// store, so the number must match what goose actually applied — a hardcoded 0 or a
	// count of embedded files would both pass a weaker assertion and be wrong.
	if body.SchemaVersion <= 0 {
		t.Errorf("schemaVersion = %d, want the applied migration version", body.SchemaVersion)
	}

	// ⚠ startedAt is an INSTANT, not a duration. A pre-computed uptime is stale the moment
	// it is serialized; the client counts from this.
	started, err := time.Parse(time.RFC3339, body.StartedAt)
	if err != nil {
		t.Fatalf("startedAt = %q, want RFC3339: %v", body.StartedAt, err)
	}
	if started.After(time.Now().Add(time.Second)) {
		t.Errorf("startedAt %v is in the future", started)
	}
	if time.Since(started) > time.Hour {
		t.Errorf("startedAt %v is implausibly old for a test process", started)
	}
}

// ⚠ REGRESSION: `startedAt` must be per-handler, not per-process.
//
// It began as a package-level `var processStart = time.Now()`, and the §9.2 restart loop
// proved that wrong on its first live test: the value survived the rebuild, so About kept
// reporting the ORIGINAL boot and would have claimed days of uptime on an instance
// restarted seconds ago. No panic, no log line — just a number quietly lying in the one
// place an operator looks when writing a bug report.
//
// Two handlers built moments apart must report different instants, which a package-level
// var cannot do.
func TestSystemVersion_StartedAtIsPerGeneration(t *testing.T) {
	read := func() string {
		srv := newHelpServer(t, nil)
		resp := do(t, srv, http.MethodGet, "/v1/system/version", adminToken, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("version → %d, want 200", resp.StatusCode)
		}
		var body struct {
			StartedAt string `json:"startedAt"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body.StartedAt
	}

	first := read()
	// RFC3339 is second-resolution, so two builds in the same second would legitimately
	// match. Wait past the boundary rather than asserting on a value that cannot differ.
	time.Sleep(1100 * time.Millisecond)
	second := read()

	if first == second {
		t.Errorf("both generations report startedAt=%s — a restart would keep counting from the original boot", first)
	}
}

// A store-less boot still serves this endpoint — it is what an operator reads to find out
// WHY the install is unhealthy. The two store-derived rows are absent rather than the whole
// call failing.
func TestSystemVersion_WithoutStoreOmitsSchemaRows(t *testing.T) {
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Auth: testAuthorizer{},
		Log:  slog.New(slog.DiscardHandler),
	}))
	t.Cleanup(srv.Close)

	resp := do(t, srv, http.MethodGet, "/v1/system/version", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("version with no store → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Version       string `json:"version"`
		GoVersion     string `json:"goVersion"`
		SchemaVersion int64  `json:"schemaVersion"`
		Backend       string `json:"backend"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Version == "" || body.GoVersion == "" {
		t.Error("the store-independent rows must still be reported")
	}
	if body.SchemaVersion != 0 || body.Backend != "" {
		t.Errorf("schemaVersion/backend = %d/%q with no store, want both absent", body.SchemaVersion, body.Backend)
	}
}

// The ops probes answer with NO credential. Their consumers are Docker HEALTHCHECK and
// orchestrators, which hold no session and must never need one.
//
// ⚠ **This test used to pin something stronger — that the probes stay OUTSIDE huma — and that
// pin has been deliberately released.** Its reasoning was that registering them "would put an
// auth requirement in front of a container health probe", which was true when a Huma operation
// implied a session. `RolePublic` makes non-authentication an explicit, greppable property of
// the operation, so the probes are Huma ops now and still answer anonymously. The objection was
// ANSWERED, not overruled: what it actually cared about is the assertion below, which is why
// that assertion survives while the framework claim did not.
//
// Both paths are checked. `/v1/...` is canonical; the bare path is a permanent alias, because a
// healthcheck configured in someone's compose file cannot be migrated by editing this repo.
func TestOpsProbesStayUnauthenticated(t *testing.T) {
	srv := newHelpServer(t, func() (bool, string) { return true, "ok" })
	for _, path := range []string{
		"/v1/healthz", "/v1/readyz",
		"/healthz", "/readyz",
	} {
		if resp := do(t, srv, http.MethodGet, path, "", ""); resp.StatusCode != http.StatusOK {
			t.Errorf("%s without credentials → %d, want 200", path, resp.StatusCode)
		}
	}
}

// The alias must be the SAME endpoint, not merely another 200 — a probe that reports readiness
// on one path and something else on the other is worse than having one path.
func TestOpsProbeAliasesAgreeWithTheCanonicalPaths(t *testing.T) {
	srv := newHelpServer(t, func() (bool, string) { return false, "no store configured" })

	for _, pair := range [][2]string{
		{"/v1/healthz", "/healthz"},
		{"/v1/readyz", "/readyz"},
	} {
		canonical := do(t, srv, http.MethodGet, pair[0], "", "")
		alias := do(t, srv, http.MethodGet, pair[1], "", "")
		if canonical.StatusCode != alias.StatusCode {
			t.Errorf("%s → %d but %s → %d; the alias must be the same endpoint",
				pair[0], canonical.StatusCode, pair[1], alias.StatusCode)
		}
		cBody, aBody := readBody(t, canonical), readBody(t, alias)
		if cBody != aBody {
			t.Errorf("%s body %q != %s body %q", pair[0], cBody, pair[1], aBody)
		}
	}

	// ⚠ And the un-ready answer keeps its SHAPE. Returning a huma error would have been the
	// idiomatic way to get a 503 and would have replaced {ready, detail} with an RFC 7807
	// problem — a wire change for every orchestrator parsing this, to gain nothing.
	resp := do(t, srv, http.MethodGet, "/v1/readyz", "", "")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unready /v1/readyz → %d, want 503", resp.StatusCode)
	}
	var body struct {
		Ready  bool   `json:"ready"`
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Ready || body.Detail != "no store configured" {
		t.Errorf("unready body = %+v, want ready=false and the reason — a probe that cannot "+
			"explain itself is the thing §17 added it for", body)
	}
}
