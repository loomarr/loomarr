package auth_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/auth"
)

// stubIDP is a real, minimal OIDC provider: it publishes a discovery document and a JWKS,
// and it signs id_tokens with its own RSA key.
//
// ⚠ **Deliberately a REAL signer rather than a stubbed verifier.** The alternative — faking
// verification — would let a test pass while the production path accepted an unsigned token,
// which is the one failure mode that must be impossible here. This way `go-oidc` does the
// same signature, audience, expiry and nonce checks it will do in production, and the tests
// exercise our decisions on top of a genuinely verified identity.
//
// It speaks discovery, token, JWKS and userinfo.
//
// ⚠ **Userinfo was added AFTER a real Authelia proved the stub was misleading.** The stub put
// every claim in the id_token, so it never exercised the spec-compliant shape Authelia uses
// (id_token carries `sub`; the profile lives at userinfo) — and Loomarr read only the
// id_token, so every login against a default Authelia was refused. `userinfoClaims` now lets
// a test choose which style a provider uses, including a MISMATCHED subject.
type stubIDP struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	claims map[string]any
	// userinfoClaims is what /userinfo returns. nil ⇒ the endpoint 404s, modelling a provider
	// that inlines everything in the id_token.
	userinfoClaims map[string]any

	// nonce is captured from the authorize URL the service builds and echoed back inside the
	// signed token, so the service's replay check has something real to compare. Held per
	// instance rather than in a package var: two tests in one package would otherwise
	// interfere through it.
	mu    sync.Mutex
	nonce string
	// tokenForm is the last form the /token endpoint received, so a test can assert what
	// Loomarr actually SENT rather than only what it accepted back. Without it a PKCE
	// assertion would pass whether or not the verifier ever left the process.
	tokenForm url.Values
}

func newStubIDP(t *testing.T, claims map[string]any) *stubIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &stubIDP{key: key, claims: claims}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                idp.server.URL,
			"authorization_endpoint":                idp.server.URL + "/authorize",
			"token_endpoint":                        idp.server.URL + "/token",
			"jwks_uri":                              idp.server.URL + "/jwks",
			"userinfo_endpoint":                     idp.server.URL + "/userinfo",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		pub := idp.key.PublicKey
		writeJSON(w, map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "test-key",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		idp.mu.Lock()
		ui := idp.userinfoClaims
		idp.mu.Unlock()
		if ui == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, ui)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		idp.mu.Lock()
		idp.tokenForm = r.Form
		nonce := idp.nonce
		idp.mu.Unlock()
		writeJSON(w, map[string]any{
			"access_token": "stub-access",
			"token_type":   "Bearer",
			"id_token":     idp.signIDToken(t, nonce),
		})
	})

	idp.server = httptest.NewServer(mux)
	t.Cleanup(idp.server.Close)
	return idp
}

// config returns the SSOConfig pointing at this stub, ready for NewSSOService.
func (i *stubIDP) config() func() auth.SSOConfig {
	return func() auth.SSOConfig {
		return auth.SSOConfig{
			Enabled:      true,
			Issuer:       i.server.URL,
			ClientID:     "loomarr",
			ClientSecret: "shh",
			RedirectURL:  "http://loomarr.test/v1/auth/sso/callback",
		}
	}
}

// captureNonce records the nonce from an authorize URL the service produced. Tests call it
// between Start and Exchange, which is exactly where a browser would have carried it.
func (i *stubIDP) captureNonce(authURL string) {
	u, err := url.Parse(authURL)
	if err != nil {
		return
	}
	i.mu.Lock()
	i.nonce = u.Query().Get("nonce")
	i.mu.Unlock()
}

func (i *stubIDP) signIDToken(t *testing.T, nonce string) string {
	t.Helper()
	payload := map[string]any{
		"iss":   i.server.URL,
		"aud":   "loomarr",
		"sub":   "op-unset",
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
		"nonce": nonce,
	}
	for k, v := range i.claims {
		payload[k] = v
	}
	tok, err := signRS256(i.key, payload)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// signRS256 mints a compact JWS. Hand-rolled rather than pulling go-jose into the test:
// signing three base64 segments is a handful of lines, and the VERIFY side — the part that
// must be correct — is the production library's, not this.
func signRS256(key *rsa.PrivateKey, claims map[string]any) (string, error) {
	header, err := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT", "kid": "test-key"})
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signing := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
