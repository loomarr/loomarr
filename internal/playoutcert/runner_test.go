package playoutcert

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestValidateConfigRejectsUnsafeOrNonCertifyingInputs(t *testing.T) {
	channels := fixtureChannels(100)
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "credential in origin", mutate: func(c *Config) { c.BaseURL += "?token=secret" }, want: "query"},
		{name: "too few channels", mutate: func(c *Config) { c.Channels = c.Channels[:99] }, want: "at least 100"},
		{name: "duplicate channel", mutate: func(c *Config) { c.Channels[99].ID = c.Channels[0].ID }, want: "duplicate"},
		{name: "remote without acknowledgement", mutate: func(c *Config) { c.BaseURL = "http://192.0.2.1:8080" }, want: "remote"},
		{name: "missing bearer", mutate: func(c *Config) { c.AdminBearer = "" }, want: "administrator"},
		{name: "missing device token", mutate: func(c *Config) { c.DeviceToken = "" }, want: "device"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{BaseURL: "http://127.0.0.1:8080", AdminBearer: "admin-secret", DeviceToken: "device-secret", Channels: append([]Channel(nil), channels...), Certify: true}
			tc.mutate(&cfg)
			if err := cfg.Validate(); err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("Validate() = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRunExercisesSignedPreparedSurfRawAndCleanupWithoutLeakingPrivateInputs(t *testing.T) {
	t.Parallel()
	fixture := newHTTPFixture(t, 100)
	validator := &recordingValidator{}
	cfg := Config{
		BaseURL: fixture.server.URL, AdminBearer: fixture.admin, DeviceToken: fixture.device,
		Channels: fixtureChannels(100), Certify: true, RemoteAcknowledged: true,
		Concurrency: 12, SurfRounds: 1, FanInViewers: 4, RequestTimeout: time.Second,
		CleanupTimeout: time.Second, CleanupPoll: time.Millisecond, RawCaptureBytes: 188,
		Validator: validator, Decoder: validator,
	}
	report, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Certified {
		t.Fatalf("report not certified: %+v", report.Failures)
	}
	if report.Target.ConfiguredChannels != 100 || report.Target.Capacity != 4 {
		t.Fatalf("target = %+v", report.Target)
	}
	if report.Resources[0].PreparedChannels != 100 || report.Resources[0].ReadyChannels != 100 {
		t.Fatalf("baseline prepared readiness = %+v", report.Resources[0])
	}
	for _, name := range []string{"mint", "configured", "surf", "prepared_fan_in", "fan_in", "prepared_raw", "raw_capacity", "capacity_recovery", "overload", "cleanup"} {
		phase, ok := report.Phase(name)
		if !ok || phase.Attempts == 0 || phase.Failures != 0 {
			t.Fatalf("phase %q = %+v, present=%t", name, phase, ok)
		}
	}
	if report.PhaseMust("configured").PreparedHits != 100 || report.PhaseMust("configured").P95MS < 0 {
		t.Fatalf("configured phase = %+v", report.PhaseMust("configured"))
	}
	if validator.calls < 5 {
		t.Fatalf("validator calls = %d, want raw capacity plus overload", validator.calls)
	}
	blob, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	unsafe := []string{fixture.admin, fixture.device, "signed-secret", "channel-private-", "Operator Library Title", fixture.server.URL}
	for _, value := range unsafe {
		if strings.Contains(string(blob), value) {
			t.Fatalf("report leaked %q: %s", value, blob)
		}
	}
	if fixture.maxConcurrentRaw < 4 {
		t.Fatalf("raw requests were not barrier-concurrent: peak=%d", fixture.maxConcurrentRaw)
	}
}

func TestNearestRankPercentilesKeepFailuresSeparate(t *testing.T) {
	got := summarize([]time.Duration{10 * time.Millisecond, 40 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}, 2)
	if got.Attempts != 6 || got.Successes != 4 || got.Failures != 2 || got.P50MS != 20 || got.P95MS != 40 || got.P99MS != 40 {
		t.Fatalf("summary = %+v", got)
	}
}

type recordingValidator struct {
	mu    sync.Mutex
	calls int
}

func (v *recordingValidator) Validate(_ context.Context, body []byte) (MediaShape, error) {
	v.mu.Lock()
	v.calls++
	v.mu.Unlock()
	if len(body) == 0 {
		return MediaShape{}, fmt.Errorf("empty body")
	}
	return MediaShape{VideoStreams: 1, AudioStreams: 1, VideoCodec: "h264", AudioCodec: "aac"}, nil
}

func (v *recordingValidator) FirstFrame(_ context.Context, input io.Reader, maxBytes int) ([]byte, error) {
	return io.ReadAll(io.LimitReader(input, int64(maxBytes)))
}

type httpFixture struct {
	server           *httptest.Server
	admin            string
	device           string
	mu               sync.Mutex
	activeRaw        int
	maxConcurrentRaw int
	sessions         map[string]*fixtureSession
	starts           int
}

type fixtureSession struct {
	viewers   int
	graceEnds time.Time
}

func newHTTPFixture(t *testing.T, channels int) *httpFixture {
	t.Helper()
	f := &httpFixture{admin: "admin-super-secret", device: "device-super-secret", sessions: map[string]*fixtureSession{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/system/version", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+f.admin {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "fixture", "commit": "0123456789abcdef", "ready": true})
	})
	mux.HandleFunc("/v1/playout/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+f.admin {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		f.expireGraceLocked(time.Now())
		active, viewers, grace := len(f.sessions), 0, 0
		sessions := make([]map[string]any, 0, active)
		for channelID, session := range f.sessions {
			if session.viewers > 0 {
				viewers++
			} else {
				grace++
			}
			sessions = append(sessions, map[string]any{"channelId": channelID, "target": "full", "viewers": session.viewers})
		}
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"running": true, "capacity": 4, "active": active, "viewerActiveSessions": viewers, "graceIdleSessions": grace, "transcodeCost": min(active, 4), "sessions": sessions})
	})
	mux.HandleFunc("/v1/playout/status", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"running": true, "gpu": map[string]any{"name": "fixture"}, "channels": []any{}, "prepared": map[string]any{"readyChannels": channels, "channels": channels}})
	})
	mux.HandleFunc("/v1/diagnostics/processes", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		active := f.activeRaw
		f.mu.Unlock()
		items := make([]map[string]any, active)
		for i := range items {
			items[i] = map[string]any{"id": fmt.Sprintf("run-%d", i), "purpose": "playout_channel", "executable": "ffmpeg", "status": "running"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		starts := f.starts
		f.mu.Unlock()
		_, _ = fmt.Fprintf(w, "process_resident_memory_bytes 104857600\nprocess_cpu_seconds_total 2\nprocess_open_fds 12\ngo_goroutines 18\nloomarr_http_requests_in_flight 1\nloomarr_playout_sessions_active 0\nloomarr_playout_session_starts_total{result=\"success\"} %d\n", starts)
	})
	mux.HandleFunc("/v1/channels/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/play-url") {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+f.admin {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		parts := strings.Split(r.URL.Path, "/")
		id := parts[3]
		_ = json.NewEncoder(w).Encode(map[string]any{"relativeUrl": "/v1/playout/hls/" + id + "/master.m3u8?exp=1&sig=signed-secret"})
	})
	mux.HandleFunc("/v1/playout/hls/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("sig") != "signed-secret" {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/master.m3u8") {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4?exp=1&sig=signed-secret\"\n#EXTINF:2,\nsegment-000001.m4s?exp=1&sig=signed-secret\n")
			return
		}
		_, _ = w.Write([]byte("fixture-media-body"))
	})
	mux.HandleFunc("/v1/playout/stream/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != f.device {
			http.NotFound(w, r)
			return
		}
		channelID := strings.TrimPrefix(r.URL.Path, "/v1/playout/stream/")
		f.mu.Lock()
		f.expireGraceLocked(time.Now())
		session := f.sessions[channelID]
		if session == nil && len(f.sessions) >= 4 {
			f.mu.Unlock()
			http.Error(w, "capacity", http.StatusServiceUnavailable)
			return
		}
		if session == nil {
			session = &fixtureSession{}
			f.sessions[channelID] = session
			f.starts++
		}
		f.activeRaw++
		session.viewers++
		if f.activeRaw > f.maxConcurrentRaw {
			f.maxConcurrentRaw = f.activeRaw
		}
		f.mu.Unlock()
		defer func() {
			f.mu.Lock()
			f.activeRaw--
			session.viewers--
			if session.viewers == 0 {
				session.graceEnds = time.Now().Add(20 * time.Millisecond)
			}
			f.mu.Unlock()
		}()
		w.Header().Set("Content-Type", "video/mp2t")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 188))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *httpFixture) expireGraceLocked(now time.Time) {
	for channelID, session := range f.sessions {
		if session.viewers == 0 && !session.graceEnds.IsZero() && !now.Before(session.graceEnds) {
			delete(f.sessions, channelID)
		}
	}
}

func fixtureChannels(n int) []Channel {
	out := make([]Channel, n)
	for i := range out {
		out[i] = Channel{ID: fmt.Sprintf("channel-private-%03d", i+1), Roles: []string{"prepared"}}
	}
	return out
}
