package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// newDeviceHarness builds the full session-auth stack with device pairing and
// an optional approval limiter enabled.
func newDeviceHarness(t *testing.T, limiter *auth.RateLimiter) *apiHarness {
	t.Helper()
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	ms.Accounts = map[string]testkit.Account{
		"boss": {Password: "pw", ID: "u-boss", IsAdmin: true},
		"kid":  {Password: "pw", ID: "u-kid", IsAdmin: false},
	}
	return startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
		mgr := auth.NewManager(defaults.Store, time.Hour, time.Now)
		devices := auth.NewDeviceManager(defaults.Store, time.Now)
		seedImported(t, defaults.Store, "u-boss", "boss", store.RoleAdmin)
		seedImported(t, defaults.Store, "u-kid", "kid", store.RoleMember)

		return api.Router(defaults.Log, api.Options{
			Store:         defaults.Store,
			Auth:          api.NewSessionAuthorizerCurrent(mgr, devices, func(context.Context) (string, error) { return "break-glass-token", nil }),
			Log:           defaults.Log,
			Login:         auth.NewLoginService(lib, defaults.Store, mgr, nil, time.Now),
			Sessions:      mgr,
			Devices:       devices,
			DeviceLimiter: limiter,
			CookieSecure:  "false",
		})
	})
}

func postJSON(t *testing.T, srv *httptest.Server, path string, body any, cookie *http.Cookie) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
		// A cookie-authenticated mutation carries the CSRF header, exactly as the web UI does.
		req.Header.Set("X-Loomarr-Csrf", "1")
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	out := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// The whole handshake over HTTP: start, approve as a human, poll, then use the token.
func TestDevicePairingEndToEndOverHTTP(t *testing.T) {
	t.Parallel()
	srv := newDeviceHarness(t, nil).Server

	code, start := postJSON(t, srv, "/v1/auth/device/start", map[string]string{"deviceName": "Shield"}, nil)
	if code != http.StatusOK {
		t.Fatalf("start = %d, want 200 (%v)", code, start)
	}
	deviceCode, _ := start["deviceCode"].(string)
	userCode, _ := start["userCode"].(string)
	if deviceCode == "" || userCode == "" {
		t.Fatalf("start returned %v, want both codes", start)
	}

	// Polling before approval must say "keep waiting", not "you failed".
	if code, _ := postJSON(t, srv, "/v1/auth/device/poll", map[string]string{"deviceCode": deviceCode}, nil); code != http.StatusPreconditionRequired {
		t.Fatalf("poll before approval = %d, want 428", code)
	}

	session := login(t, srv, "kid", "pw")
	if code, body := postJSON(t, srv, "/v1/auth/device/approve", map[string]string{"userCode": userCode}, session); code != http.StatusOK {
		t.Fatalf("approve = %d, want 200 (%v)", code, body)
	}

	code, poll := postJSON(t, srv, "/v1/auth/device/poll", map[string]string{"deviceCode": deviceCode}, nil)
	if code != http.StatusOK {
		t.Fatalf("poll after approval = %d, want 200 (%v)", code, poll)
	}
	token, _ := poll["token"].(string)
	if token == "" {
		t.Fatal("poll returned no token")
	}

	// The token must actually authenticate a real request.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("device token on /v1/auth/me = %d, want 200", res.StatusCode)
	}
	var me map[string]any
	_ = json.NewDecoder(res.Body).Decode(&me)
	if me["id"] != "u-kid" {
		t.Errorf("device authenticated as %v, want u-kid", me["id"])
	}
	// ⚠ The escalation guard, end to end: a member's TV must not come back as admin.
	if me["role"] == "admin" {
		t.Fatalf("device escalated to admin: %v", me)
	}
}

