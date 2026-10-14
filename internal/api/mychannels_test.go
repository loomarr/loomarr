package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type myChannelsWire struct {
	Favourites []struct {
		ChannelID string `json:"channelId"`
	} `json:"favourites"`
	Recent []struct {
		ChannelID string `json:"channelId"`
	} `json:"recent"`
}

func (w myChannelsWire) String() string {
	fav := make([]string, 0, len(w.Favourites))
	for _, f := range w.Favourites {
		fav = append(fav, f.ChannelID)
	}
	recent := make([]string, 0, len(w.Recent))
	for _, r := range w.Recent {
		recent = append(recent, r.ChannelID)
	}
	return fmt.Sprintf("favourites=%v recent=%v", fav, recent)
}

// pairDevice runs the device-code flow for the signed-in person and returns the TV's token.
func pairDevice(t *testing.T, srv *httptest.Server, session *http.Cookie) string {
	t.Helper()
	_, start := postJSON(t, srv, "/v1/auth/device/start", map[string]string{"deviceName": "Living room TV"}, nil)
	if code, body := postJSON(t, srv, "/v1/auth/device/approve", map[string]string{"userCode": start["userCode"].(string)}, session); code != http.StatusOK {
		t.Fatalf("approve = %d (%v)", code, body)
	}
	code, poll := postJSON(t, srv, "/v1/auth/device/poll", map[string]string{"deviceCode": start["deviceCode"].(string)}, nil)
	if code != http.StatusOK {
		t.Fatalf("poll = %d (%v)", code, poll)
	}
	return poll["token"].(string)
}

// myChannelsCall sends one /v1/me request as a session (cookie) or a bearer (paired TV or API token).
func myChannelsCall(t *testing.T, srv *httptest.Server, method, path string, session *http.Cookie, bearer string) (int, myChannelsWire) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, nil)
	if session != nil {
		req.AddCookie(session)
		req.Header.Set("X-Loomarr-Csrf", "1")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out myChannelsWire
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// N3: a paired TV acts as the person who paired it, so a star or a tune on either device shows on
// the other, and never on anyone else's.
func TestMyChannelsFollowThePersonAcrossDevices(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	srv := h.Server
	for i, id := range []string{"ch-news", "ch-films", "ch-kids"} {
		seedGridChannel(t, h.Store, id, i+1)
	}
	kid := login(t, srv, "kid", "pw")
	boss := login(t, srv, "boss", "pw")
	tv := pairDevice(t, srv, kid)

	if code, got := myChannelsCall(t, srv, http.MethodPut, "/v1/me/favourites/ch-films", kid, ""); code != http.StatusOK ||
		got.String() != "favourites=[ch-films] recent=[]" {
		t.Fatalf("member stars ch-films = %d %s", code, got)
	}
	if code, got := myChannelsCall(t, srv, http.MethodPut, "/v1/me/favourites/ch-news", nil, tv); code != http.StatusOK ||
		got.String() != "favourites=[ch-films ch-news] recent=[]" {
		t.Fatalf("paired TV stars ch-news = %d %s, want the person's list plus ch-news", code, got)
	}
	if code, got := myChannelsCall(t, srv, http.MethodPut, "/v1/me/recent-channels/ch-kids", nil, tv); code != http.StatusOK ||
		got.String() != "favourites=[ch-films ch-news] recent=[ch-kids]" {
		t.Fatalf("paired TV tunes ch-kids = %d %s", code, got)
	}
	if code, got := myChannelsCall(t, srv, http.MethodGet, "/v1/me/channels", kid, ""); code != http.StatusOK ||
		got.String() != "favourites=[ch-films ch-news] recent=[ch-kids]" {
		t.Fatalf("member's own session after the TV's writes = %d %s", code, got)
	}
	if code, got := myChannelsCall(t, srv, http.MethodDelete, "/v1/me/favourites/ch-films", nil, tv); code != http.StatusOK ||
		got.String() != "favourites=[ch-news] recent=[ch-kids]" {
		t.Fatalf("paired TV unstars ch-films = %d %s", code, got)
	}

	// An admin has their own lists; being admin grants no view of anyone else's.
	if code, got := myChannelsCall(t, srv, http.MethodGet, "/v1/me/channels", boss, ""); code != http.StatusOK ||
		got.String() != "favourites=[] recent=[]" {
		t.Fatalf("admin reads = %d %s, want only their own (empty) lists", code, got)
	}
	if code, got := myChannelsCall(t, srv, http.MethodPut, "/v1/me/favourites/ch-kids", boss, ""); code != http.StatusOK ||
		got.String() != "favourites=[ch-kids] recent=[]" {
		t.Fatalf("admin stars ch-kids = %d %s", code, got)
	}
	if _, got := myChannelsCall(t, srv, http.MethodGet, "/v1/me/channels", kid, ""); got.String() != "favourites=[ch-news] recent=[ch-kids]" {
		t.Fatalf("member's lists after the admin's star = %s, want unchanged", got)
	}
}

func TestMyChannelsBelongToAPerson(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	seedGridChannel(t, h.Store, "ch-news", 1)
	for _, tc := range []struct {
		name   string
		bearer string
	}{
		{"anonymous", ""},
		// The break-glass API token is an admin with no person behind it, so it has no lists.
		{"api token", "break-glass-token"},
	} {
		for _, call := range [][2]string{
			{http.MethodGet, "/v1/me/channels"},
			{http.MethodPut, "/v1/me/favourites/ch-news"},
			{http.MethodPut, "/v1/me/recent-channels/ch-news"},
		} {
			if code, _ := myChannelsCall(t, h.Server, call[0], call[1], nil, tc.bearer); code != http.StatusUnauthorized {
				t.Errorf("%s %s %s = %d, want 401", tc.name, call[0], call[1], code)
			}
		}
	}
}

func TestMyChannelsUnknownChannelIsNotFound(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	kid := login(t, h.Server, "kid", "pw")
	for _, path := range []string{"/v1/me/favourites/ch-gone", "/v1/me/recent-channels/ch-gone"} {
		if code, _ := myChannelsCall(t, h.Server, http.MethodPut, path, kid, ""); code != http.StatusNotFound {
			t.Errorf("PUT %s = %d, want 404", path, code)
		}
	}
	// Unstarring a channel that doesn't exist already holds.
	if code, _ := myChannelsCall(t, h.Server, http.MethodDelete, "/v1/me/favourites/ch-gone", kid, ""); code != http.StatusOK {
		t.Errorf("DELETE unknown favourite = %d, want 200", code)
	}
}
