package integration_test

import (
	"net/http"
	"testing"
)

const chrisID = "b1df9e921c8f4ddb85f5b032f93ebdf4" // a non-admin fixture user

// TestJourney_Member drives the non-admin experience through the REAL composition:
// an admin imports a media-server user (the untested import→member-login chain),
// the member does what members may, is refused on the FULL admin-route matrix (not
// just the titles/users subset the existing §19 test covers — also settings,
// system-llm, setup, filler, backup), and loses their session the instant an admin
// disables them. This is the authorization contract the FE codes every screen
// against; a gap here is exactly an FE surprise.
func TestJourney_Member(t *testing.T) {
	h := newHarness(t)
	admin := h.asAdmin()
	member := h.asMember(admin, "Chris", chrisID)

	// B1: the import→media-server-login chain produced a MEMBER session.
	var me struct {
		Role string `json:"role"`
	}
	h.getJSON("/v1/auth/me", member, &me)
	if me.Role != "member" {
		t.Fatalf("imported user role = %q, want member", me.Role)
	}

	// B2: members MAY do member things (auth passes → not 401/403).
	allowed := []struct{ method, path, body string }{
		{http.MethodGet, "/v1/search?q=matrix", ""},
		{http.MethodGet, "/v1/proposals", ""},
		{http.MethodGet, "/v1/channels", ""},
		{http.MethodGet, "/v1/titles?state=wanted", ""},
		{http.MethodGet, "/v1/filler", ""},
		{http.MethodPost, "/v1/proposals", `{"description":"a cozy mystery channel"}`},
	}
	for _, r := range allowed {
		if code := h.status(r.method, r.path, r.body, member); code == http.StatusForbidden || code == http.StatusUnauthorized {
			t.Errorf("member %s %s → %d, want allowed (not 401/403)", r.method, r.path, code)
		}
	}

	// B3: members are 403 on the FULL admin-route matrix. requireAdmin runs before
	// any nil-dep/501 check, so a member is refused regardless of configuration.
	forbidden := []struct{ method, path, body string }{
		{http.MethodPost, "/v1/titles", `{"mediaType":"movie","tmdbId":1}`},
		{http.MethodDelete, "/v1/titles/movie:tmdb:1", ""},
		{http.MethodGet, "/v1/users", ""},
		{http.MethodPatch, "/v1/users/" + chrisID, `{"disabled":true}`},
		{http.MethodPost, "/v1/users/import", `{"ids":["x"]}`},
		{http.MethodPost, "/v1/users/sync", ""},
		{http.MethodGet, "/v1/settings", ""},
		{http.MethodPatch, "/v1/settings", `{"edits":{}}`},
		{http.MethodPost, "/v1/settings/secrets/api_token/regenerate", ""},
		{http.MethodGet, "/v1/setup/status", ""},
		{http.MethodPost, "/v1/setup/test", `{"check":"media_server"}`},
		{http.MethodPost, "/v1/setup/tunarr-connect", ""},
		{http.MethodGet, "/v1/system/llm", ""},
		{http.MethodPost, "/v1/system/llm/select", `{"provider":"ollama","model":"x"}`},
		{http.MethodPost, "/v1/system/llm/test", `{"provider":"openrouter"}`},
		{http.MethodPost, "/v1/system/llm/pull", `{"model":"x"}`},
		{http.MethodPost, "/v1/channels", `{"id":"x","name":"X","number":1,"strategy":"shuffle"}`},
		{http.MethodDelete, "/v1/channels/x", ""},
		{http.MethodPost, "/v1/channels/x/reconcile", ""},
		{http.MethodPost, "/v1/proposals/x/approve", ""},
		{http.MethodPost, "/v1/proposals/x/deny", `{}`},
		{http.MethodPost, "/v1/filler/sync", ""},
		{http.MethodPost, "/v1/filler/tag", ""},
		{http.MethodPatch, "/v1/filler/x", `{"era":1990}`},
		{http.MethodGet, "/v1/backup", ""},
	}
	for _, r := range forbidden {
		if code := h.status(r.method, r.path, r.body, member); code != http.StatusForbidden {
			t.Errorf("member %s %s → %d, want 403", r.method, r.path, code)
		}
	}

	// B4: an admin disabling the member kills their live session (§19).
	if code := h.status(http.MethodPatch, "/v1/users/"+chrisID, `{"disabled":true}`, admin); code != http.StatusOK {
		t.Fatalf("admin disable member → %d, want 200", code)
	}
	if code := h.status(http.MethodGet, "/v1/auth/me", "", member); code != http.StatusUnauthorized {
		t.Errorf("disabled member's session → %d on /me, want 401", code)
	}
}
