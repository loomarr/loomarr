package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/mantonx/loomarr/internal/store"
)

// SSO is the third credential path onto §11's owned identity (D-F, V8).
//
// ⚠ **It proves WHO you are. It never decides WHETHER you may enter.** A verified token
// from a correctly configured provider still resolves against the `users` allowlist, and a
// missing row is a refusal — the direct analogue of "an un-imported media-server user is
// denied even with valid Emby credentials" (§11). There is no code path here that creates,
// promotes, or enables a row; grep this file for Upsert and you will find only the
// login-refresh the other paths already do.
//
// ⚠ **Roles are not read from the token.** A provider claiming group membership is telling
// us about its own world; role is a Loomarr decision. There is deliberately no group or
// admin-claim handling anywhere in this file.
//
// OIDC rather than the mock's forward-auth header mode, deliberately: a signed token carries
// its own proof, whereas header trust is only as strong as the operator's network wiring
// (§11).

// ErrSSONotConfigured is returned when a caller reaches an SSO route while SSO is off or
// incompletely configured. Distinct from a credential failure: nothing was rejected, the
// path simply is not available.
var ErrSSONotConfigured = errors.New("sso is not configured")

// ErrSSOStateMismatch is returned when a callback's state does not match a pending login.
// Its own error because it means the request did not originate from this server's redirect —
// a CSRF attempt or a stale bookmark, not a bad credential.
var ErrSSOStateMismatch = errors.New("sso state does not match a pending login")

// SSOClaims is what the provider told us about the person signing in.
//
// Surfaced to the API on a REFUSAL as well as a success, because the operator's question
// when SSO does not work is "which claim arrived, and why did it not match a row?" — and
// without that they are comparing a provider's config to a user list by guesswork.
type SSOClaims struct {
	Subject           string `json:"sub,omitempty"`
	PreferredUsername string `json:"preferredUsername,omitempty"`
	Email             string `json:"email,omitempty"`
	Name              string `json:"name,omitempty"`
}

// MatchName is the value an SSO login is looked up by in the allowlist.
//
// `preferred_username` first, then `email`, then the subject. NOT the subject first: `sub`
// is stable but opaque, and matching on it would mean the People list an admin curates could
// never be matched by a provider — they would have to paste an opaque id into a name field.
// Matching on a human name keeps ONE allowlist rather than a parallel SSO-identity table.
func (c SSOClaims) MatchName() string {
	if c.PreferredUsername != "" {
		return c.PreferredUsername
	}
	if c.Email != "" {
		return c.Email
	}
	return c.Subject
}

// SSOConfig is the operator's provider settings, read live so a saved change applies without
// a restart (config-design §3).
type SSOConfig struct {
	Enabled      bool
	Issuer       string
	ClientID     string
	ClientSecret string
	// RedirectURL is where the provider sends the browser back. Derived from the server's
	// public URL rather than configured separately: two places to state one address is two
	// places for it to disagree, and a mismatched redirect is the most common OIDC
	// misconfiguration.
	RedirectURL string
}

// ready reports whether every field a login needs is present. Enabled alone is not enough —
// a half-configured provider must refuse rather than fail mid-redirect with the operator
// wondering which field is missing.
func (c SSOConfig) ready() bool {
	return c.Enabled && c.Issuer != "" && c.ClientID != "" && c.ClientSecret != "" && c.RedirectURL != ""
}

// SSOService runs the OIDC authorization-code flow and lands it on §11's allowlist.
type SSOService struct {
	cfg   func() SSOConfig
	store store.Store
	mgr   *Manager
	now   func() time.Time
	log   *slog.Logger

	// pending holds in-flight logins by state. In-memory and per-process on purpose: a
	// login is a single browser round trip measured in seconds, so persistence would add a
	// table and a purge job to hold data whose useful life is shorter than one reconcile
	// tick. A restart mid-login means the operator clicks Sign in again.
	mu      sync.Mutex
	pending map[string]pendingLogin

	// provider is cached per issuer: discovery is an HTTP round trip to the provider's
	// well-known document, and doing it per login would put the provider's availability in
	// the path of every sign-in twice over. Keyed by issuer so a settings change rebuilds.
	providerMu  sync.Mutex
	provider    *oidc.Provider
	providerFor string
}

type pendingLogin struct {
	nonce     string
	returnTo  string
	createdAt time.Time
}

// pendingTTL bounds how long a started login stays valid. Short: it covers a redirect to a
// provider and back, and a longer window is just a larger set of guessable states.
const pendingTTL = 10 * time.Minute

