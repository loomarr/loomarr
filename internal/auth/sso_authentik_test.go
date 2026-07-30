//go:build integration

// A REAL Authentik, driven through the whole OIDC flow (§11, V8).
//
// ⚠ **A second provider, because the first one's blind spots were invisible from inside it.**
// Authelia found that profile claims live at userinfo (the stub had inlined them). Authentik
// then found that its issuer is path-based WITH a trailing slash, which our normalisation
// stripped — a hard discovery failure Authelia could never have shown, since its issuer is a
// host root. Two providers, two bugs, neither visible to the other.
//
// They also disagree about something we might have taken as a rule: Authelia REFUSES plain
// HTTP (it derives its issuer from the request), while Authentik serves discovery over it
// happily. "A provider requires HTTPS" is per-provider, so Loomarr assumes neither.
//
// Heavier than the Authelia harness by design: Authentik needs Postgres, Redis, a server AND
// a worker, and its OIDC provider is configured through its API rather than a config file.
// The worker is not optional — without it the bootstrap token is never processed and every
// API call 403s with "Token invalid/expired", which reads as a credential problem rather than
// a missing service.
package auth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/store"
)

const (
	authentikImage = "ghcr.io/goauthentik/server:2025.10"
	akSecretKey    = "test-secret-key-at-least-50-chars-long-for-authentik-testing"
	akBootstrapTok = "test-bootstrap-token-for-api-access"
	akAdminPass    = "akadmin-test-password"
	akHostPort     = 19000
)

