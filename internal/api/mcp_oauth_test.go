package api_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// newOAuthHarness is the device harness with its own address as the public one, so the sign-in's
// issuer and endpoints are reachable by the test's client.
func newOAuthHarness(t *testing.T) *apiHarness {
	t.Helper()
	var base atomic.Value
	h := newDeviceHarnessAt(t, nil, func() string { s, _ := base.Load().(string); return s })
	base.Store(h.Server.URL)
	return h
}

// noFollow is a browser that shows us each redirect instead of following it.
func noFollow(srv *httptest.Server, cookie *http.Cookie) *http.Client {
	return &http.Client{
		Transport: cookieTransport{cookie: cookie, base: srv.Client().Transport},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type cookieTransport struct {
	cookie *http.Cookie
	base   http.RoundTripper
}

func (c cookieTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if c.cookie != nil {
		r = r.Clone(r.Context())
		r.AddCookie(c.cookie)
	}
	return c.base.RoundTrip(r)
}

func getJSON(t *testing.T, client *http.Client, raw string, out any) int {
	t.Helper()
	resp, err := client.Get(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func pkce() (verifier, challenge string) {
	verifier = "a-verifier-long-enough-for-rfc7636-section-4-1-padding-xyz"
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

type oauthStart struct {
	clientID, redirect, verifier, userCode, then string
}

// startSignIn discovers, registers and authorizes, stopping where the browser reaches /pair.
func startSignIn(t *testing.T, srv *httptest.Server) oauthStart {
	t.Helper()
	anon := noFollow(srv, nil)

	resp, err := anon.Post(srv.URL+"/v1/mcp", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":"probe","method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	challenge := resp.Header.Get("WWW-Authenticate")
	wantMeta := srv.URL + "/.well-known/oauth-protected-resource/v1/mcp"
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(challenge, `resource_metadata="`+wantMeta+`"`) {
		t.Fatalf("no token: %d %q, want 401 naming %s", resp.StatusCode, challenge, wantMeta)
	}
	var resource struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if code := getJSON(t, anon, wantMeta, &resource); code != http.StatusOK || resource.Resource != srv.URL+"/v1/mcp" || !slices.Equal(resource.AuthorizationServers, []string{srv.URL}) {
		t.Fatalf("resource metadata = %d %+v", code, resource)
	}
	var server struct {
		Authorize string   `json:"authorization_endpoint"`
		Token     string   `json:"token_endpoint"`
		Register  string   `json:"registration_endpoint"`
		Methods   []string `json:"code_challenge_methods_supported"`
	}
	if code := getJSON(t, anon, srv.URL+"/.well-known/oauth-authorization-server", &server); code != http.StatusOK || !slices.Equal(server.Methods, []string{"S256"}) {
		t.Fatalf("server metadata = %d %+v", code, server)
	}

	resp, err = anon.Post(server.Register, "application/json", strings.NewReader(`{"client_name":"Test agent","redirect_uris":["http://127.0.0.1:9999/callback"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var reg struct {
		ClientID string `json:"client_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&reg)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || reg.ClientID == "" {
		t.Fatalf("register = %d %+v", resp.StatusCode, reg)
	}

	verifier, challengeS256 := pkce()
	// A loopback redirect may name any port: a local app listens on whichever was free.
	redirect := "http://127.0.0.1:41234/callback"
	q := url.Values{
		"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {redirect},
		"code_challenge": {challengeS256}, "code_challenge_method": {"S256"}, "state": {"st-1"},
		"resource": {srv.URL + "/v1/mcp"},
	}
	resp, err = anon.Get(server.Authorize + "?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusFound || loc == nil || loc.Path != "/pair" || loc.Query().Get("code") == "" {
		t.Fatalf("authorize = %d to %q, want a redirect to /pair with a code", resp.StatusCode, resp.Header.Get("Location"))
	}
	return oauthStart{clientID: reg.ClientID, redirect: redirect, verifier: verifier, userCode: loc.Query().Get("code"), then: loc.Query().Get("next")}
}

func continueSignIn(t *testing.T, srv *httptest.Server, cookie *http.Cookie, then string) *http.Response {
	t.Helper()
	resp, err := noFollow(srv, cookie).Get(srv.URL + then)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp
}

func exchange(t *testing.T, srv *httptest.Server, form url.Values) (int, map[string]string) {
	t.Helper()
	resp, err := srv.Client().PostForm(srv.URL+"/v1/oauth/token", form)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// The whole sign-in an MCP client runs, end to end: the 401 challenge, discovery, registration,
// authorization through the /pair approval, the code exchange with PKCE, and the token working on
// /v1/mcp as the person who approved, with that person's tools.
func TestMCPOAuthSignInEndsInTheApproversDeviceToken(t *testing.T) {
	t.Parallel()
	h := newOAuthHarness(t)
	s := startSignIn(t, h.Server)

	kid := login(t, h.Server, "kid", "pw")
	if code, body := postJSON(t, h.Server, "/v1/auth/device/approve", map[string]string{"userCode": s.userCode}, kid); code != http.StatusOK {
		t.Fatalf("approve = %d (%v)", code, body)
	}
	resp := continueSignIn(t, h.Server, kid, s.then)
	back, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusFound || back == nil || !strings.HasPrefix(back.String(), s.redirect) ||
		back.Query().Get("state") != "st-1" || back.Query().Get("iss") != h.Server.URL {
		t.Fatalf("continue = %d to %q, want the client's redirect with code, state and iss", resp.StatusCode, resp.Header.Get("Location"))
	}
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {back.Query().Get("code")}, "redirect_uri": {s.redirect},
		"client_id": {s.clientID}, "code_verifier": {s.verifier},
	}
	code, tok := exchange(t, h.Server, form)
	if code != http.StatusOK || tok["access_token"] == "" || tok["token_type"] != "Bearer" {
		t.Fatalf("token = %d %v", code, tok)
	}

	tools := toolNames(t, mustConnectMCP(t, h.Server, tok["access_token"]))
	if !slices.Contains(tools, "whats_on") || slices.Contains(tools, "needs_attention") {
		t.Fatalf("the signed-in agent's tools are %v, want the member's", tools)
	}
	if code, again := exchange(t, h.Server, form); code != http.StatusBadRequest || again["error"] != "invalid_grant" {
		t.Fatalf("replaying the code = %d %v, want invalid_grant", code, again)
	}
	// The grant is an ordinary paired device, named for the client and where it sends grants.
	resp2 := authed(t, http.MethodGet, h.Server.URL+"/v1/auth/devices", kid, "")
	defer func() { _ = resp2.Body.Close() }()
	var devices struct {
		Devices []struct {
			Name string `json:"deviceName"`
		} `json:"devices"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&devices)
	if len(devices.Devices) != 1 || devices.Devices[0].Name != "Test agent (127.0.0.1)" {
		t.Fatalf("devices = %+v, want one named \"Test agent (127.0.0.1)\"", devices.Devices)
	}
}

// A wrong verifier gets nothing, and it burns the code: a stolen code cannot be retried until a
// verifier is guessed.
// Consent needs to know who is asking BEFORE the click: /pair reads the app's name and the host
// the grant goes to, signed in, and only while the sign-in is waiting.
func TestMCPOAuthPairShowsWhoIsAskingBeforeApproval(t *testing.T) {
	t.Parallel()
	h := newOAuthHarness(t)
	s := startSignIn(t, h.Server)
	next, _ := url.Parse(s.then)
	describe := h.Server.URL + "/v1/oauth/request?" + next.RawQuery

	kid := login(t, h.Server, "kid", "pw")
	var asking struct {
		App      string `json:"app"`
		SendsTo  string `json:"sendsTo"`
		Loopback bool   `json:"loopback"`
	}
	if code := getJSON(t, noFollow(h.Server, kid), describe, &asking); code != http.StatusOK ||
		asking.App != "Test agent" || asking.SendsTo != "127.0.0.1" || !asking.Loopback {
		t.Fatalf("describe = %d %+v, want the app's name and where it sends the grant", code, asking)
	}
	if code := getJSON(t, noFollow(h.Server, nil), describe, &asking); code != http.StatusUnauthorized {
		t.Fatalf("describe signed out = %d, want 401", code)
	}
	if code := getJSON(t, noFollow(h.Server, kid), h.Server.URL+"/v1/oauth/request?request=nope", &asking); code != http.StatusNotFound {
		t.Fatalf("describe an unknown request = %d, want 404", code)
	}

	if code, body := postJSON(t, h.Server, "/v1/auth/device/approve", map[string]string{"userCode": s.userCode}, kid); code != http.StatusOK {
		t.Fatalf("approve = %d (%v)", code, body)
	}
	continueSignIn(t, h.Server, kid, s.then)
	if code := getJSON(t, noFollow(h.Server, kid), describe, &asking); code != http.StatusNotFound {
		t.Fatalf("describe a finished sign-in = %d, want 404", code)
	}
	if resp := continueSignIn(t, h.Server, kid, s.then); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("continuing a finished sign-in again = %d, want 404", resp.StatusCode)
	}
}

func TestMCPOAuthRefusesAWrongVerifierAndBurnsTheCode(t *testing.T) {
	t.Parallel()
	h := newOAuthHarness(t)
	s := startSignIn(t, h.Server)
	kid := login(t, h.Server, "kid", "pw")
	_, _ = postJSON(t, h.Server, "/v1/auth/device/approve", map[string]string{"userCode": s.userCode}, kid)
	back, _ := url.Parse(continueSignIn(t, h.Server, kid, s.then).Header.Get("Location"))
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {back.Query().Get("code")},
		"client_id": {s.clientID}, "code_verifier": {"not-the-verifier-not-the-verifier-not-the-verifier"},
	}
	if code, out := exchange(t, h.Server, form); code != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("wrong verifier = %d %v, want invalid_grant", code, out)
	}
	form.Set("code_verifier", s.verifier)
	if code, out := exchange(t, h.Server, form); code != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("the right verifier after a wrong one = %d %v, want invalid_grant (the code is spent)", code, out)
	}
}

// Only the person who approved finishes the sign-in, and before anyone approves, continue goes
// back to the approval screen rather than issuing a code.
func TestMCPOAuthContinueBelongsToTheApprover(t *testing.T) {
	t.Parallel()
	h := newOAuthHarness(t)
	s := startSignIn(t, h.Server)
	kid := login(t, h.Server, "kid", "pw")
	boss := login(t, h.Server, "boss", "pw")

	if resp := continueSignIn(t, h.Server, kid, s.then); resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "/pair?") {
		t.Fatalf("continue before approval = %d to %q, want back to /pair", resp.StatusCode, resp.Header.Get("Location"))
	}
	_, _ = postJSON(t, h.Server, "/v1/auth/device/approve", map[string]string{"userCode": s.userCode}, kid)
	if resp := continueSignIn(t, h.Server, boss, s.then); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("continue by someone who did not approve = %d, want 403", resp.StatusCode)
	}
	if resp := continueSignIn(t, h.Server, nil, s.then); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("continue signed out = %d, want 401", resp.StatusCode)
	}
}

// Until the redirect address is checked, an error is shown, never sent to it; and registration
// refuses an address that would send the code across the network in the clear.
func TestMCPOAuthNeverRedirectsToAnUncheckedAddress(t *testing.T) {
	t.Parallel()
	h := newOAuthHarness(t)
	s := startSignIn(t, h.Server)
	_, challenge := pkce()
	q := url.Values{
		"response_type": {"code"}, "client_id": {s.clientID}, "redirect_uri": {"https://evil.example/callback"},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}
	resp, err := noFollow(h.Server, nil).Get(h.Server.URL + "/v1/oauth/authorize?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || resp.Header.Get("Location") != "" {
		t.Fatalf("unregistered redirect_uri = %d to %q, want a 400 and no redirect", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, err = h.Server.Client().Post(h.Server.URL+"/v1/oauth/register", "application/json",
		strings.NewReader(`{"client_name":"x","redirect_uris":["http://lan-box.example/callback"]}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("plain-http non-loopback redirect registered: %d", resp.StatusCode)
	}
}

// With no public address set, there is no sign-in to discover: the challenge names no metadata
// and the metadata is absent, so a client falls back to a pasted token instead of a dead end.
func TestMCPOAuthIsOffWithoutAPublicAddress(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	resp, err := h.Server.Client().Post(h.Server.URL+"/v1/mcp", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || strings.Contains(resp.Header.Get("WWW-Authenticate"), "resource_metadata") {
		t.Fatalf("no public address: %d %q, want a 401 naming no metadata", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
	if code := getJSON(t, h.Server.Client(), h.Server.URL+"/.well-known/oauth-authorization-server", nil); code != http.StatusNotFound {
		t.Fatalf("server metadata without a public address = %d, want 404", code)
	}
}
