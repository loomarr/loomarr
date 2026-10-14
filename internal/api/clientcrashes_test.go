package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/events"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// newCrashHarness is the real session and device-pairing stack with crash reports mounted (#1816).
func newCrashHarness(t *testing.T) *apiHarness {
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
			Events:        events.NewBus(),
			Store:         defaults.Store,
			Auth:          api.NewSessionAuthorizerCurrent(mgr, devices, func(context.Context) (string, error) { return "break-glass-token", nil }),
			Log:           defaults.Log,
			Login:         auth.NewLoginService(lib, defaults.Store, mgr, nil, time.Now),
			Sessions:      mgr,
			Devices:       devices,
			CookieSecure:  "false",
			ClientCrashes: diagnostics.NewClientCrashes(defaults.Store, time.Now),
		})
	})
}

// crashCall sends one request as a session (cookie), a bearer (paired device or API token), or
// nobody, and returns the status and raw body.
func crashCall(t *testing.T, srv *httptest.Server, method, path, body string, session *http.Cookie, bearer string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
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
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(raw)
}

func crashBody(t *testing.T, mutate func(map[string]any)) string {
	t.Helper()
	body := map[string]any{
		"kind": "js-fatal", "platform": "ios", "appVersion": "1.4.0", "appBuild": "212",
		"osVersion": "26.1", "deviceModel": "iPhone17,3", "occurredAt": "2026-10-13T08:00:00Z",
		"message": "TypeError: undefined is not an object", "stack": "at render (App.tsx:12:3)",
		"breadcrumbs": []map[string]any{{"at": "2026-10-13T07:59:58Z", "category": "navigation", "message": "/watch/7"}},
	}
	if mutate != nil {
		mutate(body)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// A paired phone reports a crash; the server records whose phone it was from the token, scrubs
// what must never be kept, and an admin reads it back.
func TestClientCrashFromPairedDeviceIsScrubbedAndReadableByAdmin(t *testing.T) {
	t.Parallel()
	srv := newCrashHarness(t).Server
	token := pairDevice(t, srv, login(t, srv, "kid", "pw"))

	code, raw := crashCall(t, srv, http.MethodPost, "/v1/diagnostics/client-crashes", crashBody(t, func(b map[string]any) {
		b["kind"] = "native"
		b["message"] = "EXC_BAD_ACCESS on https://loomarr.example/v1/live/7.m3u8?token=s3cret#t=2"
		b["stack"] = "Authorization: Bearer s3cret\nCookie: loomarr_session=s3cret; theme=s3cret"
		b["breadcrumbs"] = []map[string]any{{"at": "2026-10-13T07:59:58Z", "category": "http",
			"message": "GET /v1/live/7.m3u8?sig=s3cret 200"}}
		b["nativePayload"] = map[string]any{"crashes": []any{map[string]any{"url": "https://loomarr.example/p?token=s3cret"}},
			"token": "s3cret"}
	}), nil, token)
	if code != http.StatusAccepted {
		t.Fatalf("report = %d (%s), want 202", code, raw)
	}
	var accepted struct{ ID string }
	_ = json.Unmarshal([]byte(raw), &accepted)
	if accepted.ID == "" {
		t.Fatalf("report returned no id: %s", raw)
	}

	admin := login(t, srv, "boss", "pw")
	code, raw = crashCall(t, srv, http.MethodGet, "/v1/diagnostics/client-crashes", "", admin, "")
	if code != http.StatusOK {
		t.Fatalf("admin list = %d (%s)", code, raw)
	}
	var page diagnostics.ClientCrashPage
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != accepted.ID {
		t.Fatalf("list = %s, want the one report", raw)
	}
	item := page.Items[0]
	if item.UserID != "u-kid" || item.DeviceID == "" || item.DeviceName != "Living room TV" {
		t.Fatalf("reporter = user %q device %q name %q; want the paired device's owner, id and name",
			item.UserID, item.DeviceID, item.DeviceName)
	}
	if strings.Contains(raw, `"stack"`) || strings.Contains(raw, "nativePayload") {
		t.Fatalf("the list carries heavy fields: %s", raw)
	}

	code, raw = crashCall(t, srv, http.MethodGet, "/v1/diagnostics/client-crashes/"+accepted.ID, "", admin, "")
	if code != http.StatusOK {
		t.Fatalf("admin get = %d (%s)", code, raw)
	}
	if strings.Contains(raw, "s3cret") || strings.Contains(raw, token) {
		t.Fatalf("stored report carries a secret: %s", raw)
	}
	var report diagnostics.ClientCrashReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatal(err)
	}
	if report.Message != "EXC_BAD_ACCESS on https://loomarr.example/v1/live/7.m3u8" ||
		report.Breadcrumbs[0].Message != "GET /v1/live/7.m3u8 200" || len(report.NativePayload) == 0 {
		t.Fatalf("report = %+v, want URLs cut to their path and the native payload kept", report)
	}

	if code, _ := crashCall(t, srv, http.MethodGet, "/v1/diagnostics/client-crashes/crash_missing", "", admin, ""); code != http.StatusNotFound {
		t.Fatalf("missing report = %d, want 404", code)
	}
}

// Writing is any signed-in caller; reading is admin only (design §19 negatives).
func TestClientCrashAuthorization(t *testing.T) {
	t.Parallel()
	srv := newCrashHarness(t).Server
	member := login(t, srv, "kid", "pw")

	if code, raw := crashCall(t, srv, http.MethodPost, "/v1/diagnostics/client-crashes", crashBody(t, nil), nil, ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous report = %d (%s), want 401", code, raw)
	}
	if code, raw := crashCall(t, srv, http.MethodPost, "/v1/diagnostics/client-crashes", crashBody(t, nil), member, ""); code != http.StatusAccepted {
		t.Fatalf("member session report = %d (%s), want 202", code, raw)
	}
	if code, raw := crashCall(t, srv, http.MethodPost, "/v1/diagnostics/client-crashes", crashBody(t, nil), nil, "break-glass-token"); code != http.StatusAccepted {
		t.Fatalf("API-token report = %d (%s), want 202", code, raw)
	}
	for _, path := range []string{"/v1/diagnostics/client-crashes", "/v1/diagnostics/client-crashes/crash_x"} {
		if code, _ := crashCall(t, srv, http.MethodGet, path, "", member, ""); code != http.StatusForbidden {
			t.Fatalf("member GET %s = %d, want 403", path, code)
		}
		if code, _ := crashCall(t, srv, http.MethodGet, path, "", nil, pairDevice(t, srv, member)); code != http.StatusForbidden {
			t.Fatalf("member's paired device GET %s = %d, want 403", path, code)
		}
		if code, _ := crashCall(t, srv, http.MethodGet, path, "", nil, ""); code != http.StatusUnauthorized {
			t.Fatalf("anonymous GET %s = %d, want 401", path, code)
		}
	}
	code, raw := crashCall(t, srv, http.MethodGet, "/v1/diagnostics/client-crashes", "", nil, "break-glass-token")
	if code != http.StatusOK || strings.Count(raw, `"id":`) != 2 {
		t.Fatalf("API-token list = %d (%s), want both reports", code, raw)
	}
}

func TestClientCrashLimits(t *testing.T) {
	t.Parallel()
	srv := newCrashHarness(t).Server
	token := pairDevice(t, srv, login(t, srv, "kid", "pw"))
	post := func(body string) (int, string) {
		return crashCall(t, srv, http.MethodPost, "/v1/diagnostics/client-crashes", body, nil, token)
	}

	for name, mutate := range map[string]func(map[string]any){
		"kind":     func(b map[string]any) { b["kind"] = "panic" },
		"platform": func(b map[string]any) { b["platform"] = "windows" },
		"category": func(b map[string]any) {
			b["breadcrumbs"] = []map[string]any{{"at": "2026-10-13T07:59:58Z", "category": "console", "message": "x"}}
		},
		"message":    func(b map[string]any) { b["message"] = strings.Repeat("m", 2049) },
		"occurredAt": func(b map[string]any) { b["occurredAt"] = "yesterday" },
	} {
		if code, raw := post(crashBody(t, mutate)); code != http.StatusBadRequest {
			t.Fatalf("bad %s = %d (%s), want 400", name, code, raw)
		}
	}
	// The body cannot name who sent it, and a wrong JSON type never reaches the module: both are
	// the framework's decode refusal.
	if code, raw := post(crashBody(t, func(b map[string]any) { b["userId"] = "u-boss" })); code != http.StatusUnprocessableEntity {
		t.Fatalf("self-attributed report = %d (%s), want 422", code, raw)
	}
	if code, raw := post(crashBody(t, func(b map[string]any) { b["kind"] = 5 })); code != http.StatusUnprocessableEntity {
		t.Fatalf("numeric kind = %d (%s), want 422", code, raw)
	}
	oversized := crashBody(t, func(b map[string]any) { b["stack"] = strings.Repeat("s", 130<<10) })
	if code, raw := post(oversized); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body = %d (%.200s), want 413", code, raw)
	}

	// The rejections above stored nothing and so charged nothing; the burst is still whole.
	accepted := 0
	for range 30 {
		code, raw := post(crashBody(t, nil))
		if code == http.StatusTooManyRequests {
			break
		}
		if code != http.StatusAccepted {
			t.Fatalf("report = %d (%s)", code, raw)
		}
		accepted++
	}
	if accepted == 0 || accepted >= 30 {
		t.Fatalf("accepted %d reports before 429, want a bounded burst", accepted)
	}
}
