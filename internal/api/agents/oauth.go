package agents

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/people/auth"
)

// OAuth 2.1 for MCP clients that cannot take a pasted token: the Claude apps' connectors, and
// anything else that follows the MCP authorization spec (RFC 9728 resource metadata, RFC 8414
// server metadata, RFC 7591 registration, PKCE with S256).
//
// IT IS DEVICE PAIRING WITH A REDIRECT. Loomarr already has one way to give a program a credential
// that acts as a person: pair it as a device, approved on /pair. OAuth here is that flow with the
// two ends a browser-based client needs:
//
//	authorize  → start a pairing, send the browser to /pair with its code and a way back
//	/pair      → the person signs in and approves it (the existing screen, the existing consent)
//	continue   → the approver's browser is sent to the client with a one-time code
//	token      → code + PKCE verifier → the paired device's token, the same credential
//	             `loomarr pair` prints
//
// So the access token IS a device token: it appears in the person's device list, acts with their
// role, and is revoked like any other device. It does not expire (no device token does) and there
// is no refresh token; revoking the device is how it ends.
//
// The in-flight state (a request waiting for approval, a code waiting for exchange) is in memory:
// a restart in the ten minutes a person takes to approve costs one retry, which is cheaper than a
// table nobody reads after the flow.

const (
	// oauthCodeTTL is how long a code may wait for the client's token request; the client makes it
	// the instant its redirect lands, so a minute is generous.
	oauthCodeTTL = time.Minute
	// maxInFlight caps each in-memory map. An authorize request is unauthenticated, so this is what
	// stops a flood of them from growing memory; the pairing limiter stops it earlier.
	maxInFlight = 256
	// maxOAuthBody bounds a registration or token request body.
	maxOAuthBody = 16 << 10
	// continuePath is where /pair sends the approver once they approve; the web app follows only
	// this prefix (web/apps/web/src/routes/pair.tsx).
	continuePath = "/v1/oauth/continue"
	// maxState bounds the client's opaque state, which is held until the flow ends and echoed back.
	maxState = 1024
)

// authRequest is an authorization waiting for its pairing to be approved.
type authRequest struct {
	deviceCode string
	client     oauthClient
	redirect   string
	challenge  string
	state      string
	expires    time.Time
}

// authCode is an approved authorization waiting for the client's token request.
type authCode struct {
	deviceCode string
	clientID   string
	redirect   string
	challenge  string
	expires    time.Time
}

type oauthFlows struct {
	mu       sync.Mutex
	requests map[string]authRequest
	codes    map[string]authCode
}

func newOAuthFlows() *oauthFlows {
	return &oauthFlows{requests: map[string]authRequest{}, codes: map[string]authCode{}}
}

// sweep drops what has expired. Callers hold mu.
func (f *oauthFlows) sweep(now time.Time) {
	for k, v := range f.requests {
		if now.After(v.expires) {
			delete(f.requests, k)
		}
	}
	for k, v := range f.codes {
		if now.After(v.expires) {
			delete(f.codes, k)
		}
	}
}

func randomID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (Go 1.24+)
	return base64.RawURLEncoding.EncodeToString(b)
}

