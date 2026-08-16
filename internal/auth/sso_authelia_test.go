//go:build integration

// A REAL Authelia, in a container, driven through the whole OIDC flow (§11, V8).
//
// ⚠ **Why this exists rather than trusting the stub IdP.** The stub in ssostub_test.go signs
// real tokens, but it is *my* reading of the spec on both sides of the wire — so a
// misunderstanding would be invisible: the test would agree with the code because the same
// mistake wrote both. Authelia is the provider a homelab operator actually runs, and it
// implements the parts I might have got wrong (discovery shape, PKCE, claim naming, the
// `preferred_username` Loomarr matches on).
//
// It earned its place: systematic sabotage found FOUR checks in this path guarded by nothing
// — nonce, audience, expiry, single-use state. Four of five, found by breaking the code rather
// than by foresight. That is a poor enough hit rate to want a real provider in the loop.
//
// Runs under the `integration` tag so the default `make test` stays Docker-free (§19), like
// the Postgres conformance suite. `make test-sso` runs it.
package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/store"
)

// autheliaImage is pinned. A floating tag would make this test's meaning change under us:
// "SSO works" would silently become "SSO works with whatever Authelia shipped today", and a
// break would look like our regression.
const autheliaImage = "authelia/authelia:4.39.20"

const (
	ssoClientID     = "loomarr"
	ssoClientSecret = "loomarr-test-secret"
	ssoUser         = "sam"
	ssoPassword     = "sam-password-that-is-long-enough"
)

