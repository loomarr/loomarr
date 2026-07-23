package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/app"
	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/testkit"
	"github.com/mantonx/loomarr/internal/tmdb"
)

// harness drives the REAL composition root (app.BuildHandler) end to end, faking
// only the true external boundaries (media server, Seerr, TMDB, Tunarr, LLM,
// Ollama) via testkit doubles. Unlike pipeline_test.go's rig — which hand-wires a
// SUBSET of api.Options — this exercises the WHOLE surface the FE will call:
// onboarding (bootstrap/login/sessions), settings, users/roles, the model picker,
// and the suggestion→approve→channel pipeline. Its job is to prove the composition
// is wired with no seams, so the frontend meets a predictable backend (§21).
type harness struct {
	t          *testing.T
	srv        *httptest.Server
	store      store.Store
	ms         *testkit.MediaServer
	seerr      *testkit.Seerr
	tmdb       *testkit.TMDB
	tun        *testkit.Tunarr
	llm        *testkit.LLM
	ollama     *testkit.Ollama
	tunarrStub *httptest.Server // reachability stub for tunarr.url (pushes go to tun)
}

type harnessConfig struct {
	llm         *testkit.LLM
	seerr       bool
	connections bool // false ⇒ fresh install: seed NO connection settings
}

type harnessOpt func(*harnessConfig)

// withLLM injects a scripted LLM for the suggester (the pipeline journey).
func withLLM(l *testkit.LLM) harnessOpt { return func(c *harnessConfig) { c.llm = l } }

// withSeerr seeds seerr.url so the Acquisition feature is on.
func withSeerr() harnessOpt { return func(c *harnessConfig) { c.seerr = true } }

// withoutConnections builds a store-only "fresh install" (the FE's initial state):
// no library/tunarr/llm/... configured, so every feature route is unconfigured.
func withoutConnections() harnessOpt { return func(c *harnessConfig) { c.connections = false } }