// A paired TV's role is capped at member (maintainer decision on #1659, 2026-09-28): an admin who
// pairs a TV gets a TV that acts as them for their lists, but not one that can administer the
// house. The admin's own session reaching the same routes proves the 403s come from the cap, not
// from a route the harness can't serve.
func TestAdminPairedDeviceIsCappedAtMember(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	srv := h.Server
	seedGridChannel(t, h.Store, "ch-news", 1)
	seedGridChannel(t, h.Store, "ch-films", 2)
	boss := login(t, srv, "boss", "pw")
	tv := pairDevice(t, srv, boss)

	send := func(method, path string, session *http.Cookie, bearer string) int {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
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
		_ = res.Body.Close()
		return res.StatusCode
	}
	for _, call := range [][2]string{
		{http.MethodGet, "/v1/users"},
		{http.MethodGet, "/v1/settings"},
		{http.MethodGet, "/v1/users/u-kid/sessions"},
		{http.MethodPatch, "/v1/users/u-kid"},
	} {
		if code := send(call[0], call[1], nil, tv); code != http.StatusForbidden {
			t.Errorf("admin-paired TV %s %s = %d, want 403", call[0], call[1], code)
		}
	}
	if code := send(http.MethodGet, "/v1/users", boss, ""); code != http.StatusOK {
		t.Fatalf("the admin's own session GET /v1/users = %d, want 200", code)
	}

	// The TV is still the admin as a person: /me names them, with the member role it actually has.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tv)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var me map[string]any
	_ = json.NewDecoder(res.Body).Decode(&me)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || me["id"] != "u-boss" || me["role"] != "member" {
		t.Fatalf("admin-paired TV /v1/auth/me = %d %v, want u-boss as member", res.StatusCode, me)
	}

	// N3 holds: what the TV stars and tunes lands in the admin's own lists.
	if code, got := myChannelsCall(t, srv, http.MethodPut, "/v1/me/favourites/ch-news", nil, tv); code != http.StatusOK ||
		got.String() != "favourites=[ch-news] recent=[]" {
		t.Fatalf("admin-paired TV stars ch-news = %d %s", code, got)
	}
	if code, got := myChannelsCall(t, srv, http.MethodPut, "/v1/me/recent-channels/ch-films", nil, tv); code != http.StatusOK ||
		got.String() != "favourites=[ch-news] recent=[ch-films]" {
		t.Fatalf("admin-paired TV tunes ch-films = %d %s", code, got)
	}
	if code, got := myChannelsCall(t, srv, http.MethodGet, "/v1/me/channels", boss, ""); code != http.StatusOK ||
		got.String() != "favourites=[ch-news] recent=[ch-films]" {
		t.Fatalf("the admin's session after the TV's writes = %d %s, want the TV's star and tune", code, got)
	}
}

// Approving requires a session — an anonymous caller must not be able to approve its own pairing.
func TestDeviceApproveRejectsAnonymous(t *testing.T) {
	t.Parallel()
	srv := newDeviceHarness(t, nil).Server
	_, start := postJSON(t, srv, "/v1/auth/device/start", map[string]string{"deviceName": "Shield"}, nil)
	userCode, _ := start["userCode"].(string)

	code, _ := postJSON(t, srv, "/v1/auth/device/approve", map[string]string{"userCode": userCode}, nil)
	if code == http.StatusOK {
		t.Fatal("an anonymous caller approved its own pairing")
	}
	if code != http.StatusUnauthorized && code != http.StatusForbidden {
		t.Errorf("approve without a session = %d, want 401/403", code)
	}
}

// A wrong code must not be distinguishable from an expired one, and must not grant anything.
func TestDeviceApproveRejectsUnknownCode(t *testing.T) {
	t.Parallel()
	srv := newDeviceHarness(t, nil).Server
	session := login(t, srv, "kid", "pw")
	code, _ := postJSON(t, srv, "/v1/auth/device/approve", map[string]string{"userCode": "BCDF-GHJK"}, session)
	if code != http.StatusNotFound {
		t.Errorf("approve with an unknown code = %d, want 404", code)
	}
}

