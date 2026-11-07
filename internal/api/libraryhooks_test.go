package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/library/scan"
)

type fakeHooks struct {
	received []string
	tested   []string
	testErr  error
}

func (f *fakeHooks) Authorized(user, password string) bool {
	return user == "loomarr" && password == "secret-90000011"
}

func (f *fakeHooks) Receive(_ context.Context, app string, body []byte) error {
	if !strings.Contains(string(body), "eventType") {
		return errors.New("not a webhook")
	}
	f.received = append(f.received, app)
	return nil
}

func (f *fakeHooks) Statuses() []scan.HookStatus {
	return []scan.HookStatus{
		{App: "radarr", State: scan.HookFailed, Error: "radarr refused"},
		{App: "sonarr", State: scan.HookRegistered, RegisteredAt: time.Unix(90000012, 0), LastEvent: "Download", LastEventAt: time.Unix(90000013, 0)},
	}
}

func (f *fakeHooks) Test(_ context.Context, app string) error {
	f.tested = append(f.tested, app)
	return f.testErr
}

// An arr's delivery needs the registered credentials and no session; anything else is refused
// before the scanner hears of it.
func TestLibraryHooks_DeliveryAndStatus(t *testing.T) {
	hooks := &fakeHooks{}
	h := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log,
			LibraryRoots: newScanLibrary(t, defaults.Store), LibraryHooks: hooks,
		})
	})
	post := func(path, user, password, body string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, h.Server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if user != "" {
			req.SetBasicAuth(user, password)
		}
		resp, err := h.Server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	event := `{"eventType":"Download"}`
	for _, tc := range []struct {
		name, path, user, password, body string
		want                             int
	}{
		{"no credentials", "/v1/library/hooks/sonarr", "", "", event, http.StatusUnauthorized},
		{"wrong secret", "/v1/library/hooks/sonarr", "loomarr", "guess", event, http.StatusUnauthorized},
		{"another app", "/v1/library/hooks/lidarr", "loomarr", "secret-90000011", event, http.StatusUnprocessableEntity},
		{"not a webhook", "/v1/library/hooks/sonarr", "loomarr", "secret-90000011", `{"hello":1}`, http.StatusBadRequest},
		{"delivered", "/v1/library/hooks/radarr", "loomarr", "secret-90000011", event, http.StatusAccepted},
	} {
		if got := post(tc.path, tc.user, tc.password, tc.body); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
	if len(hooks.received) != 1 || hooks.received[0] != "radarr" {
		t.Fatalf("received = %v, want one radarr event", hooks.received)
	}

	if got := h.Do(http.MethodPost, "/v1/library/hooks/sonarr/test", memberToken, "").StatusCode; got != http.StatusForbidden {
		t.Errorf("member test = %d, want 403", got)
	}
	if got := h.Do(http.MethodPost, "/v1/library/hooks/sonarr/test", adminToken, "").StatusCode; got != http.StatusNoContent {
		t.Errorf("admin test = %d, want 204", got)
	}
	hooks.testErr = scan.ErrHookNoAddress
	if got := h.Do(http.MethodPost, "/v1/library/hooks/sonarr/test", adminToken, "").StatusCode; got != http.StatusBadGateway {
		t.Errorf("failed test = %d, want 502", got)
	}

	resp := h.Do(http.MethodGet, "/v1/library/status", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Hooks []struct {
			App, State, Error, LastEvent string
			RegisteredAt, LastEventAt    *time.Time
		} `json:"hooks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Hooks) != 2 || body.Hooks[0].State != "failed" || body.Hooks[0].Error != "radarr refused" ||
		body.Hooks[1].LastEvent != "Download" || body.Hooks[1].LastEventAt == nil || body.Hooks[0].RegisteredAt != nil {
		t.Fatalf("hooks = %+v", body.Hooks)
	}
}