func newHarness(t *testing.T, opts ...harnessOpt) *harness {
	t.Helper()
	cfg := harnessConfig{connections: true}
	for _, o := range opts {
		o(&cfg)
	}

	// Env pins win over db values (settings resolves env > db > default), so a dirty
	// dev shell (a sourced .env) would poison the seeded settings. Neutralize the
	// Loomarr keys for the duration of the test.
	clearLoomarrEnv(t)

	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/e2e.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	h := &harness{t: t, store: st}
	h.ms = testkit.NewMediaServer(t)
	h.tmdb = testkit.NewTMDB(t)
	h.tun = testkit.NewTunarr()
	h.ollama = testkit.NewOllama(t)
	// A reachable Tunarr: 404 everything (the GetChannel probe treats that as up),
	// EXCEPT /api/transcode_configs, which the `tunarr` setup check now resolves — a
	// real instance always has a Default (Phase-0 finding 3).
	h.tunarrStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/transcode_configs" {
			_, _ = w.Write([]byte(`[{"id":"default-uuid","name":"Default"}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(h.tunarrStub.Close)
	if h.llm = cfg.llm; h.llm == nil {
		h.llm = testkit.NewLLM(testkit.FinalResponse(`{"picks":[]}`))
	}
	if cfg.seerr {
		h.seerr = testkit.NewSeerr(t)
		t.Cleanup(h.seerr.Close)
	}

	if cfg.connections {
		h.seedConnections()
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	handler, err := app.BuildHandler(ctx, st, testkit.Logger(), app.Overrides{
		Programmer: h.tun,
		LLM:        h.llm,
		TMDB:       tmdb.NewWithBase(h.tmdb.URL, "test-key"),
	})
	if err != nil {
		t.Fatal(err)
	}
	h.srv = httptest.NewServer(handler)
	t.Cleanup(h.srv.Close)
	return h
}

// seedConnections writes the settings rows that point the real adapters at the
// testkit doubles — the same table bootSettings reads (§3). Large runner intervals
// keep periodic ticks from racing the assertions; the suggester worker pool is a
// queue consumer, not a ticker, so it stays live.
func (h *harness) seedConnections() {
	set := func(k, v string) {
		if err := h.store.SetSetting(context.Background(), k, v); err != nil {
			h.t.Fatal(err)
		}
	}
	set("library.flavor", "emby")
	set("library.url", h.ms.URL)
	set("library.token", h.ms.AdminToken)
	set("tmdb.api_key", "test-key")
	set("llm.provider", "ollama")
	set("llm.url", h.ollama.URL)
	set("llm.model", "qwen3:8b")
	set("tunarr.url", h.tunarrStub.URL) // gate only; real pushes hit the injected tun
	set("filler.dir", h.t.TempDir())
	set("reconcile.every", "9999h")
	set("channel.reconcile_every", "9999h")
	set("filler.sync_every", "9999h")
	if h.seerr != nil {
		set("seerr.url", h.seerr.URL)
		set("seerr.api_key", "seerr-key")
	}
}

// clearLoomarrEnv unsets every Loomarr env var for the test (restoring after), so a
// sourced .env in the dev shell can't override the seeded db settings.
func clearLoomarrEnv(t *testing.T) {
	keys := []string{
		"LIBRARY_FLAVOR", "LIBRARY_URL", "LIBRARY_TOKEN", "SEASON_PRECISION",
		"SEERR_URL", "SEERR_API_KEY", "SONARR_URL", "SONARR_API_KEY", "RADARR_URL", "RADARR_API_KEY",
		"TUNARR_URL", "TUNARR_TRANSCODE_CONFIG_ID",
		"LLM_PROVIDER", "LLM_URL", "LLM_MODEL", "LLM_API_KEY", "TMDB_API_KEY",
		"FILLER_DIR", "FILLER_AI_TAGGING", "SESSION_SECRET", "API_TOKEN",
		"RECONCILE_EVERY", "CHANNEL_RECONCILE_EVERY", "FILLER_SYNC_EVERY",
		"JOB_WORKERS", "SUGGEST_MAX_ACQUISITIONS", "SUGGEST_AUTO_APPROVE",
	}
	for _, k := range keys {
		if old, ok := os.LookupEnv(k); ok {
			_ = os.Unsetenv(k)
			t.Cleanup(func() { _ = os.Setenv(k, old) })
		}
	}
}

// ---- HTTP helpers -------------------------------------------------------------

// do issues a request against the harness server. A non-nil cookie authenticates
// as that session; mutations carry the CSRF header (login is exempt, §11).
func (h *harness) do(method, path, body string, cookie *http.Cookie) *http.Response {
	h.t.Helper()
	req, _ := http.NewRequest(method, h.srv.URL+path, strings.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && path != "/v1/auth/login" {
		req.Header.Set("X-Loomarr-Csrf", "1")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	return resp
}

// getJSON does a GET (optionally authed) and decodes a 200 body into v.
func (h *harness) getJSON(path string, cookie *http.Cookie, v any) {
	h.t.Helper()
	resp := h.do(http.MethodGet, path, "", cookie)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		h.t.Fatalf("GET %s → %d, want 200", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		h.t.Fatalf("decode %s: %v", path, err)
	}
}

// status issues a request and returns just the status code (body closed).
func (h *harness) status(method, path, body string, cookie *http.Cookie) int {
	resp := h.do(method, path, body, cookie)
	_ = resp.Body.Close()
	return resp.StatusCode
}

// ---- onboarding helpers -------------------------------------------------------

// bootstrap creates the owning admin (real Provisioner + bcrypt) and returns the
// response status so callers can assert 200 (first) vs 409 (subsequent).
func (h *harness) bootstrap(user, pass string) int {
	h.t.Helper()
	return h.status(http.MethodPost, "/v1/setup/bootstrap",
		`{"username":"`+user+`","password":"`+pass+`"}`, nil)
}

// login performs a local/imported login and returns the session cookie.
func (h *harness) login(user, pass string) *http.Cookie {
	h.t.Helper()
	resp := h.do(http.MethodPost, "/v1/auth/login",
		`{"username":"`+user+`","password":"`+pass+`"}`, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		h.t.Fatalf("login %s → %d, want 200", user, resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.Value != "" {
			return c
		}
	}
	h.t.Fatalf("login %s set no session cookie", user)
	return nil
}

// asAdmin bootstraps the owning admin (local bcrypt) once and logs in — the
// untested bootstrap→local-login chain. Returns the admin session cookie.
func (h *harness) asAdmin() *http.Cookie {
	h.t.Helper()
	if code := h.bootstrap("owner", "owner-pass"); code != http.StatusOK {
		h.t.Fatalf("bootstrap owner → %d, want 200", code)
	}
	return h.login("owner", "owner-pass")
}

// asMember imports a media-server user (real admin-gated import) and logs in as
// them via the media server — the untested import→member-login chain. name/id must
// match a fixture user (only its id is allowlisted; the login verifies creds).
func (h *harness) asMember(admin *http.Cookie, name, id string) *http.Cookie {
	h.t.Helper()
	// The member must be an authenticatable media-server account whose id is imported.
	if h.ms.Accounts == nil {
		h.ms.Accounts = map[string]testkit.Account{}
	}
	h.ms.Accounts[name] = testkit.Account{Password: "member-pass", ID: id, IsAdmin: false}
	if code := h.status(http.MethodPost, "/v1/users/import",
		`{"ids":["`+id+`"]}`, admin); code != http.StatusOK {
		h.t.Fatalf("import member → %d, want 200", code)
	}
	return h.login(name, "member-pass")
}

// ---- settings helpers ---------------------------------------------------------

// features reads GET /v1/settings and returns its computed feature map (admin).
func (h *harness) features(admin *http.Cookie) map[string]bool {
	h.t.Helper()
	var out struct {
		Features map[string]bool `json:"features"`
	}
	h.getJSON("/v1/settings", admin, &out)
	return out.Features
}

// setupCheckOK returns whether the named /v1/setup/status check reports OK.
func (h *harness) setupCheckOK(admin *http.Cookie, name string) bool {
	h.t.Helper()
	var out struct {
		Checks []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"checks"`
	}
	h.getJSON("/v1/setup/status", admin, &out)
	for _, c := range out.Checks {
		if c.Name == name {
			return c.OK
		}
	}
	return false
}

// patchSettings applies a settings edit as admin and returns the status.
func (h *harness) patchSettings(admin *http.Cookie, edits map[string]string) int {
	h.t.Helper()
	body, _ := json.Marshal(map[string]any{"edits": edits})
	return h.status(http.MethodPatch, "/v1/settings", string(body), admin)
}

// ---- pipeline helpers ---------------------------------------------------------

// awaitProposal polls the submitted queue until the job's proposal appears (the
// worker ran) or the deadline passes; a worker failure surfaces the job's error.
func (h *harness) awaitProposal(cookie *http.Cookie, jobID string) (string, suggest.Proposal) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var list struct {
			Proposals []struct {
				ID       string          `json:"id"`
				JobID    string          `json:"jobId"`
				Proposal json.RawMessage `json:"proposal"`
			} `json:"proposals"`
		}
		h.getJSON("/v1/suggestions?status=submitted", cookie, &list)
		for _, p := range list.Proposals {
			if p.JobID == jobID {
				var prop suggest.Proposal
				if err := json.Unmarshal(p.Proposal, &prop); err != nil {
					h.t.Fatalf("decode proposal: %v", err)
				}
				return p.ID, prop
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if job, err := h.store.GetJob(context.Background(), jobID); err == nil {
		h.t.Fatalf("proposal never appeared for job %s (status=%s, err=%q)", jobID, job.Status, job.LastError)
	}
	h.t.Fatalf("proposal never appeared for job %s", jobID)
	return "", suggest.Proposal{}
}