// The guess-guard: repeated wrong codes must be throttled, or a short code is brute-forceable.
func TestDeviceApproveIsRateLimited(t *testing.T) {
	t.Parallel()
	// Two attempts then throttle, so the test does not depend on production's numbers.
	srv := newDeviceHarness(t, auth.NewRateLimiter(0.01, 2)).Server
	session := login(t, srv, "kid", "pw")

	seen := map[int]int{}
	for i := 0; i < 5; i++ {
		code, _ := postJSON(t, srv, "/v1/auth/device/approve", map[string]string{"userCode": "BCDF-GHJK"}, session)
		seen[code]++
	}
	if seen[http.StatusTooManyRequests] == 0 {
		t.Fatalf("no attempt was throttled; statuses = %v", seen)
	}
}

// A device is listed for its owner and revoking it kills the token immediately.
func TestDeviceListAndRevoke(t *testing.T) {
	t.Parallel()
	srv := newDeviceHarness(t, nil).Server
	_, start := postJSON(t, srv, "/v1/auth/device/start", map[string]string{"deviceName": "Shield"}, nil)
	deviceCode, _ := start["deviceCode"].(string)
	userCode, _ := start["userCode"].(string)
	session := login(t, srv, "kid", "pw")
	postJSON(t, srv, "/v1/auth/device/approve", map[string]string{"userCode": userCode}, session)
	_, poll := postJSON(t, srv, "/v1/auth/device/poll", map[string]string{"deviceCode": deviceCode}, nil)
	token, _ := poll["token"].(string)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/devices", nil)
	req.AddCookie(session)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Devices []struct {
			ID         string `json:"id"`
			DeviceName string `json:"deviceName"`
		} `json:"devices"`
	}
	_ = json.NewDecoder(res.Body).Decode(&list)
	_ = res.Body.Close()
	if len(list.Devices) != 1 || list.Devices[0].DeviceName != "Shield" {
		t.Fatalf("device list = %+v, want one Shield", list.Devices)
	}
	// The list must never hand back a usable credential.
	if list.Devices[0].ID == token {
		t.Fatal("the device list returned the live token as its id")
	}

	del, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/auth/devices/"+list.Devices[0].ID, nil)
	del.AddCookie(session)
	del.Header.Set("X-Loomarr-Csrf", "1")
	dres, err := srv.Client().Do(del)
	if err != nil {
		t.Fatal(err)
	}
	_ = dres.Body.Close()
	if dres.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke = %d, want 204", dres.StatusCode)
	}

	// The revoked token must stop working at once.
	me, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/me", nil)
	me.Header.Set("Authorization", "Bearer "+token)
	mres, err := srv.Client().Do(me)
	if err != nil {
		t.Fatal(err)
	}
	_ = mres.Body.Close()
	if mres.StatusCode == http.StatusOK {
		t.Fatal("a revoked device token still authenticates")
	}
}

// One user must not be able to revoke another's device.
func TestDeviceRevokeIsScopedToOwner(t *testing.T) {
	t.Parallel()
	srv := newDeviceHarness(t, nil).Server
	_, start := postJSON(t, srv, "/v1/auth/device/start", map[string]string{"deviceName": "Shield"}, nil)
	deviceCode, _ := start["deviceCode"].(string)
	userCode, _ := start["userCode"].(string)
	kid := login(t, srv, "kid", "pw")
	postJSON(t, srv, "/v1/auth/device/approve", map[string]string{"userCode": userCode}, kid)
	_, poll := postJSON(t, srv, "/v1/auth/device/poll", map[string]string{"deviceCode": deviceCode}, nil)
	token, _ := poll["token"].(string)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/devices", nil)
	req.AddCookie(kid)
	res, _ := srv.Client().Do(req)
	var list struct {
		Devices []struct {
			ID string `json:"id"`
		} `json:"devices"`
	}
	_ = json.NewDecoder(res.Body).Decode(&list)
	_ = res.Body.Close()

	boss := login(t, srv, "boss", "pw")
	del, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/auth/devices/"+list.Devices[0].ID, nil)
	del.AddCookie(boss)
	del.Header.Set("X-Loomarr-Csrf", "1")
	dres, _ := srv.Client().Do(del)
	_ = dres.Body.Close()
	if dres.StatusCode == http.StatusNoContent {
		t.Fatal("another user revoked a device they do not own")
	}

	me, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/me", nil)
	me.Header.Set("Authorization", "Bearer "+token)
	mres, _ := srv.Client().Do(me)
	_ = mres.Body.Close()
	if mres.StatusCode != http.StatusOK {
		t.Error("the owner's device stopped working after a foreign revoke attempt")
	}
}