// issuer is this server's OAuth identity: its public address, without a trailing slash. Empty
// means none is configured, and OAuth is off: a client discovers nothing and keeps to a token.
func (h *Handlers) issuer() string {
	if h.deps.PublicURL == nil {
		return ""
	}
	raw := strings.TrimRight(strings.TrimSpace(h.deps.PublicURL()), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return raw
}

func (h *Handlers) resourceURL(issuer string) string { return issuer + "/v1/mcp" }

// registerOAuth mounts the metadata, registration, authorization and token endpoints. All are
// Hidden from the OpenAPI document for the reason /v1/mcp is: their contract is an RFC, not ours.
func (h *Handlers) registerOAuth(api huma.API) {
	raw := func(id, method, path, summary string, role httpcore.Role, body func(http.ResponseWriter, *http.Request)) {
		httpcore.RawOp[struct{}](api, httpcore.BytesResponse(huma.Operation{
			OperationID: id, Method: method, Path: path, Summary: summary,
			Tags: []string{"Agents"}, Hidden: true,
		}, "The RFC-defined reply", "application/json"), role, body)
	}
	raw("oauth-protected-resource", http.MethodGet, "/.well-known/oauth-protected-resource", "OAuth protected-resource metadata (RFC 9728)", httpcore.RolePublic, h.resourceMetadata)
	raw("oauth-protected-resource-mcp", http.MethodGet, "/.well-known/oauth-protected-resource/v1/mcp", "OAuth protected-resource metadata for /v1/mcp (RFC 9728)", httpcore.RolePublic, h.resourceMetadata)
	raw("oauth-authorization-server", http.MethodGet, "/.well-known/oauth-authorization-server", "OAuth authorization-server metadata (RFC 8414)", httpcore.RolePublic, h.serverMetadata)
	raw("oauth-register", http.MethodPost, "/v1/oauth/register", "Register an OAuth client (RFC 7591)", httpcore.RolePublic, h.register)
	raw("oauth-authorize", http.MethodGet, "/v1/oauth/authorize", "Start an OAuth authorization", httpcore.RolePublic, h.authorize)
	raw("oauth-request", http.MethodGet, "/v1/oauth/request", "Describe an OAuth authorization waiting for approval", httpcore.RoleMember, h.describeRequest)
	raw("oauth-continue", http.MethodGet, continuePath, "Finish an OAuth authorization after approving its pairing", httpcore.RoleMember, h.continueAuthorization)
	raw("oauth-token", http.MethodPost, "/v1/oauth/token", "Exchange an OAuth code for a token", httpcore.RolePublic, h.token)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// oauthError is an RFC 6749 §5.2 error body; OAuth clients read `error`, not problem+json.
func oauthError(w http.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

func (h *Handlers) resourceMetadata(w http.ResponseWriter, _ *http.Request) {
	iss := h.issuer()
	if iss == "" {
		oauthError(w, http.StatusNotFound, "not_configured", "Set Loomarr's public address to sign agents in with OAuth.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 h.resourceURL(iss),
		"authorization_servers":    []string{iss},
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "Loomarr",
	})
}

func (h *Handlers) serverMetadata(w http.ResponseWriter, _ *http.Request) {
	iss := h.issuer()
	if iss == "" {
		oauthError(w, http.StatusNotFound, "not_configured", "Set Loomarr's public address to sign agents in with OAuth.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                         iss,
		"authorization_endpoint":                         iss + "/v1/oauth/authorize",
		"token_endpoint":                                 iss + "/v1/oauth/token",
		"registration_endpoint":                          iss + "/v1/oauth/register",
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"authorization_response_iss_parameter_supported": true,
	})
}

func (h *Handlers) register(w http.ResponseWriter, r *http.Request) {
	if h.issuer() == "" {
		oauthError(w, http.StatusNotFound, "not_configured", "Set Loomarr's public address to sign agents in with OAuth.")
		return
	}
	var in struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
		AuthMethod   string   `json:"token_endpoint_auth_method"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxOAuthBody)).Decode(&in); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "The registration is not valid JSON.")
		return
	}
	if in.AuthMethod != "" && in.AuthMethod != "none" {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "Only public clients (token_endpoint_auth_method none, with PKCE) are supported.")
		return
	}
	c, err := newOAuthClient(in.ClientName, in.RedirectURIs)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  c.id(),
		"client_id_issued_at":        time.Now().Unix(),
		"client_name":                c.Name,
		"redirect_uris":              c.Redirects,
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
}

// authorize starts a pairing for the client and sends the browser to approve it.
//
// Until the redirect address is known good, an error is shown here (a 400), never sent to it:
// redirecting to an unchecked address is the open redirect RFC 6749 §4.1.2.1 forbids.
func (h *Handlers) authorize(w http.ResponseWriter, r *http.Request) {
	iss := h.issuer()
	if iss == "" || h.deps.Devices == nil {
		httpcore.WriteProblem(h.log, w, r, http.StatusNotFound, "Not configured", "Set Loomarr's public address to sign agents in with OAuth.")
		return
	}
	q := r.URL.Query()
	client, err := parseClientID(q.Get("client_id"))
	if err != nil {
		httpcore.WriteProblem(h.log, w, r, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	redirect, err := client.redirectFor(q.Get("redirect_uri"))
	if err != nil {
		httpcore.WriteProblem(h.log, w, r, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	state := q.Get("state")
	if len(state) > maxState {
		state = "" // never echo it back; the client sees the refusal without it
	}
	fail := func(code, description string) {
		http.Redirect(w, r, withParams(redirect, url.Values{
			"error": {code}, "error_description": {description}, "state": {state}, "iss": {iss},
		}), http.StatusFound)
	}
	switch {
	case q.Get("response_type") != "code":
		fail("unsupported_response_type", "Only response_type=code is supported.")
		return
	case len(q.Get("state")) > maxState:
		fail("invalid_request", "state is too long.")
		return
	case q.Get("code_challenge_method") != "S256" || !isPKCEValue(q.Get("code_challenge")):
		fail("invalid_request", "PKCE with code_challenge_method=S256 is required.")
		return
	case q.Get("resource") != "" && strings.TrimRight(q.Get("resource"), "/") != h.resourceURL(iss) && strings.TrimRight(q.Get("resource"), "/") != iss:
		fail("invalid_target", "This server issues tokens only for "+h.resourceURL(iss)+".")
		return
	}
	// An unauthenticated request that writes a row, so it is rate limited like a TV starting a
	// pairing, but in its OWN bucket: a flood of sign-ins must not stop the household's TVs pairing.
	if h.deps.DeviceLimiter != nil && !h.deps.DeviceLimiter.Allow("oauth|"+auth.ThrottleSource(h.deps.Origin.ClientIP(r))) {
		fail("temporarily_unavailable", "Too many sign-ins started; wait a minute and try again.")
		return
	}
	// A slot is reserved BEFORE the pairing row is written, so a full table refuses without
	// leaving a row behind.
	id := randomID()
	h.flows.mu.Lock()
	h.flows.sweep(time.Now())
	full := len(h.flows.requests) >= maxInFlight
	if !full {
		h.flows.requests[id] = authRequest{expires: time.Now().Add(auth.PairingTTL)}
	}
	h.flows.mu.Unlock()
	if full {
		fail("temporarily_unavailable", "Too many sign-ins in progress; try again shortly.")
		return
	}
	deviceCode, userCode, expires, err := h.deps.Devices.StartPairing(r.Context(), client.deviceName(redirect))
	h.flows.mu.Lock()
	if err != nil {
		delete(h.flows.requests, id)
	} else {
		h.flows.requests[id] = authRequest{
			deviceCode: deviceCode, client: client, redirect: redirect,
			challenge: q.Get("code_challenge"), state: state, expires: expires,
		}
	}
	h.flows.mu.Unlock()
	if err != nil {
		fail("server_error", "Couldn't start the sign-in.")
		return
	}
	http.Redirect(w, r, pairPage(userCode, id), http.StatusFound)
}

// isPKCEValue reports whether s has the shape RFC 7636 §4.1 gives a verifier, which an S256
// challenge (43 base64url characters) also has: 43 to 128 unreserved characters.
func isPKCEValue(s string) bool {
	if len(s) < 43 || len(s) > 128 {
		return false
	}
	for _, c := range []byte(s) {
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-', c == '.', c == '_', c == '~':
		default:
			return false
		}
	}
	return true
}

// describeRequest tells /pair, BEFORE anyone approves, which app is asking and where the grant
// will be sent. The request id is 256 random bits that only the browser sent to /pair holds, so
// knowing it is the authorisation to ask; being signed in is required as for approving.
func (h *Handlers) describeRequest(w http.ResponseWriter, r *http.Request) {
	h.flows.mu.Lock()
	req, ok := h.flows.requests[r.URL.Query().Get("request")]
	h.flows.mu.Unlock()
	if !ok || req.deviceCode == "" || time.Now().After(req.expires) {
		httpcore.WriteProblem(h.log, w, r, http.StatusNotFound, "Sign-in expired",
			"This sign-in has expired or was already finished. Start it again from the app.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app":      req.client.Name,
		"sendsTo":  sendsTo(req.redirect),
		"loopback": isLoopback(sendsTo(req.redirect)),
	})
}

// pairPage is the approval screen for one sign-in: the code filled in, and `next` naming the step
// that finishes it once someone approves. The web app accepts only that exact path shape in `next`.
func pairPage(userCode, requestID string) string {
	next := continuePath + "?" + url.Values{"request": {requestID}}.Encode()
	return "/pair?" + url.Values{"code": {userCode}, "next": {next}}.Encode()
}

// continueAuthorization runs in the approver's own browser right after they approve on /pair. It
// checks that THEY approved this pairing, then sends them to the client with a one-time code.
func (h *Handlers) continueAuthorization(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("request")
	h.flows.mu.Lock()
	req, ok := h.flows.requests[id]
	h.flows.mu.Unlock()
	if !ok || req.deviceCode == "" || time.Now().After(req.expires) {
		httpcore.WriteProblem(h.log, w, r, http.StatusNotFound, "Sign-in expired",
			"This sign-in has expired or was already finished. Start it again from the app.")
		return
	}
	user, _ := httpcore.UserFrom(r.Context())
	pairing, err := h.deps.Devices.Pairing(r.Context(), req.deviceCode)
	if err != nil {
		httpcore.WriteProblem(h.log, w, r, http.StatusNotFound, "Sign-in expired",
			"This sign-in has expired. Start it again from the app.")
		return
	}
	if !pairing.Approved() {
		// Back to the approval screen for this same code, rather than a dead end.
		http.Redirect(w, r, pairPage(pairing.UserCode, id), http.StatusFound)
		return
	}
	if user.ID == "" || pairing.UserID != user.ID {
		// Someone else approved it: the grant is theirs to finish, not this browser's.
		httpcore.WriteProblem(h.log, w, r, http.StatusForbidden, "Forbidden", "This sign-in was approved by someone else.")
		return
	}
	// Taking the request and issuing its code happen under one lock, so two continues racing on
	// the same request issue one code, not two.
	code := randomID()
	h.flows.mu.Lock()
	_, still := h.flows.requests[id]
	delete(h.flows.requests, id)
	h.flows.sweep(time.Now())
	full := len(h.flows.codes) >= maxInFlight
	if still && !full {
		h.flows.codes[code] = authCode{
			deviceCode: req.deviceCode, clientID: req.client.id(), redirect: req.redirect,
			challenge: req.challenge, expires: time.Now().Add(oauthCodeTTL),
		}
	}
	h.flows.mu.Unlock()
	switch {
	case !still:
		httpcore.WriteProblem(h.log, w, r, http.StatusNotFound, "Sign-in expired",
			"This sign-in was already finished. Start it again from the app.")
		return
	case full:
		httpcore.WriteProblem(h.log, w, r, http.StatusServiceUnavailable, "Busy", "Too many sign-ins in progress; try again shortly.")
		return
	}
	http.Redirect(w, r, withParams(req.redirect, url.Values{"code": {code}, "state": {req.state}, "iss": {h.issuer()}}), http.StatusFound)
}

// token exchanges a code for the paired device's token (RFC 6749 §4.1.3, PKCE RFC 7636 §4.6).
func (h *Handlers) token(w http.ResponseWriter, r *http.Request) {
	if h.deps.Devices == nil {
		oauthError(w, http.StatusNotFound, "not_configured", "Device pairing is not available.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxOAuthBody)
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "The token request must be a form.")
		return
	}
	if r.PostForm.Get("grant_type") != "authorization_code" {
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "Only grant_type=authorization_code is supported.")
		return
	}
	code := r.PostForm.Get("code")
	h.flows.mu.Lock()
	c, ok := h.flows.codes[code]
	delete(h.flows.codes, code) // single use, even when the rest of the request is wrong
	h.flows.mu.Unlock()
	if !ok || time.Now().After(c.expires) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "The code is unknown, used or expired.")
		return
	}
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	verifier := base64.RawURLEncoding.EncodeToString(sum[:])
	switch {
	case !isPKCEValue(r.PostForm.Get("code_verifier")):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "The code_verifier must be 43 to 128 unreserved characters.")
		return
	case r.PostForm.Get("client_id") != c.clientID:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "The code was issued to another client.")
		return
	case r.PostForm.Get("redirect_uri") != "" && r.PostForm.Get("redirect_uri") != c.redirect:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match the authorization request.")
		return
	case subtle.ConstantTimeCompare([]byte(verifier), []byte(c.challenge)) != 1:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "The code_verifier does not match the code_challenge.")
		return
	}
	token, _, err := h.deps.Devices.Redeem(r.Context(), c.deviceCode)
	if err != nil {
		if errors.Is(err, auth.ErrPairingNotApproved) {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "The sign-in was not approved.")
			return
		}
		oauthError(w, http.StatusBadRequest, "invalid_grant", "The sign-in expired or was already used.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"access_token": token, "token_type": "Bearer"})
}

// withParams adds query parameters to a redirect address, keeping any it already has.
func withParams(raw string, add url.Values) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	for k, vs := range add {
		if len(vs) > 0 && vs[0] != "" {
			q[k] = vs
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