// TestSSO_AgainstRealAuthentik asserts the same two outcomes the Authelia suite does, against
// a different implementation: an allowlisted person gets in, and a verified identity with no
// row does not.
func TestSSO_AgainstRealAuthentik(t *testing.T) {
	ak := startAuthentik(t)
	st := newSSOStore(t)
	ctx := context.Background()

	// Only `sam` is allowlisted. Authentik will authenticate `stranger` perfectly well.
	if err := st.UpsertUser(ctx, store.User{
		ID: "u-sam", Name: ssoUser, Role: "member", UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	svc := auth.NewSSOService(ak.ssoConfig(), st, auth.NewManager(st, time.Hour, nil), time.Now, slog.New(slog.DiscardHandler))

	t.Run("an allowlisted person signs in", func(t *testing.T) {
		authURL, state, err := svc.Start(ctx, "/guide")
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		code := ak.signInAndGetCode(t, authURL, ssoUser)

		token, _, u, claims, returnTo, err := svc.Exchange(ctx, state, code)
		if err != nil {
			t.Fatalf("Exchange against real Authentik: %v", err)
		}
		if token == "" || u.ID != "u-sam" {
			t.Errorf("no session for the allowlisted row (user=%+v)", u)
		}
		// ⚠ Role from OUR row, never the provider. Authentik knows nothing of Loomarr roles.
		if u.Role != "member" {
			t.Errorf("role = %q, want member from the allowlist row", u.Role)
		}
		// The claim Loomarr matches on, as Authentik spells it — from userinfo, since its
		// `claims_supported` advertises only protocol claims in the id_token.
		if claims.PreferredUsername != ssoUser {
			t.Errorf("preferred_username = %q, want %q", claims.PreferredUsername, ssoUser)
		}
		if returnTo != "/guide" {
			t.Errorf("returnTo = %q, want the deep link", returnTo)
		}
	})

	t.Run("a verified identity with no allowlist row is refused", func(t *testing.T) {
		authURL, state, err := svc.Start(ctx, "")
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		code := ak.signInAndGetCode(t, authURL, "stranger")

		token, _, _, claims, _, err := svc.Exchange(ctx, state, code)
		if err != auth.ErrInvalidCredentials {
			t.Fatalf("Exchange for an un-allowlisted identity = %v, want ErrInvalidCredentials", err)
		}
		if token != "" {
			t.Fatal("a session was issued to someone with no allowlist row")
		}
		if claims.PreferredUsername != "stranger" {
			t.Errorf("claims = %+v, want the provider's claims on a refusal", claims)
		}
		if users, err := st.ListUsers(ctx); err == nil && len(users) != 1 {
			t.Errorf("user count = %d, want 1 — SSO must never provision", len(users))
		}
	})
}

// --- the stack ---

type authentikIDP struct{ baseURL string }

func (a authentikIDP) ssoConfig() func() auth.SSOConfig {
	return func() auth.SSOConfig {
		return auth.SSOConfig{
			Enabled: true,
			// ⚠ WITH the trailing slash, exactly as Authentik advertises it. Trimming it is
			// what broke discovery before this suite existed.
			Issuer:       a.baseURL + "/application/o/loomarr/",
			ClientID:     ssoClientID,
			ClientSecret: ssoClientSecret,
			RedirectURL:  "http://loomarr.test/v1/auth/sso/callback",
		}
	}
}

func startAuthentik(t *testing.T) authentikIDP {
	t.Helper()
	suffix := time.Now().UnixNano()
	net := fmt.Sprintf("loomarr-ak-net-%d", suffix)
	names := struct{ pg, redis, server, worker string }{
		fmt.Sprintf("loomarr-ak-pg-%d", suffix),
		fmt.Sprintf("loomarr-ak-redis-%d", suffix),
		fmt.Sprintf("loomarr-ak-server-%d", suffix),
		fmt.Sprintf("loomarr-ak-worker-%d", suffix),
	}

	run := func(args ...string) {
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("network", "create", net)
	t.Cleanup(func() {
		for _, n := range []string{names.worker, names.server, names.redis, names.pg} {
			if t.Failed() {
				if logs, err := exec.Command("docker", "logs", "--tail", "40", n).CombinedOutput(); err == nil {
					t.Logf("%s logs:\n%s", n, logs)
				}
			}
			_ = exec.Command("docker", "rm", "-f", n).Run()
		}
		_ = exec.Command("docker", "network", "rm", net).Run()
	})

	run("run", "-d", "--name", names.pg, "--network", net,
		"-e", "POSTGRES_PASSWORD=akpass", "-e", "POSTGRES_USER=authentik",
		"-e", "POSTGRES_DB=authentik", "postgres:16-alpine")
	run("run", "-d", "--name", names.redis, "--network", net, "redis:alpine")

	akEnv := []string{
		"-e", "AUTHENTIK_SECRET_KEY=" + akSecretKey,
		"-e", "AUTHENTIK_POSTGRESQL__HOST=" + names.pg,
		"-e", "AUTHENTIK_POSTGRESQL__USER=authentik",
		"-e", "AUTHENTIK_POSTGRESQL__NAME=authentik",
		"-e", "AUTHENTIK_POSTGRESQL__PASSWORD=akpass",
		"-e", "AUTHENTIK_REDIS__HOST=" + names.redis,
		"-e", "AUTHENTIK_BOOTSTRAP_PASSWORD=" + akAdminPass,
		"-e", "AUTHENTIK_BOOTSTRAP_TOKEN=" + akBootstrapTok,
	}
	run(append(append([]string{"run", "-d", "--name", names.server, "--network", net,
		"-p", fmt.Sprintf("127.0.0.1:%d:9000", akHostPort)}, akEnv...), authentikImage, "server")...)
	// ⚠ The worker is REQUIRED, not optional: it processes the bootstrap token, and without it
	// every API call 403s with "Token invalid/expired" — which reads as a credential problem
	// rather than a missing service. That cost a debugging detour worth recording.
	run(append(append([]string{"run", "-d", "--name", names.worker, "--network", net}, akEnv...),
		authentikImage, "worker")...)

	base := fmt.Sprintf("http://127.0.0.1:%d", akHostPort)
	waitForAuthentik(t, base)
	provisionAuthentik(t, base)
	return authentikIDP{baseURL: base}
}

func waitForAuthentik(t *testing.T, base string) {
	t.Helper()
	// Authentik runs migrations on first boot, so this is minutes rather than seconds.
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(base + "/-/health/ready/"); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				// Health is the server; the API needs the worker to have consumed the
				// bootstrap token. Poll the API itself rather than assuming.
				if akAPIReady(base) {
					return
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatal("authentik never became ready")
}

func akAPIReady(base string) bool {
	req, _ := http.NewRequest(http.MethodGet, base+"/api/v3/flows/instances/?designation=authorization", nil)
	req.Header.Set("Authorization", "Bearer "+akBootstrapTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// provisionAuthentik creates the OAuth2 provider, its application, and two users — one
// Loomarr allowlists and one it does not.
//
// Through the API rather than a blueprint: the API is what an operator's own setup goes
// through, and a blueprint would be a second description of the same thing to keep correct.
func provisionAuthentik(t *testing.T, base string) {
	t.Helper()

	// An implicit-consent flow, so the test does not have to click through a consent screen
	// that is Authentik's UI rather than the OIDC contract.
	flow := akPick(t, base, "/api/v3/flows/instances/?designation=authorization", func(r map[string]any) bool {
		slug, _ := r["slug"].(string)
		return strings.Contains(slug, "implicit")
	})
	scopes := akScopeMappings(t, base)

	akPost(t, base, "/api/v3/providers/oauth2/", map[string]any{
		"name": "loomarr", "authorization_flow": flow, "invalidation_flow": flow,
		"client_type": "confidential", "client_id": ssoClientID, "client_secret": ssoClientSecret,
		"redirect_uris": []map[string]string{
			{"matching_mode": "strict", "url": "http://loomarr.test/v1/auth/sso/callback"},
		},
		"property_mappings": scopes,
		// The subject is the username, so `sub` is legible in a log — but Loomarr still
		// matches on preferred_username, which is what the claim-precedence rule specifies.
		"sub_mode": "user_username",
	})
	akPost(t, base, "/api/v3/core/applications/", map[string]any{
		"name": "Loomarr", "slug": "loomarr", "provider": 1,
	})

	for _, u := range []string{ssoUser, "stranger"} {
		created := akPost(t, base, "/api/v3/core/users/", map[string]any{
			"username": u, "name": u + " Example", "email": u + "@loomarr.test",
			"is_active": true, "path": "users",
		})
		pk, ok := created["pk"]
		if !ok {
			t.Fatalf("no pk for user %s: %v", u, created)
		}
		akPost(t, base, fmt.Sprintf("/api/v3/core/users/%v/set_password/", pk),
			map[string]any{"password": ssoPassword})
	}
}

func akPick(t *testing.T, base, path string, match func(map[string]any) bool) string {
	t.Helper()
	var body struct {
		Results []map[string]any `json:"results"`
	}
	akGet(t, base, path, &body)
	if len(body.Results) == 0 {
		t.Fatalf("no results from %s", path)
	}
	for _, r := range body.Results {
		if match(r) {
			return fmt.Sprint(r["pk"])
		}
	}
	return fmt.Sprint(body.Results[0]["pk"]) // any authorization flow beats none
}

func akScopeMappings(t *testing.T, base string) []string {
	t.Helper()
	var body struct {
		Results []struct {
			PK        string `json:"pk"`
			ScopeName string `json:"scope_name"`
		} `json:"results"`
	}
	akGet(t, base, "/api/v3/propertymappings/provider/scope/", &body)
	want := map[string]bool{"openid": true, "profile": true, "email": true}
	var out []string
	for _, r := range body.Results {
		if want[r.ScopeName] {
			out = append(out, r.PK)
		}
	}
	if len(out) != 3 {
		t.Fatalf("found %d of the 3 required scope mappings", len(out))
	}
	return out
}

func akGet(t *testing.T, base, path string, into any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, base+path, nil)
	req.Header.Set("Authorization", "Bearer "+akBootstrapTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s → %d: %s", path, resp.StatusCode, payload)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func akPost(t *testing.T, base, path string, body map[string]any) map[string]any {
	t.Helper()
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, base+path, strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer "+akBootstrapTok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("POST %s → %d: %s", path, resp.StatusCode, raw)
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out) // 204s have no body, which is fine
	return out
}

// signInAndGetCode plays the browser through Authentik's flow executor and returns the `code`.
//
// Authentik drives login as a sequence of flow "challenges" over its API rather than a form
// post, so this walks that sequence instead of posting credentials once.
func (a authentikIDP) signInAndGetCode(t *testing.T, authURL, username string) string {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(r *http.Request, _ []*http.Request) error {
			if strings.HasPrefix(r.URL.String(), "http://loomarr.test/") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	// 1. Hitting authorize starts a flow and lands on the executor.
	resp, err := client.Get(authURL)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	_ = resp.Body.Close()

	// 2. Walk the challenge sequence: identification, then password.
	executorURL := a.baseURL + "/api/v3/flows/executor/default-authentication-flow/?query=" + url.QueryEscape(strings.TrimPrefix(authURL, a.baseURL))
	post := func(body map[string]any) map[string]any {
		payload, _ := json.Marshal(body)
		r, err := client.Post(executorURL, "application/json", strings.NewReader(string(payload)))
		if err != nil {
			t.Fatalf("flow executor: %v", err)
		}
		defer func() { _ = r.Body.Close() }()
		raw, _ := io.ReadAll(r.Body)
		out := map[string]any{}
		_ = json.Unmarshal(raw, &out)
		return out
	}
	post(map[string]any{"component": "ak-stage-identification", "uid_field": username})
	final := post(map[string]any{"component": "ak-stage-password", "password": ssoPassword})

	// 3. The flow ends by telling the browser where to go — carrying the code.
	to, _ := final["to"].(string)
	if to == "" {
		t.Fatalf("flow did not finish with a redirect: %v", final)
	}
	u, err := url.Parse(to)
	if err != nil {
		t.Fatalf("parse %q: %v", to, err)
	}
	code := u.Query().Get("code")
	if code == "" {
		// Authentik may hand back a relative continuation; follow it once.
		if !strings.HasPrefix(to, "http") {
			to = a.baseURL + to
		}
		r, ferr := client.Get(to)
		if ferr != nil {
			t.Fatalf("follow %q: %v", to, ferr)
		}
		_ = r.Body.Close()
		loc, lerr := url.Parse(r.Header.Get("Location"))
		if lerr == nil {
			code = loc.Query().Get("code")
		}
	}
	if code == "" {
		t.Fatalf("no code in %q", to)
	}
	return code
}
