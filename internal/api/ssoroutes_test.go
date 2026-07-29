package api_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeSSO scripts the credential path so the ROUTES can be tested without an OIDC provider.
// The flow itself is covered against a real signing IdP in internal/auth.
type fakeSSO struct {
	available bool
	authURL   string
	startErr  error

	exchangeErr error
	user        store.User
	claims      auth.SSOClaims
	returnTo    string

	startedWith []string // every returnTo Start was called with
}

func (f *fakeSSO) Available() bool { return f.available }

func (f *fakeSSO) Start(_ context.Context, returnTo string) (string, string, error) {
	f.startedWith = append(f.startedWith, returnTo)
	if f.startErr != nil {
		return "", "", f.startErr
	}
	return f.authURL, "state-1", nil
}

func (f *fakeSSO) Exchange(_ context.Context, _, _ string) (string, time.Time, store.User, auth.SSOClaims, string, error) {
	if f.exchangeErr != nil {
		return "", time.Time{}, store.User{}, f.claims, f.returnTo, f.exchangeErr
	}
	return "session-token", time.Now().Add(time.Hour), f.user, f.claims, f.returnTo, nil
}

// noRedirectClient stops the test client following 302s, so assertions land on the redirect
// itself rather than on wherever it points.
func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func serverWithSSO(t *testing.T, svc api.SSOService) *httptest.Server {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/sso.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st,
		Auth:  api.NewTokenAuthorizer(adminToken),
		Log:   slog.New(slog.DiscardHandler),
		SSO:   svc,
	}))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := noRedirectClient().Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// With no provider wired the routes are NOT MOUNTED — a 404, not a 501. An unconfigured
// install should look like one that never had SSO, so nothing offers a button that cannot work.
func TestSSORoutes_NotMountedWithoutAProvider(t *testing.T) {
	srv := serverWithSSO(t, nil)
	for _, path := range []string{"/v1/auth/sso/start", "/v1/auth/sso/callback"} {
		resp := get(t, srv, path)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s with no provider → %d, want 404 (route absent)", path, resp.StatusCode)
		}
	}
}

func TestSSOStart_RedirectsToTheProvider(t *testing.T) {
	fake := &fakeSSO{available: true, authURL: "https://auth.test/authorize?state=state-1"}
	srv := serverWithSSO(t, fake)

	resp := get(t, srv, "/v1/auth/sso/start")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("start → %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != fake.authURL {
		t.Errorf("Location = %q, want the provider URL", loc)
	}
}

// ⚠ **THE OPEN-REDIRECT GATE.** `next` must never carry the browser off-site. An absolute URL
// here would let an attacker send a Loomarr link that bounces to a look-alike login page —
// with our domain in the address bar right up to the hop.
//
// `//evil.test` is in the table deliberately: browsers treat a protocol-relative URL as
// off-site despite it looking like a path, which is the case a naive `HasPrefix("/")` check
// lets through.
func TestSSOStart_RefusesAnOffSiteReturnPath(t *testing.T) {
	for _, next := range []string{
		"https://evil.test/login",
		"http://evil.test",
		"//evil.test",
		"//evil.test/path",
		"javascript:alert(1)",
		"https://loomarr.test.evil.test/",
	} {
		t.Run(next, func(t *testing.T) {
			fake := &fakeSSO{available: true, authURL: "https://auth.test/authorize"}
			srv := serverWithSSO(t, fake)

			resp := get(t, srv, "/v1/auth/sso/start?next="+next)
			if resp.StatusCode != http.StatusFound {
				t.Fatalf("start → %d, want 302", resp.StatusCode)
			}
			// The off-site value must have been DROPPED before it could be remembered as
			// somewhere to land after signing in.
			if len(fake.startedWith) != 1 || fake.startedWith[0] != "" {
				t.Errorf("returnTo = %q, want it dropped — an off-site path is an open redirect", fake.startedWith)
			}
		})
	}
}