// TestSSO_AgainstRealAuthelia drives Start → Authelia's consent → callback → Exchange against
// a real provider, and asserts the two outcomes that matter most: an allowlisted person gets
// in, and a verified identity with no row does not.
func TestSSO_AgainstRealAuthelia(t *testing.T) {
	idp := startAuthelia(t)
	st := newSSOStore(t)
	ctx := idp.ctx()

	// Only `sam` is allowlisted. Authelia will happily authenticate `stranger` too, which is
	// exactly the case §11 exists for.
	if err := st.UpsertUser(ctx, store.User{
		ID: "u-sam", Name: ssoUser, Role: "member", UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	svc := auth.NewSSOService(idp.ssoConfig(), st, auth.NewManager(st, time.Hour, nil), time.Now, slog.New(slog.DiscardHandler))

	t.Run("an allowlisted person signs in", func(t *testing.T) {
		authURL, state, err := svc.Start(ctx, "/guide")
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		code := idp.signInAndGetCode(t, authURL, ssoUser, ssoPassword)

		token, expires, u, claims, returnTo, err := svc.Exchange(ctx, state, code)
		if err != nil {
			t.Fatalf("Exchange against real Authelia: %v", err)
		}
		if token == "" || expires.IsZero() {
			t.Error("no session issued")
		}
		if u.ID != "u-sam" {
			t.Errorf("user = %+v, want the ALLOWLIST row", u)
		}
		// ⚠ Role comes from OUR row, never the provider (§11). Authelia knows nothing about
		// Loomarr roles, and this proves we do not invent one from its claims.
		if u.Role != "member" {
			t.Errorf("role = %q, want member from the allowlist row", u.Role)
		}
		// The claim Loomarr matches on, as a real provider actually spells it.
		if claims.PreferredUsername != ssoUser {
			t.Errorf("preferred_username = %q, want %q", claims.PreferredUsername, ssoUser)
		}
		if returnTo != "/guide" {
			t.Errorf("returnTo = %q, want the deep link", returnTo)
		}
	})

	t.Run("a verified identity with no allowlist row is refused", func(t *testing.T) {
		// ⚠ **THE PHASE GATE against a real provider.** Authelia authenticates `stranger`
		// perfectly well — the refusal is entirely Loomarr's allowlist doing its job.
		authURL, state, err := svc.Start(ctx, "")
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		code := idp.signInAndGetCode(t, authURL, "stranger", ssoPassword)

		token, _, _, claims, _, err := svc.Exchange(ctx, state, code)
		if err != auth.ErrInvalidCredentials {
			t.Fatalf("Exchange for an un-allowlisted identity = %v, want ErrInvalidCredentials", err)
		}
		if token != "" {
			t.Fatal("a session was issued to someone with no allowlist row")
		}
		// The claims still come back, so an operator can see WHICH identity was refused.
		if claims.PreferredUsername != "stranger" {
			t.Errorf("claims = %+v, want the provider's claims on a refusal", claims)
		}
		// And nothing was created.
		users, err := st.ListUsers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 {
			t.Errorf("user count = %d, want 1 — SSO must never provision", len(users))
		}
	})
}

// --- the container ---

type autheliaIDP struct {
	baseURL  string
	redirect string
	// client trusts the container's self-signed cert. Passed to the service via
	// oidc.ClientContext, never wired into production.
	client *http.Client
}

// ctx returns a context carrying the trusting HTTP client, which is how the SSO service
// reaches this provider without any production change (§11, V8).
func (a autheliaIDP) ctx() context.Context {
	return oidc.ClientContext(context.Background(), a.client)
}

func (a autheliaIDP) ssoConfig() func() auth.SSOConfig {
	return func() auth.SSOConfig {
		return auth.SSOConfig{
			Enabled:      true,
			Issuer:       a.baseURL,
			ClientID:     ssoClientID,
			ClientSecret: ssoClientSecret,
			RedirectURL:  a.redirect,
		}
	}
}

func startAuthelia(t *testing.T) autheliaIDP {
	t.Helper()

	// Authelia hashes client secrets, and the hash embeds a random salt — so it is generated
	// here rather than pinned, using Authelia's own tool. Pinning a hash would mean pinning a
	// salt, and a future Authelia changing its default parameters would fail confusingly.
	secretHash := autheliaHash(t, ssoClientSecret)
	userHash := autheliaHash(t, ssoPassword)

	// ⚠ A FIXED port, and `docker run` rather than testcontainers — both forced by Authelia,
	// not preference.
	//
	// Authelia needs its own public URL inside the config BEFORE it starts: the session cookie
	// domain must match the URL the client dials, and the OIDC issuer is derived from that
	// match (a mismatch 400s discovery with "no session cookie configuration matches url").
	// A dynamically-mapped port is unknowable at config-writing time, and this testcontainers
	// version does not accept a fixed host binding without taking docker/docker as a direct
	// dependency — a poor trade for a port (§14).
	//
	// The honest cost: two concurrent runs on one machine collide. Acceptable for a tagged
	// integration test, and a collision fails loudly on bind rather than subtly.
	const hostPort = 19091
	base := fmt.Sprintf("https://127.0.0.1:%d", hostPort)

	// A redirect URI nothing has to serve: the BROWSER would follow it, and this test plays
	// the browser and stops at the redirect to read the code. So it only has to match what the
	// service sends and what Authelia has registered.
	redirect := "http://loomarr.test/v1/auth/sso/callback"

	// The cert's SAN must cover the address the test dials (an IP SAN — a DNS name would not
	// match 127.0.0.1).
	certPEM, keyPEM := selfSignedCert(t)

	dir := t.TempDir()
	write := func(name, content string, mode os.FileMode) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("configuration.yml", autheliaConfig(secretHash, redirect, base), 0o644)
	write("users.yml", autheliaUsers(userHash), 0o644)
	write("tls.crt", certPEM, 0o644)
	write("tls.key", keyPEM, 0o600)

	name := fmt.Sprintf("loomarr-sso-test-%d", time.Now().UnixNano())
	out, err := exec.Command("docker", "run", "-d", "--name", name,
		"-p", fmt.Sprintf("127.0.0.1:%d:9091", hostPort),
		"-v", dir+":/config", autheliaImage).CombinedOutput()
	if err != nil {
		t.Fatalf("start authelia: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		// Logs on failure: Authelia names its own config objections precisely, and without
		// them a failure here reads as "the container did not start" with no cause.
		if t.Failed() {
			if logs, lerr := exec.Command("docker", "logs", name).CombinedOutput(); lerr == nil {
				t.Logf("authelia logs:\n%s", logs)
			}
		}
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})

	client := trustingClient(t, certPEM)
	waitForDiscovery(t, base, client)
	return autheliaIDP{baseURL: base, redirect: redirect, client: client}
}

// autheliaHash runs Authelia's own hashing tool, so the digest format is whatever this
// version expects rather than whatever I remember it being.
func autheliaHash(t *testing.T, password string) string {
	t.Helper()
	out, err := exec.Command("docker", "run", "--rm", "--entrypoint", "/app/authelia",
		autheliaImage, "crypto", "hash", "generate", "pbkdf2", "--password", password).CombinedOutput()
	if err != nil {
		t.Fatalf("authelia hash: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if _, digest, ok := strings.Cut(line, "Digest: "); ok {
			return strings.TrimSpace(digest)
		}
	}
	t.Fatalf("no digest in authelia output:\n%s", out)
	return ""
}

func waitForDiscovery(t *testing.T, base string, client *http.Client) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/.well-known/openid-configuration")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	// Report WHAT the provider said, not just that it did not work — the previous version of
	// this loop swallowed a 400 whose body named the exact cause.
	resp, err := client.Get(base + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("authelia discovery unreachable at %s: %v", base, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	t.Fatalf("authelia discovery never became available: %s → %d %s", base, resp.StatusCode, body)
}

// signInAndGetCode plays the browser: it posts the password to Authelia's API, follows the
// consent flow, and returns the `code` from the redirect Loomarr would have received.
//
// Driving Authelia's real endpoints rather than minting a code ourselves is the point — a
// hand-made code would only prove our own exchange call compiles.
func (a autheliaIDP) signInAndGetCode(t *testing.T, authURL, username, password string) string {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:       jar,
		Transport: a.client.Transport, // the same pinned root; this plays the browser
		// Stop at the redirect carrying the code — following it would leave the test chasing
		// a host that does not exist.
		CheckRedirect: func(r *http.Request, _ []*http.Request) error {
			if strings.HasPrefix(r.URL.String(), a.redirect) {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	// 1. First-factor sign-in. `targetURL` is the authorize URL, so Authelia knows where to
	//    send the browser once the password is accepted.
	body := fmt.Sprintf(`{"username":%q,"password":%q,"targetURL":%q,"requestMethod":"GET","keepMeLoggedIn":false}`,
		username, password, authURL)
	resp, err := client.Post(a.baseURL+"/api/firstfactor", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("authelia firstfactor: %v", err)
	}
	first, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authelia firstfactor → %d: %s", resp.StatusCode, first)
	}

	// 2. Follow the authorize URL now that a session cookie exists. Authelia issues the code
	//    and redirects to our registered URI.
	resp, err = client.Get(authURL)
	if err != nil {
		t.Fatalf("authelia authorize: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	loc := resp.Header.Get("Location")
	if loc == "" {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("authorize did not redirect (status %d): %s", resp.StatusCode, payload)
	}
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse redirect %q: %v", loc, err)
	}
	code := u.Query().Get("code")
	if code == "" {
		t.Fatalf("no code in redirect %q (error=%q)", loc, u.Query().Get("error"))
	}
	return code
}

// autheliaConfig is the minimum that turns on the OIDC provider. Deliberately small: every
// extra knob is a thing that can break for reasons unrelated to what this test asserts.
func autheliaConfig(clientSecretHash, redirect, baseURL string) string {
	return fmt.Sprintf(`
server:
  address: 'tcp://0.0.0.0:9091'
  tls:
    # ⚠ TLS is not incidental here. Authelia derives its OIDC issuer from the request and
    # REFUSES plain HTTP outright ("invalid X-Forwarded-Proto header value 'http'"), which the
    # stub IdP never did — a real provider will not talk to Loomarr over http. Serving real
    # TLS keeps the issuer consistent with what the test dials, instead of reaching for
    # go-oidc's InsecureIssuerURLContext, which would mean adding an issuer-override path to
    # PRODUCTION auth code so a test could pass.
    certificate: '/config/tls.crt'
    key: '/config/tls.key'
log:
  level: 'info'
authentication_backend:
  file:
    path: '/config/users.yml'
session:
  secret: 'insecure_test_session_secret_at_least_64_characters_long_for_authelia' # retired-ok: Authelia fixture key, not Loomarr configuration
  cookies:
    # ⚠ Must MATCH the URL the client dials, or discovery 400s with "no session cookie
    # configuration matches url" — Authelia derives its issuer from this. And it must be an IP
    # or contain a period: it rejects bare 'localhost' outright.
    - domain: '127.0.0.1'
      authelia_url: '%[5]s'
storage:
  encryption_key: 'insecure_test_storage_key_at_least_64_characters_long_for_authelia'
  local:
    path: '/config/db.sqlite3'
notifier:
  filesystem:
    filename: '/config/notification.txt'
identity_validation:
  reset_password:
    # Required by Authelia unless password reset is disabled entirely. Supplied rather than
    # fought with: this test never exercises reset, so the value only has to satisfy the
    # config validator.
    jwt_lifespan: '5 minutes'
    jwt_secret: 'insecure_test_reset_secret_at_least_64_characters_long_for_authelia'
access_control:
  default_policy: 'one_factor'
identity_providers:
  oidc:
    hmac_secret: 'insecure_test_hmac_secret_at_least_64_characters_long_for_authelia'
    jwks:
      - key_id: 'test'
        algorithm: 'RS256'
        use: 'sig'
        key: |
%[1]s
    clients:
      - client_id: '%[2]s'
        client_name: 'Loomarr'
        client_secret: '%[3]s'
        public: false
        authorization_policy: 'one_factor'
        require_pkce: false
        consent_mode: 'implicit'
        redirect_uris:
          - '%[4]s'
        scopes:
          - 'openid'
          - 'profile'
          - 'email'
        userinfo_signed_response_alg: 'none'
`, indent(testSigningKey, 10), ssoClientID, clientSecretHash, redirect, baseURL)
}

func autheliaUsers(passwordHash string) string {
	return fmt.Sprintf(`
users:
  %s:
    disabled: false
    displayname: 'Sam Example'
    password: '%s'
    email: 'sam@loomarr.test'
    groups: ['loomarr-admins']
  stranger:
    disabled: false
    displayname: 'A Stranger'
    password: '%s'
    email: 'stranger@loomarr.test'
    groups: ['loomarr-admins']
`, ssoUser, passwordHash, passwordHash)
}

func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

// ⚠ A TEST KEY, generated for this file and used nowhere else. It is committed on purpose —
// the test must be reproducible without generating an RSA key on every run (~1s), and a key
// that only ever signs tokens for a throwaway container inside a test grants nothing.
//
// `stranger` is deliberately in `loomarr-admins`: the group means NOTHING to Loomarr (§11
// keeps roles Loomarr-owned), and having a real provider assert group membership that we then
// ignore is the strongest available proof we do not read roles from claims.
const testSigningKey = `-----BEGIN PRIVATE KEY-----
MIIEvwIBADANBgkqhkiG9w0BAQEFAASCBKkwggSlAgEAAoIBAQDI0vw+PAZMwsj1
ZspiIFQe45deDX2+RGFEG/WoU7IZBNQLC7k8PVRrVlPw8/gFzPVnRVPE59benIbT
jr2Aa3nk23+DZf1wt5cZlcoGpOCQTbTmZtfHtpT99TaaimtPRxSN4e6pJ5q8neOM
k87IZHZWvrNK9RiTqdFzqYAKXGSSSeKurjo68JPGXZrnBaNU8TwSL5FXcOmLL5uY
38W+2imA1qR16+pKJJztbkRJ5CdTFvXnUhGlyrO35wz1euKFijqvhyGd8Xh7WCP3
8cJSMTLCMq3S0G4bGX6/v+qrKM8zB09DqMDyq7So0qzlRRBeLSaCMFIE3wV8w+Gz
ZStceZ5DAgMBAAECggEAFljJUjDiZT707GnTHMw1es0fQOCwx1qwlXWkW7tvLAg0
2EMmX6UAydjstPlQ9hmdN9qnvFeCuiQiKf8jkzC30Fb51NNP5Rpo0/iuZxgzF5uu
BPLDZtPTDIIcNgzabWieyZMEjbTR1n9InOdYW/A3QuaBk6u7tQ1xOIpPVy0k0SpO
Z1TbHRMuY5Sf4GkFf9ah2zEIvdQ27yVctmTNcdpN2OkY5wrHVkaxNfByjgNp29O2
NAGEJ7pIU5QkhDOucuU3QBk+rwEb1pfSBDUzGMMAe1NCH4aLTwSwWmdoj0srqID6
IegGdyEJnx5vwowuCkBENghze0RDTI0pTDoNZ553YQKBgQDvkxwgTfQkLmrrqnzE
iuLorzdyXbjmQbH4mUQr702h6Tf5I7gRTn+WdJLk5FnWydQKDDJLwYGZo3o2wdpk
TM7NTHtVKAPjDvyaKn/s+QIeZuioGrQRniQ9sV3j8dIzmHlGhZLTIhqAHeSLxd0E
WojRVuAPrVa4Qo6mQpNYaiZ1mwKBgQDWl786pRVO5ZvVXp+82U9NQuZiXib18lpT
yiBwaX/bJLQoxuwJD78RHxjPboErMGTOnNwiVbO+xKEpULWUSxdq3C739QjLbF6b
JI2K//5V00htS2f4CVmIU0PNnuEp/vnfu9VfLsec9YzKebgEHMnynFkHWqdoOor1
+urOsuiYeQKBgQDOR5+HHTfi02jSpAgr/t4jtYKLYbFr6RMBV46AOdthVvsP9LZv
iGSJOrSkiw3jyAJf6GKCIgqiLEV10nJlvFCwKnNjPkIihmvvnjpp43n0jW00GVIE
fWL9D7QlKblxHY8GrspeKtYgjByWUPbd4co+XYwtU3YAz6Yd9+MA1N1qkwKBgQCS
8bRr5xFRRl9QW4xMmA36nP3/i/Nn5T2/NKDD+SopGNgZOCX3CoZOphmqKURgG7Jb
3QPMqxz7W8/z56V/V3BAp2euOWd9TMb3u68E6MjzYkutM76NFXHurP239ry+si/O
6eNxWyorK+Xt3C2K+1+6Nx+rroMGF1iCmgBh7BbkGQKBgQCtPm1gypvdqfPaekL0
ZjMFNrAZ9ylaa8eA3LFGAGn3qSYOr70MQnMfesRfLVHDt2X0sxu2UXs1LLMDolO7
GZO5JbmhRznrzvwDOyp9sK2JaP6KBVnziOvO2TQEmtxtBHKZJeUfnD5M1tbu+w9m
+F00T5yXkhWKf9mPNLgxZLaY/Q==
-----END PRIVATE KEY-----`

// selfSignedCert mints a cert valid for 127.0.0.1, which is where testcontainers maps the
// port. An IP SAN rather than a DNS name: the test dials an address, not a hostname.
func selfSignedCert(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "authelia-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	return certPEM, keyPEM
}

// trustingClient trusts exactly the container's cert — a pinned root, not
// InsecureSkipVerify. The distinction matters even in a test: a client that verifies against
// a known root still exercises the TLS path, while one that skips verification would let a
// broken handshake pass unnoticed.
func trustingClient(t *testing.T, certPEM string) *http.Client {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(certPEM)) {
		t.Fatal("could not add the test cert to a pool")
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
	}
}
