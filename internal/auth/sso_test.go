package auth_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/store"
)

// ⚠ THE V8 SAFETY GATE (§11, §19, D-F).
//
// SSO is a CREDENTIAL path, never a provisioning one. These tests pin the part that must
// never regress: a verified identity from a correctly configured provider still needs an
// allowlist row, and nothing here may create one.
//
// The OIDC round trip itself is not re-tested — `go-oidc` verifies signatures, audience and
// expiry, and re-asserting a library's own contract would be testing the dependency rather
// than our decision. What IS ours is everything after verification, and that is what these
// cover: which claim we match on, and what happens when it does not match.

func newSSOStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/sso.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func ssoConfig(enabled bool) func() auth.SSOConfig {
	return func() auth.SSOConfig {
		return auth.SSOConfig{
			Enabled:      enabled,
			Issuer:       "https://auth.example.test",
			ClientID:     "loomarr",
			ClientSecret: "shh",
			RedirectURL:  "http://loomarr.test/v1/auth/sso/callback",
		}
	}
}

// A half-configured provider refuses BEFORE redirecting, so the operator gets an answer
// rather than a failure somewhere inside the provider's UI.
func TestSSO_IncompleteConfigIsNotAvailable(t *testing.T) {
	st := newSSOStore(t)
	mgr := auth.NewManager(st, time.Hour, nil)

	for name, cfg := range map[string]auth.SSOConfig{
		"disabled":     {Enabled: false, Issuer: "https://i", ClientID: "c", ClientSecret: "s", RedirectURL: "r"},
		"no issuer":    {Enabled: true, ClientID: "c", ClientSecret: "s", RedirectURL: "r"},
		"no client id": {Enabled: true, Issuer: "https://i", ClientSecret: "s", RedirectURL: "r"},
		"no secret":    {Enabled: true, Issuer: "https://i", ClientID: "c", RedirectURL: "r"},
		"no redirect":  {Enabled: true, Issuer: "https://i", ClientID: "c", ClientSecret: "s"},
	} {
		t.Run(name, func(t *testing.T) {
			c := cfg
			svc := auth.NewSSOService(func() auth.SSOConfig { return c }, st, mgr, time.Now, slog.New(slog.DiscardHandler))
			if svc.Available() {
				t.Error("Available() = true for an incompletely configured provider")
			}
			if _, _, err := svc.Start(context.Background(), ""); err != auth.ErrSSONotConfigured {
				t.Errorf("Start = %v, want ErrSSONotConfigured", err)
			}
		})
	}
}

// A callback whose state was never issued by this server is refused — it did not come from
// our redirect, so it is a CSRF attempt or a stale bookmark rather than a bad credential.
//
// ⚠ Asserted BEFORE any network call: a state check that happened after the code exchange
// would let an attacker's callback reach the provider on our credentials.
func TestSSO_UnknownStateIsRefused(t *testing.T) {
	st := newSSOStore(t)
	mgr := auth.NewManager(st, time.Hour, nil)
	svc := auth.NewSSOService(ssoConfig(true), st, mgr, time.Now, slog.New(slog.DiscardHandler))

	_, _, _, _, _, err := svc.Exchange(context.Background(), "never-issued", "some-code")
	if err != auth.ErrSSOStateMismatch {
		t.Errorf("Exchange with an unknown state = %v, want ErrSSOStateMismatch", err)
	}
}