// A paired client can deliberately revoke only the credential authenticating this request. This
// is the native client's Disconnect action: server state dies before SecureStore is cleared.
func TestDeviceCanRevokeItself(t *testing.T) {
	t.Parallel()
	srv := newDeviceHarness(t, nil).Server
	_, start := postJSON(t, srv, "/v1/auth/device/start", map[string]string{"deviceName": "Shield"}, nil)
	deviceCode, _ := start["deviceCode"].(string)
	userCode, _ := start["userCode"].(string)
	session := login(t, srv, "kid", "pw")
	postJSON(t, srv, "/v1/auth/device/approve", map[string]string{"userCode": userCode}, session)
	_, poll := postJSON(t, srv, "/v1/auth/device/poll", map[string]string{"deviceCode": deviceCode}, nil)
	token, _ := poll["token"].(string)

	disconnect, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/auth/device", nil)
	disconnect.Header.Set("Authorization", "Bearer "+token)
	res, err := srv.Client().Do(disconnect)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("self revoke = %d, want 204", res.StatusCode)
	}

	me, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/me", nil)
	me.Header.Set("Authorization", "Bearer "+token)
	meRes, err := srv.Client().Do(me)
	if err != nil {
		t.Fatal(err)
	}
	_ = meRes.Body.Close()
	if meRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("self-revoked token = %d, want 401", meRes.StatusCode)
	}
}

// A browser session is a person, not a current paired device. It must use the existing device list
// and id-scoped revoke route instead of turning "current device" into an ambiguous credential. The
// break-glass API token has no paired-device identity either and must fail the same way.
func TestDeviceSelfRevokeRejectsHumanSession(t *testing.T) {
	t.Parallel()
	srv := newDeviceHarness(t, nil).Server
	_, start := postJSON(t, srv, "/v1/auth/device/start", map[string]string{"deviceName": "Shield"}, nil)
	deviceCode, _ := start["deviceCode"].(string)
	userCode, _ := start["userCode"].(string)
	session := login(t, srv, "kid", "pw")
	postJSON(t, srv, "/v1/auth/device/approve", map[string]string{"userCode": userCode}, session)
	_, poll := postJSON(t, srv, "/v1/auth/device/poll", map[string]string{"deviceCode": deviceCode}, nil)
	token, _ := poll["token"].(string)

	disconnect, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/auth/device", nil)
	disconnect.AddCookie(session)
	disconnect.Header.Set("X-Loomarr-Csrf", "1")
	res, err := srv.Client().Do(disconnect)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("human self revoke = %d, want 401", res.StatusCode)
	}

	breakGlass, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/auth/device", nil)
	breakGlass.Header.Set("Authorization", "Bearer "+adminToken)
	breakGlassRes, err := srv.Client().Do(breakGlass)
	if err != nil {
		t.Fatal(err)
	}
	_ = breakGlassRes.Body.Close()
	if breakGlassRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("API token self revoke = %d, want 401", breakGlassRes.StatusCode)
	}

	me, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/me", nil)
	me.Header.Set("Authorization", "Bearer "+token)
	meRes, err := srv.Client().Do(me)
	if err != nil {
		t.Fatal(err)
	}
	_ = meRes.Body.Close()
	if meRes.StatusCode != http.StatusOK {
		t.Fatalf("device after refused human self revoke = %d, want 200", meRes.StatusCode)
	}
}
