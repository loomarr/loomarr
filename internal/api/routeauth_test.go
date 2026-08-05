package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
)

// ⚠ **THE AUTHORIZATION GATE (§11, §19).** These pin the invariant the architecture review
// found broken: anonymous callers reached 27 of 83 operations, including one that spent
// LLM tokens.
//
// The shape that allowed it: enforcement lived in a `requireAdmin(ctx)` call inside a
// handler body, so "did this route authorize?" was answerable only by reading every
// handler. The role now rides the operation and the middleware enforces it, which is what
// makes a test like this expressible at all.

// anonReads are routes an unauthenticated caller must NOT reach. Not exhaustive by design —
// TestEveryOperationDeclaresARole is the exhaustive guard; this one states the outcome in
// terms an operator would recognise.
func TestAnonymousIsRefusedOnMemberRoutes(t *testing.T) {
	srv, _ := newServer(t)
	for _, path := range []string{
		"/v1/channels",
		"/v1/titles?state=available",
		"/v1/proposals",
	} {
		t.Run(path, func(t *testing.T) {
			resp := do(t, srv, http.MethodGet, path, "", "")
			defer func() { _ = resp.Body.Close() }()
			// 401, not 403: the caller has no identity, so "sign in" is the actionable
			// answer. 403 would tell someone with no account to go find an admin.
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("anonymous GET %s → %d, want 401", path, resp.StatusCode)
			}
		})
	}
}

// ⚠ **The one that mattered most.** Before this gate, POST /v1/proposals returned 200 to a
// caller with no credential and invoked the LLM — unauthenticated spend, with created_by
// stamped "" because there was nobody to attribute it to.
//
// The fake suggester is FULLY WIRED here on purpose. A test against an unwired server passes
// for the wrong reason: it gets a 501 from the nil-service guard and never reaches
// authorization at all, so it would keep passing with the role check deleted.
func TestAnonymousCannotSpendLLMTokens(t *testing.T) {
	srv, _, fs := newSuggestServer(t)

	resp := do(t, srv, http.MethodPost, "/v1/proposals", "", `{"description":"90s sci-fi"}`)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous submit → %d, want 401", resp.StatusCode)
	}
	// The assertion that actually pins the cost: the suggester must never have been called.
	// A 401 that still ran the job would be a leak the status code hides.
	if fs.submits != 0 {
		t.Errorf("the suggester ran %d time(s) for an anonymous caller — unauthenticated spend", fs.submits)
	}
}

// An admin still gets through, so the negatives above are proven to be refusals rather than
// a route that stopped working for everyone.
func TestAdminStillReachesMemberAndAdminRoutes(t *testing.T) {
	srv, _ := newServer(t)
	for _, tc := range []struct{ path string }{
		{"/v1/channels"},               // member
		{"/v1/settings"},               // admin
		{"/v1/titles?state=available"}, // member
	} {
		resp := do(t, srv, http.MethodGet, tc.path, adminToken, "")
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			t.Errorf("admin GET %s → %d, want it allowed", tc.path, resp.StatusCode)
		}
	}
}

// The pre-authentication routes must stay reachable without a credential — a role check
// that locked these would make the install unenterable, and it would look like a bug in
// login rather than in authorization.
func TestPublicRoutesStayReachableAnonymously(t *testing.T) {
	srv, _ := newServer(t)
	for _, path := range []string{"/v1/setup/state", "/v1/system/version"} {
		resp := do(t, srv, http.MethodGet, path, "", "")
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			t.Errorf("anonymous GET %s → %d, want it public (§11)", path, resp.StatusCode)
		}
	}
}

// ⚠ **THE EXHAUSTIVE GUARD.** Every registered operation must declare a role, so a route
// added without one fails here instead of shipping.
//
// This is the piece that makes the model durable: the review found the gap by probing 83
// routes by hand, which is not a thing anyone repeats. Reading the declarations out of the
// exported spec means the check costs nothing and cannot drift from what is actually
// mounted.
func TestEveryOperationDeclaresARole(t *testing.T) {
	missing, err := api.OperationsMissingARole()
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Errorf("these operations declare no required role — wrap the huma.Operation in withRole(…):\n  %s",
			strings.Join(missing, "\n  "))
	}
}