// NewSSOService builds the service. cfg is read per call so an operator's saved change takes
// effect on the next login with no restart.
func NewSSOService(cfg func() SSOConfig, st store.Store, mgr *Manager, now func() time.Time, log *slog.Logger) *SSOService {
	if now == nil {
		now = time.Now
	}
	return &SSOService{cfg: cfg, store: st, mgr: mgr, now: now, log: log, pending: map[string]pendingLogin{}}
}

// Available reports whether SSO can be offered right now, so the login screen shows the
// button only when it would work.
func (s *SSOService) Available() bool { return s.cfg().ready() }

// Start begins a login: it mints state + nonce and returns the provider URL to redirect to.
//
// `returnTo` is remembered so a person deep-linking into the app lands back where they were
// rather than on the dashboard.
func (s *SSOService) Start(ctx context.Context, returnTo string) (authURL, state string, err error) {
	cfg := s.cfg()
	if !cfg.ready() {
		return "", "", ErrSSONotConfigured
	}
	provider, err := s.oidcProvider(ctx, cfg.Issuer)
	if err != nil {
		return "", "", err
	}

	state, err = randomToken()
	if err != nil {
		return "", "", err
	}
	nonce, err := randomToken()
	if err != nil {
		return "", "", err
	}

	s.mu.Lock()
	s.sweepLocked()
	s.pending[state] = pendingLogin{nonce: nonce, returnTo: returnTo, createdAt: s.now()}
	s.mu.Unlock()

	conf := s.oauthConfig(cfg, provider)
	// The nonce goes to the provider AND is kept here; Exchange verifies they match, which
	// is what stops a token minted for one login being replayed into another.
	return conf.AuthCodeURL(state, oidc.Nonce(nonce)), state, nil
}

// Exchange completes the flow: verify the token, then resolve the identity against the
// allowlist.
//
// ⚠ The claims are returned even on FAILURE (alongside the error), so the API can show the
// operator what the provider said. A refusal an operator cannot debug is a refusal they will
// work around by disabling the allowlist.
func (s *SSOService) Exchange(ctx context.Context, state, code string) (token string, expires time.Time, u store.User, claims SSOClaims, returnTo string, err error) {
	cfg := s.cfg()
	if !cfg.ready() {
		return "", time.Time{}, store.User{}, claims, "", ErrSSONotConfigured
	}

	s.mu.Lock()
	s.sweepLocked()
	p, ok := s.pending[state]
	delete(s.pending, state) // single use, whatever the outcome
	s.mu.Unlock()
	if !ok {
		return "", time.Time{}, store.User{}, claims, "", ErrSSOStateMismatch
	}

	provider, err := s.oidcProvider(ctx, cfg.Issuer)
	if err != nil {
		return "", time.Time{}, store.User{}, claims, p.returnTo, err
	}
	conf := s.oauthConfig(cfg, provider)

	tok, err := conf.Exchange(ctx, code)
	if err != nil {
		return "", time.Time{}, store.User{}, claims, p.returnTo, fmt.Errorf("sso: code exchange: %w", err)
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return "", time.Time{}, store.User{}, claims, p.returnTo, errors.New("sso: provider returned no id_token")
	}

	// Verified against the issuer's published keys, with audience and expiry checked by the
	// library. This is the whole reason for the dependency: hand-rolled JWT verification is
	// the kind of security code that looks right and is not.
	idTok, err := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}).Verify(ctx, rawID)
	if err != nil {
		return "", time.Time{}, store.User{}, claims, p.returnTo, fmt.Errorf("sso: id_token: %w", err)
	}
	if idTok.Nonce != p.nonce {
		// A valid token for a login this server did not start.
		return "", time.Time{}, store.User{}, claims, p.returnTo, ErrSSOStateMismatch
	}

	var raw ssoRawClaims
	if err := idTok.Claims(&raw); err != nil {
		return "", time.Time{}, store.User{}, claims, p.returnTo, fmt.Errorf("sso: claims: %w", err)
	}

	// ⚠ **The profile claims come from USERINFO, not the id_token** — found by testing against
	// a real Authelia (4.39), which follows the spec strictly: the id_token carries `sub` and
	// the profile lives at the userinfo endpoint. Reading only the id_token made `MatchName()`
	// fall through to an opaque `sub` UUID, so EVERY login against a default Authelia was
	// refused. A hand-written stub IdP that put everything inline never showed it.
	//
	// Fetched unconditionally rather than only when the id_token lacks a name: userinfo returns
	// the claims either way, and a conditional would leave the most common provider on the
	// less-travelled code path.
	if ui, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(tok)); err == nil {
		var uiClaims ssoRawClaims
		if err := ui.Claims(&uiClaims); err == nil {
			// ⚠ The userinfo `sub` MUST match the token's. Without this a substituted response
			// could name a different person than the one the provider authenticated — the
			// spec requires the check for exactly that reason.
			if ui.Subject == raw.Sub {
				raw.merge(uiClaims)
			} else if s.log != nil {
				s.log.Warn("sso: userinfo subject does not match the id_token; ignoring it",
					"token_sub", raw.Sub, "userinfo_sub", ui.Subject)
			}
		}
	} else if s.log != nil {
		// Not fatal: a provider that inlines claims needs no userinfo call to succeed.
		s.log.Info("sso: userinfo unavailable; using id_token claims", "err", err)
	}

	claims = SSOClaims{Subject: raw.Sub, PreferredUsername: raw.PreferredUsername, Email: raw.Email, Name: raw.Name}

	// ⚠ THE ALLOWLIST. Everything above proves identity; this decides access.
	//
	// A verified token whose name has no row is refused — no row is created, promoted, or
	// enabled. This is the same refusal the imported media-server path makes, and it is
	// what makes SSO a credential path rather than a provisioning one (§11, D-F).
	name := claims.MatchName()
	if name == "" {
		return "", time.Time{}, store.User{}, claims, p.returnTo, ErrInvalidCredentials
	}
	row, err := s.store.GetUserByName(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		s.logRefusal(name, claims)
		return "", time.Time{}, store.User{}, claims, p.returnTo, ErrInvalidCredentials
	}
	if err != nil {
		return "", time.Time{}, store.User{}, claims, p.returnTo, err
	}
	if row.Disabled {
		// Same refusal as every other path: a disabled row cannot sign in by any credential.
		return "", time.Time{}, store.User{}, claims, p.returnTo, ErrInvalidCredentials
	}

	token, expires, err = s.mgr.Issue(ctx, row.ID)
	if err != nil {
		return "", time.Time{}, store.User{}, claims, p.returnTo, err
	}
	return token, expires, row, claims, p.returnTo, nil
}