// A same-app path IS kept, so a deep link survives the round trip — otherwise the guard
// above would be indistinguishable from dropping every value.
func TestSSOStart_KeepsASameAppReturnPath(t *testing.T) {
	fake := &fakeSSO{available: true, authURL: "https://auth.test/authorize"}
	srv := serverWithSSO(t, fake)

	resp := get(t, srv, "/v1/auth/sso/start?next=/guide")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("start → %d, want 302", resp.StatusCode)
	}
	if len(fake.startedWith) != 1 || fake.startedWith[0] != "/guide" {
		t.Errorf("returnTo = %v, want [/guide]", fake.startedWith)
	}
}

// A successful callback issues the session cookie and lands the person where they started.
func TestSSOCallback_IssuesTheSessionAndReturns(t *testing.T) {
	srv := serverWithSSO(t, &fakeSSO{
		available: true,
		user:      store.User{ID: "u-sam", Name: "sam", Role: "member"},
		claims:    auth.SSOClaims{PreferredUsername: "sam"},
		returnTo:  "/guide",
	})

	resp := get(t, srv, "/v1/auth/sso/callback?state=state-1&code=abc")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("callback → %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/guide" {
		t.Errorf("Location = %q, want the deep link the login started from", loc)
	}
	if !strings.Contains(resp.Header.Get("Set-Cookie"), auth.CookieName) {
		t.Errorf("no session cookie in %q", resp.Header.Get("Set-Cookie"))
	}
}

// ⚠ Every refusal lands on the login screen with a REASON CODE, never a server message.
// Reflecting server text into a URL the browser renders is how a redirect becomes a phishing
// surface; a fixed vocabulary the frontend owns cannot put an attacker's words on our page.
func TestSSOCallback_RefusalsCarryAReasonCodeAndNoCookie(t *testing.T) {
	for _, tc := range []struct {
		name, wantReason string
		err              error
	}{
		{"no allowlist row", "sso_no_account", auth.ErrInvalidCredentials},
		{"stale or forged state", "sso_expired", auth.ErrSSOStateMismatch},
		{"provider not configured", "sso_unavailable", auth.ErrSSONotConfigured},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := serverWithSSO(t, &fakeSSO{available: true, exchangeErr: tc.err})

			resp := get(t, srv, "/v1/auth/sso/callback?state=s&code=c")
			if resp.StatusCode != http.StatusFound {
				t.Fatalf("callback → %d, want 302", resp.StatusCode)
			}
			loc := resp.Header.Get("Location")
			if !strings.HasPrefix(loc, "/login?sso=") || !strings.Contains(loc, tc.wantReason) {
				t.Errorf("Location = %q, want /login?sso=%s", loc, tc.wantReason)
			}
			// ⚠ The part that matters: a refusal must not hand out a session.
			//
			// Checks for a cookie with a VALUE, not merely the cookie's name: dropping the
			// early return on the refusal path still emits `Set-Cookie` — with an empty
			// token — so a name-only assertion passed against that sabotage and proved
			// nothing. The bug it must catch is "a refusal issued a usable session".
			for _, c := range resp.Cookies() {
				if c.Name == auth.CookieName && c.Value != "" {
					t.Errorf("a refused SSO login issued a session cookie (%q)", c.Value)
				}
			}
		})
	}
}

// A provider that refuses before we see a code (the person cancelled, or their client is
// misconfigured) says so, rather than reporting a state mismatch that points at us.
func TestSSOCallback_ProviderErrorIsItsOwnReason(t *testing.T) {
	srv := serverWithSSO(t, &fakeSSO{available: true})

	resp := get(t, srv, "/v1/auth/sso/callback?error=access_denied&error_description=user+cancelled")
	if loc := resp.Header.Get("Location"); !strings.Contains(loc, "sso_provider_error") {
		t.Errorf("Location = %q, want the provider-error reason", loc)
	}
}