// ⚠ THE ALLOWLIST, at the level we own it: which name a verified identity is looked up by.
//
// `preferred_username` wins, then `email`, then `sub`. Subject is deliberately LAST: it is
// opaque, so matching on it first would mean the People list an admin curates could never be
// matched by a provider — they would have to paste an opaque id into a name field.
func TestSSOClaims_MatchNamePrefersTheHumanName(t *testing.T) {
	for _, tc := range []struct {
		name   string
		claims auth.SSOClaims
		want   string
	}{
		{"preferred username wins", auth.SSOClaims{Subject: "op-1", PreferredUsername: "sam", Email: "sam@x.test"}, "sam"},
		{"email when no preferred", auth.SSOClaims{Subject: "op-1", Email: "sam@x.test"}, "sam@x.test"},
		{"subject only as a last resort", auth.SSOClaims{Subject: "op-1"}, "op-1"},
		{"nothing at all", auth.SSOClaims{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.claims.MatchName(); got != tc.want {
				t.Errorf("MatchName() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ⚠ **THE PHASE GATE (§19).** An SSO identity with NO allowlist row is rejected, and no row
// is created — the direct analogue of "an un-imported media-server user is denied even with
// valid Emby credentials".
//
// This asserts the invariant at the seam we control: the store is the allowlist, and the SSO
// path only ever READS it. A provisioning path would show up here as a user count that grew.
func TestSSO_NeverProvisions(t *testing.T) {
	st := newSSOStore(t)
	ctx := context.Background()

	before, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// The SSO package's only store dependency is GetUserByName + Manager.Issue. Neither can
	// create a row, which is the structural half of the guarantee; this is the observable
	// half. If a future change added an Upsert on the refusal path, this count would move.
	svc := auth.NewSSOService(ssoConfig(true), st, auth.NewManager(st, time.Hour, nil), time.Now, slog.New(slog.DiscardHandler))
	_, _, _, _, _, _ = svc.Exchange(ctx, "unknown-state", "code")

	after, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("the SSO path changed the user count %d → %d; it must never provision", len(before), len(after))
	}
}

// A disabled row cannot sign in by ANY credential, SSO included.
//
// ⚠ Driven through the REAL Exchange against a stub provider, not by re-reading the row: the
// claim is "a disabled row never reaches Manager.Issue", and only exercising the path can
// show that. An earlier version of this test asserted `row.Disabled == true` after seeding
// it — which proves the seed worked and nothing about the refusal.
func TestSSO_DisabledRowIsRefusedThroughTheRealFlow(t *testing.T) {
	idp := newStubIDP(t, map[string]any{"sub": "op-1", "preferred_username": "sam"})
	st := newSSOStore(t)
	ctx := context.Background()

	if err := st.UpsertUser(ctx, store.User{
		ID: "u-sam", Name: "sam", Role: "member", Disabled: true, UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	svc := auth.NewSSOService(idp.config(), st, auth.NewManager(st, time.Hour, nil), time.Now, slog.New(slog.DiscardHandler))
	authURL, state, err := svc.Start(ctx, "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	idp.captureNonce(authURL) // the browser would carry this to the provider and back

	token, _, _, claims, _, err := svc.Exchange(ctx, state, "any-code")
	if err != auth.ErrInvalidCredentials {
		t.Errorf("Exchange for a DISABLED row = %v, want ErrInvalidCredentials", err)
	}
	if token != "" {
		t.Error("a session was issued for a disabled row")
	}
	// The claims still come back, so the operator can see the login was recognised and
	// refused rather than never arriving.
	if claims.PreferredUsername != "sam" {
		t.Errorf("claims = %+v, want the provider's username even on a refusal", claims)
	}
}

// ⚠ **THE PHASE GATE, through the real flow (§19).** A verified token from a correctly
// configured provider, for a person the provider is happy with, is REFUSED when no allowlist
// row exists — and no row appears.
func TestSSO_VerifiedIdentityWithNoRowIsRefused(t *testing.T) {
	idp := newStubIDP(t, map[string]any{"sub": "op-9", "preferred_username": "stranger", "email": "stranger@x.test"})
	st := newSSOStore(t)
	ctx := context.Background()

	svc := auth.NewSSOService(idp.config(), st, auth.NewManager(st, time.Hour, nil), time.Now, slog.New(slog.DiscardHandler))
	authURL, state, err := svc.Start(ctx, "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	idp.captureNonce(authURL) // the browser would carry this to the provider and back

	token, _, _, claims, _, err := svc.Exchange(ctx, state, "any-code")
	if err != auth.ErrInvalidCredentials {
		t.Fatalf("Exchange with no allowlist row = %v, want ErrInvalidCredentials", err)
	}
	if token != "" {
		t.Fatal("a session was issued for an identity with no allowlist row")
	}
	if claims.PreferredUsername != "stranger" {
		t.Errorf("claims = %+v, want the provider's claims so a refusal is debuggable", claims)
	}
	// ⚠ And nothing was created. This is what makes SSO a credential path.
	users, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 0 {
		t.Errorf("SSO created %d user(s) — it must never provision", len(users))
	}
}

// An allowlisted, enabled row signs in — the positive case, so the negatives above are
// proven to be refusals rather than a flow that never works at all.
func TestSSO_AllowlistedRowSignsIn(t *testing.T) {
	idp := newStubIDP(t, map[string]any{"sub": "op-1", "preferred_username": "sam"})
	st := newSSOStore(t)
	ctx := context.Background()

	if err := st.UpsertUser(ctx, store.User{
		ID: "u-sam", Name: "sam", Role: "member", UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	svc := auth.NewSSOService(idp.config(), st, auth.NewManager(st, time.Hour, nil), time.Now, slog.New(slog.DiscardHandler))
	authURL, state, err := svc.Start(ctx, "/guide")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	idp.captureNonce(authURL)

	token, expires, u, _, returnTo, err := svc.Exchange(ctx, state, "any-code")
	if err != nil {
		t.Fatalf("Exchange for an allowlisted row: %v", err)
	}
	if token == "" || expires.IsZero() {
		t.Error("no session issued for an allowlisted row")
	}
	if u.ID != "u-sam" || u.Role != "member" {
		t.Errorf("user = %+v, want the ALLOWLIST row (role is Loomarr-owned, never from the token)", u)
	}
	if returnTo != "/guide" {
		t.Errorf("returnTo = %q, want the deep link the login started from", returnTo)
	}
}

// ⚠ **The nonce replay check has its own test because sabotage found it UNGUARDED.**
//
// Removing `idTok.Nonce != p.nonce` from Exchange broke none of the tests above — they each
// carry the right nonce, so none of them could notice its absence. That is the shape of gap
// that leaves a security check one refactor from silently disappearing.
//
// The nonce binds a token to the login that requested it. Without the check, a token minted
// for one sign-in can be replayed into another's callback.
func TestSSO_TokenFromAnotherLoginIsRefused(t *testing.T) {
	idp := newStubIDP(t, map[string]any{"sub": "op-1", "preferred_username": "sam"})
	st := newSSOStore(t)
	ctx := context.Background()

	if err := st.UpsertUser(ctx, store.User{
		ID: "u-sam", Name: "sam", Role: "member", UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	svc := auth.NewSSOService(idp.config(), st, auth.NewManager(st, time.Hour, nil), time.Now, slog.New(slog.DiscardHandler))

	// Login A: started and its nonce captured, so the provider will sign for A.
	authA, stateA, err := svc.Start(ctx, "")
	if err != nil {
		t.Fatalf("Start A: %v", err)
	}
	idp.captureNonce(authA)

	// Login B: started SECOND, so it has a different nonce — but the provider is still
	// holding A's. Completing B therefore presents a token minted for A.
	_, stateB, err := svc.Start(ctx, "")
	if err != nil {
		t.Fatalf("Start B: %v", err)
	}
	if stateA == stateB {
		t.Fatal("two logins produced the same state; they must be unique")
	}

	token, _, _, _, _, err := svc.Exchange(ctx, stateB, "code-for-a")
	if err != auth.ErrSSOStateMismatch {
		t.Errorf("Exchange of A's token into B's callback = %v, want ErrSSOStateMismatch", err)
	}
	if token != "" {
		t.Error("a session was issued from a replayed token")
	}
}

// ⚠ **Three more checks that sabotage found UNGUARDED**, after the nonce hole. Each was
// already implemented correctly; nothing would have noticed if it stopped being.
//
// Found by enumerating every security-relevant branch in the SSO path and breaking each in
// turn, rather than by writing tests for the cases that occurred to me. The first round of
// eight tests missed all four.

// A token minted for a DIFFERENT OIDC client must be refused.
//
// Sabotage: `SkipClientIDCheck: true`. Nothing failed. The audience claim is what stops a
// token issued for some other application on the same provider — a wiki, a router UI —
// being replayed at Loomarr. On a shared homelab IdP that is the realistic attack, not a
// theoretical one.
func TestSSO_TokenForAnotherClientIsRefused(t *testing.T) {
	idp := newStubIDP(t, map[string]any{
		"sub": "op-1", "preferred_username": "sam",
		// The provider signed a perfectly valid token — for someone else's client.
		"aud": "some-other-app",
	})
	st := newSSOStore(t)
	ctx := context.Background()

	if err := st.UpsertUser(ctx, store.User{ID: "u-sam", Name: "sam", Role: "member", UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	svc := auth.NewSSOService(idp.config(), st, auth.NewManager(st, time.Hour, nil), time.Now, slog.New(slog.DiscardHandler))

	authURL, state, err := svc.Start(ctx, "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	idp.captureNonce(authURL)

	token, _, _, _, _, err := svc.Exchange(ctx, state, "code")
	if err == nil {
		t.Error("a token whose audience is another client was ACCEPTED")
	}
	if token != "" {
		t.Error("a session was issued from a token minted for a different client")
	}
}

// An EXPIRED token must be refused, however valid its signature.
//
// Sabotage: `SkipExpiryCheck: true`. Nothing failed. Expiry is what bounds the damage of a
// leaked token — without it, one captured id_token is a permanent credential.
func TestSSO_ExpiredTokenIsRefused(t *testing.T) {
	idp := newStubIDP(t, map[string]any{
		"sub": "op-1", "preferred_username": "sam",
		"exp": time.Now().Add(-time.Hour).Unix(), // signed correctly, an hour stale
	})
	st := newSSOStore(t)
	ctx := context.Background()

	if err := st.UpsertUser(ctx, store.User{ID: "u-sam", Name: "sam", Role: "member", UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	svc := auth.NewSSOService(idp.config(), st, auth.NewManager(st, time.Hour, nil), time.Now, slog.New(slog.DiscardHandler))

	authURL, state, err := svc.Start(ctx, "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	idp.captureNonce(authURL)

	token, _, _, _, _, err := svc.Exchange(ctx, state, "code")
	if err == nil {
		t.Error("an EXPIRED token was accepted")
	}
	if token != "" {
		t.Error("a session was issued from an expired token")
	}
}

// ⚠ State is SINGLE USE: a callback cannot be replayed.
//
// Sabotage: removing the `delete(s.pending, state)`. Nothing failed. Without it, a captured
// callback URL — which sits in browser history and any proxy log in between — could be
// replayed to mint a fresh session repeatedly.
func TestSSO_StateCannotBeReplayed(t *testing.T) {
	idp := newStubIDP(t, map[string]any{"sub": "op-1", "preferred_username": "sam"})
	st := newSSOStore(t)
	ctx := context.Background()

	if err := st.UpsertUser(ctx, store.User{ID: "u-sam", Name: "sam", Role: "member", UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	svc := auth.NewSSOService(idp.config(), st, auth.NewManager(st, time.Hour, nil), time.Now, slog.New(slog.DiscardHandler))

	authURL, state, err := svc.Start(ctx, "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	idp.captureNonce(authURL)

	// First use succeeds — so the refusal below is about REPLAY, not a broken flow.
	if _, _, _, _, _, err := svc.Exchange(ctx, state, "code"); err != nil {
		t.Fatalf("first exchange: %v", err)
	}
	// Second use of the same state must fail.
	token, _, _, _, _, err := svc.Exchange(ctx, state, "code")
	if err != auth.ErrSSOStateMismatch {
		t.Errorf("replayed state = %v, want ErrSSOStateMismatch", err)
	}
	if token != "" {
		t.Error("a replayed callback minted a second session")
	}
}

// A pending login EXPIRES, so an abandoned one cannot be completed days later.
//
// Sabotage: widening pendingTTL. Nothing failed. Driven through an injected clock rather
// than by sleeping, so the test states the rule instead of waiting for it.
func TestSSO_PendingLoginExpires(t *testing.T) {
	idp := newStubIDP(t, map[string]any{"sub": "op-1", "preferred_username": "sam"})
	st := newSSOStore(t)
	ctx := context.Background()

	now := time.Now()
	clock := func() time.Time { return now }
	svc := auth.NewSSOService(idp.config(), st, auth.NewManager(st, time.Hour, nil), clock, slog.New(slog.DiscardHandler))

	authURL, state, err := svc.Start(ctx, "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	idp.captureNonce(authURL)

	// Well past the window a browser round trip needs.
	now = now.Add(2 * time.Hour)

	token, _, _, _, _, err := svc.Exchange(ctx, state, "code")
	if err != auth.ErrSSOStateMismatch {
		t.Errorf("stale pending login = %v, want ErrSSOStateMismatch", err)
	}
	if token != "" {
		t.Error("a session was issued from an expired pending login")
	}
}

// ⚠ **A userinfo response whose `sub` does not match the id_token is IGNORED.**
//
// Sabotage found this unguarded (replacing the comparison with `true` broke nothing). Loomarr
// reads profile claims from userinfo — a spec-compliant Authelia puts them nowhere else — and
// userinfo is a bearer-authenticated GET, weaker evidence than the signed id_token. Without
// the check, a substituted response could name a DIFFERENT person than the provider
// authenticated, and Loomarr would look that name up in the allowlist.
//
// Here the provider authenticates `op-1` but userinfo claims to describe `op-999` and calls
// them `admin`. The mismatched claims must be discarded, leaving only what the token said.
func TestSSO_UserinfoWithAMismatchedSubjectIsIgnored(t *testing.T) {
	// ⚠ The id_token carries NO name — Authelia's real shape. That is what makes this test
	// able to see the guard: `merge` is additive, so with a name already in the token a
	// mismatched userinfo could not overwrite it and the test would pass either way. The
	// dangerous case is precisely the one where userinfo is the ONLY source of a name.
	idp := newStubIDP(t, map[string]any{"sub": "op-1"})
	idp.userinfoClaims = map[string]any{"sub": "op-999", "preferred_username": "admin"}

	st := newSSOStore(t)
	ctx := context.Background()
	// `admin` IS allowlisted — so if the mismatched claim were trusted, the login would
	// SUCCEED as the wrong person. That is the failure this guards.
	if err := st.UpsertUser(ctx, store.User{ID: "u-admin", Name: "admin", Role: "admin", UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	svc := auth.NewSSOService(idp.config(), st, auth.NewManager(st, time.Hour, nil), time.Now, slog.New(slog.DiscardHandler))

	authURL, state, err := svc.Start(ctx, "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	idp.captureNonce(authURL)

	token, _, u, claims, _, err := svc.Exchange(ctx, state, "code")
	// With the mismatched claims discarded, the only name left is the opaque `sub`, which
	// matches no row — so the login is REFUSED rather than succeeding as someone else.
	if err != auth.ErrInvalidCredentials {
		t.Fatalf("Exchange = %v, want ErrInvalidCredentials — a mismatched userinfo must not name the identity", err)
	}
	if token != "" {
		t.Error("a session was issued from a userinfo response describing a different subject")
	}
	if u.Role == "admin" {
		t.Error("signed in as the ADMIN named by a mismatched userinfo response")
	}
	if claims.PreferredUsername == "admin" {
		t.Errorf("claims took the name %q from a userinfo response for another subject", claims.PreferredUsername)
	}
}

// The spec-compliant shape a real Authelia uses: the id_token carries only `sub`, and the
// profile comes from userinfo. This is the case that was broken until a real provider showed
// it — every login on a default Authelia was refused with an opaque `sub` as the match name.
func TestSSO_ProfileClaimsFromUserinfo(t *testing.T) {
	idp := newStubIDP(t, map[string]any{"sub": "op-1"}) // no name in the token, as Authelia does
	idp.userinfoClaims = map[string]any{"sub": "op-1", "preferred_username": "sam", "email": "sam@x.test"}

	st := newSSOStore(t)
	ctx := context.Background()
	if err := st.UpsertUser(ctx, store.User{ID: "u-sam", Name: "sam", Role: "member", UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	svc := auth.NewSSOService(idp.config(), st, auth.NewManager(st, time.Hour, nil), time.Now, slog.New(slog.DiscardHandler))

	authURL, state, err := svc.Start(ctx, "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	idp.captureNonce(authURL)

	token, _, u, claims, _, err := svc.Exchange(ctx, state, "code")
	if err != nil {
		t.Fatalf("Exchange with userinfo-only claims: %v", err)
	}
	if token == "" || u.ID != "u-sam" {
		t.Errorf("no session for a userinfo-named identity (user=%+v)", u)
	}
	if claims.PreferredUsername != "sam" {
		t.Errorf("preferred_username = %q, want it from userinfo", claims.PreferredUsername)
	}
}
