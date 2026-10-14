package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/loomarr/loomarr/internal/store"
)

const firefoxOnMac = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14.5; rv:128.0) Gecko/20100101 Firefox/128.0"

// People "Last seen" and "Where you're signed in" (#1667), end to end through the real session
// stack: a request on a session cookie is what records the use, so the roster and the session
// list read it back without any other write.
func TestLastSeen_SessionUseShowsOnRosterAndSessionList(t *testing.T) {
	harness := newAuthFlowHarness(t)
	srv := harness.Server
	seedImported(t, harness.Store, "u-never", "never", store.RoleMember)
	admin := login(t, srv, "boss", "pw")
	member := login(t, srv, "kid", "pw")

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/me", nil)
	req.AddCookie(member)
	req.Header.Set("User-Agent", firefoxOnMac)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("member me → %d, want 200", resp.StatusCode)
	}

	resp = authed(t, http.MethodGet, srv.URL+"/v1/users", admin, "")
	var roster struct {
		Users []struct {
			ID         string `json:"id"`
			LastSeenAt int64  `json:"lastSeenAt"`
		} `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&roster); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	seen := map[string]int64{}
	for _, u := range roster.Users {
		seen[u.ID] = u.LastSeenAt
	}
	if seen["u-kid"] == 0 {
		t.Errorf("member who just used their session has no lastSeenAt on the roster: %+v", roster.Users)
	}
	if got, ok := seen["u-never"]; !ok || got != 0 {
		t.Errorf("never-seen person lastSeenAt = %d (listed %v), want absent", got, ok)
	}

	resp = authed(t, http.MethodGet, srv.URL+"/v1/users/u-kid/sessions", admin, "")
	var list struct {
		Sessions []struct {
			ClientLabel string `json:"clientLabel"`
			LastSeenAt  int64  `json:"lastSeenAt"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if len(list.Sessions) != 1 {
		t.Fatalf("member sessions = %d, want 1", len(list.Sessions))
	}
	if got := list.Sessions[0]; got.ClientLabel != "Firefox on macOS" || got.LastSeenAt == 0 {
		t.Errorf("session = %+v, want clientLabel \"Firefox on macOS\" and a lastSeenAt", got)
	}
}