// ssoRawClaims is the wire shape, read from BOTH the id_token and userinfo.
type ssoRawClaims struct {
	Sub               string `json:"sub"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	Name              string `json:"name"`
}

// merge fills empty fields from another source. Additive only — a claim already present in
// the verified id_token is never overwritten by the userinfo response, which is the weaker
// of the two (the id_token is signed; userinfo is a bearer-authenticated GET).
func (c *ssoRawClaims) merge(other ssoRawClaims) {
	if c.PreferredUsername == "" {
		c.PreferredUsername = other.PreferredUsername
	}
	if c.Email == "" {
		c.Email = other.Email
	}
	if c.Name == "" {
		c.Name = other.Name
	}
}

// logRefusal records a verified-but-unallowlisted login at INFO, not as an error.
//
// It is the expected outcome of an over-broad provider — anyone in the directory can reach
// the callback — so logging it as an error would fill an operator's log with other people's
// correct behaviour. But it must be logged: "SSO does not work for Sam" is unanswerable
// without knowing which name arrived.
func (s *SSOService) logRefusal(name string, c SSOClaims) {
	if s.log == nil {
		return
	}
	s.log.Info("sso login refused: no matching account",
		"matched_on", name, "sub", c.Subject, "email", c.Email)
}

// oidcProvider returns a cached provider for the issuer, discovering it on first use.
func (s *SSOService) oidcProvider(ctx context.Context, issuer string) (*oidc.Provider, error) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	if s.provider != nil && s.providerFor == issuer {
		return s.provider, nil
	}
	p, err := oidc.NewProvider(ctx, strings.TrimRight(issuer, "/"))
	if err != nil {
		return nil, fmt.Errorf("sso: reach provider at %s: %w", issuer, err)
	}
	s.provider, s.providerFor = p, issuer
	return p, nil
}

func (s *SSOService) oauthConfig(cfg SSOConfig, p *oidc.Provider) oauth2.Config {
	return oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  cfg.RedirectURL,
		// The minimum that identifies a person. No groups scope: roles are Loomarr's, so
		// requesting group claims would be asking for data we have decided not to act on.
		Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
	}
}

// sweepLocked drops expired pending logins. Called on each Start/Exchange rather than from a
// ticker: the map only grows when someone starts a login, so the work belongs where the
// growth happens — no goroutine to leak across a restart (§9.2).
func (s *SSOService) sweepLocked() {
	cutoff := s.now().Add(-pendingTTL)
	for k, v := range s.pending {
		if v.createdAt.Before(cutoff) {
			delete(s.pending, k)
		}
	}
}

// randomToken mints a 256-bit URL-safe value for state and nonce.
func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("sso: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
