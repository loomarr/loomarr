package scan

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/arrhook"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// webhookArr saves whatever connection it is sent.
func webhookArr(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/notification":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/notification/schema":
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"implementation": "Webhook", "fields": []any{}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/notification":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestArrHooks(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.st.RecordLibraryRootDiscovery(ctx, librarystore.LibraryRootDiscovery{
		Found: []librarystore.LibraryRootPath{{Authority: "sonarr", AuthorityPath: "/tv", RootPath: h.root}}, Complete: []string{"sonarr"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	arr := webhookArr(t)
	address := "http://loomarr.test"
	hooks := &ArrHooks{
		Scanner: NewScanner(h.im, time.Hour),
		Clients: map[string]arrhook.Client{
			"sonarr": {Conn: func() (string, string) { return arr.URL, "key" }},
			"radarr": {Conn: func() (string, string) { return "", "" }},
		},
		URL: func(app string) string {
			if address == "" {
				return ""
			}
			return address + "/v1/library/hooks/" + app
		},
		Secret: "secret-90000021",
	}

	address = ""
	_ = hooks.Register(ctx, "sonarr")
	if s := hooks.Status("sonarr"); s.State != HookNoAddress {
		t.Fatalf("with no address: %+v", s)
	}
	if err := hooks.Test(ctx, "sonarr"); !errors.Is(err, ErrHookNoAddress) {
		t.Fatalf("Test with no address = %v", err)
	}
	address = "http://loomarr.test"
	for _, app := range []string{"sonarr", "radarr"} {
		if err := hooks.Register(ctx, app); err != nil {
			t.Fatal(err)
		}
	}
	statuses := hooks.Statuses()
	if len(statuses) != 2 || statuses[0].App != "radarr" || statuses[0].State != HookNotConfigured ||
		statuses[1].State != HookRegistered || statuses[1].RegisteredAt.IsZero() {
		t.Fatalf("statuses = %+v", statuses)
	}

	if !hooks.Authorized(arrhook.Username, "secret-90000021") || hooks.Authorized(arrhook.Username, "guess") ||
		hooks.Authorized("someone", "secret-90000021") || (&ArrHooks{}).Authorized(arrhook.Username, "") {
		t.Fatal("Authorized accepts the wrong credentials or refuses the right ones")
	}

	body := `{"eventType":"Download","series":{"path":"/tv/Romeo"},"episodeFile":{"path":"/tv/Romeo/Season 01/Romeo S01E01.mkv"}}`
	if err := hooks.Receive(ctx, "sonarr", []byte(body)); err != nil {
		t.Fatal(err)
	}
	if s := hooks.Status("sonarr"); s.LastEvent != "Download" || s.LastEventAt.IsZero() || s.State != HookRegistered {
		t.Fatalf("after an event: %+v", s)
	}
	hooks.Scanner.mu.Lock()
	queued := hooks.Scanner.folders[h.root]
	hooks.Scanner.mu.Unlock()
	if _, ok := queued[h.root+"/Romeo/Season 01"]; !ok || len(queued) != 1 {
		t.Fatalf("queued = %v, want the episode's season folder", queued)
	}
	if err := hooks.Receive(ctx, "lidarr", []byte(body)); !errors.Is(err, ErrHookUnknownApp) {
		t.Fatalf("Receive from another app = %v", err)
	}
}

// An arr masks the stored password, so a secret that changed (a reinstall) cannot be seen: the first
// registration after start writes the connection anyway, and later ones write only on a change.
func TestArrHooksRewriteOnceAfterStart(t *testing.T) {
	h := newHarness(t)
	var puts int
	arr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/notification":
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"id": 90000031.0, "name": arrhook.Name, "fields": []any{
				map[string]any{"name": "url", "value": "http://loomarr.test/v1/library/hooks/sonarr"},
				map[string]any{"name": "method", "value": 1},
				map[string]any{"name": "username", "value": arrhook.Username},
				map[string]any{"name": "password", "value": "********"},
			}}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v3/notification/90000031":
			puts++
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(arr.Close)
	hooks := &ArrHooks{
		Scanner: NewScanner(h.im, time.Hour),
		Clients: map[string]arrhook.Client{"sonarr": {Conn: func() (string, string) { return arr.URL, "key" }}},
		URL:     func(app string) string { return "http://loomarr.test/v1/library/hooks/" + app },
		Secret:  "secret-90000032",
	}
	for range 3 {
		if err := hooks.Register(context.Background(), "sonarr"); err != nil {
			t.Fatal(err)
		}
	}
	if puts != 1 {
		t.Fatalf("writes = %d, want 1 (the first after start)", puts)
	}
}

// An arr that stops taking Loomarr's connection is reported once, when it starts failing.
func TestArrHooksReportDegradedOnce(t *testing.T) {
	h := newHarness(t)
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	t.Cleanup(refusing.Close)
	var degraded []string
	hooks := &ArrHooks{
		Scanner: NewScanner(h.im, time.Hour),
		Clients: map[string]arrhook.Client{"radarr": {Conn: func() (string, string) { return refusing.URL, "key" }}},
		URL:     func(app string) string { return "http://loomarr.test/v1/library/hooks/" + app },
		Secret:  "s",
		Degraded: func(_ context.Context, app string, err error) {
			if err == nil {
				t.Error("degraded without a reason")
			}
			degraded = append(degraded, app)
		},
	}
	for range 3 {
		if err := hooks.Register(context.Background(), "radarr"); err == nil {
			t.Fatal("Register against a refusing arr succeeded")
		}
	}
	if len(degraded) != 1 || degraded[0] != "radarr" {
		t.Fatalf("degraded = %v, want radarr once", degraded)
	}
	if s := hooks.Status("radarr"); s.State != HookFailed || s.Error == "" {
		t.Fatalf("status = %+v", s)
	}
}
