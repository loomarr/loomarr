// Package playoutcertfixture provides a neutral HTTP target for public
// playout-certification tests. It has no dependency on the runner package.
package playoutcertfixture

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

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

type Fixture struct {
	Server                                    *httptest.Server
	Admin, Device                             string
	Version                                   string
	Revision                                  string
	AllowOverload, InterruptHeld              bool
	StallHeldAfterOverload                    bool
	HeldBufferedBytes                         int
	BoundaryEarlyBurstDelay                   time.Duration
	RawOpened                                 chan struct{}
	FailMetricsAfterStart                     bool
	FailOneMetricsAfterStart                  bool
	RetainSessions                            bool
	MaxConcurrentRaw                          int
	HeldProgressAfterAdmission                int
	ResponsePadding                           map[string]string
	MintRelativeURL                           string
	PreparedMissChannels                      map[string]bool
	ZeroCostChannels                          map[string]bool
	ContinuousRawChannels                     map[string]bool
	mu                                        sync.Mutex
	activeRaw, starts                         int
	sessions                                  map[string]*session
	interrupt, continueHeld, overloadRejected chan struct{}
	capacityRejected                          chan struct{}
	interruptOnce, continueOnce, overloadOnce sync.Once
	capacityOnce                              sync.Once
	backlogReads                              int
	residualAfterRecovery                     string
}

type session struct {
	viewers      int
	graceEnds    time.Time
	faulted      chan struct{}
	faultOnce    sync.Once
	continued    chan struct{}
	continueOnce sync.Once
}

func New(t testing.TB, channels int) *Fixture {
	t.Helper()
	f := &Fixture{Admin: "admin-super-secret", Device: "device-super-secret", Version: "fixture", Revision: "0123456789abcdef0123456789abcdef01234567", sessions: map[string]*session{}, interrupt: make(chan struct{}), continueHeld: make(chan struct{}), overloadRejected: make(chan struct{}), capacityRejected: make(chan struct{})}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/system/version", f.version)
	mux.HandleFunc("/v1/playout/sessions", f.sessionsHandler)
	mux.HandleFunc("/v1/playout/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"running": true, "gpu": map[string]any{"name": "fixture"}, "channels": []any{}, "prepared": map[string]any{"readyChannels": channels, "channels": channels}})
	})
	mux.HandleFunc("/v1/diagnostics/processes", f.processes)
	mux.HandleFunc("/metrics", f.metrics)
	mux.HandleFunc("/v1/channels/", f.mint)
	mux.HandleFunc("/v1/playout/hls/", f.hls)
	mux.HandleFunc("/v1/playout/stream/", f.stream)
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Server.Close)
	return f
}

func (f *Fixture) version(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+f.Admin {
		http.Error(w, "no", http.StatusUnauthorized)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"version": f.Version, "commit": f.Revision, "ready": true})
	f.writePadding(w, r.URL.Path)
}

func (f *Fixture) sessionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+f.Admin {
		http.Error(w, "no", http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.expireLocked(time.Now())
	active, viewers, grace := len(f.sessions), 0, 0
	items := make([]map[string]any, 0, active)
	for id, item := range f.sessions {
		if item.viewers > 0 {
			viewers++
		} else {
			grace++
		}
		items = append(items, map[string]any{"channelId": id, "target": "full", "viewers": item.viewers})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"running": true, "capacity": 4, "active": active, "viewerActiveSessions": viewers, "graceIdleSessions": grace, "transcodeCost": min(f.transcodeCostLocked(), 4), "sessions": items})
}

func (f *Fixture) transcodeCostLocked() int {
	cost := 0
	for id := range f.sessions {
		if !f.ZeroCostChannels[id] {
			cost++
		}
	}
	return cost
}

// SetZeroCost changes one Channel's observed admission cost under the same
// lock as resource snapshots, for tests of cost changes during held playback.
func (f *Fixture) SetZeroCost(channelID string, zero bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ZeroCostChannels == nil {
		f.ZeroCostChannels = map[string]bool{}
	}
	f.ZeroCostChannels[channelID] = zero
}

func (f *Fixture) processes(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	active := f.activeRaw
	f.mu.Unlock()
	items := make([]map[string]any, active)
	for i := range items {
		items[i] = map[string]any{"executable": "ffmpeg", "status": "running"}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
}

func (f *Fixture) metrics(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	starts := f.starts
	fail := f.FailMetricsAfterStart && starts > 0
	if f.FailOneMetricsAfterStart && starts > 0 {
		fail = true
		f.FailOneMetricsAfterStart = false
	}
	f.mu.Unlock()
	if fail {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = fmt.Fprintf(w, "process_resident_memory_bytes 104857600\nprocess_cpu_seconds_total 2\nprocess_open_fds 12\ngo_goroutines 18\nloomarr_http_requests_in_flight 1\nloomarr_playout_sessions_active 0\nloomarr_playout_session_starts_total{result=\"success\"} %d\n", starts)
	f.writePadding(w, "/metrics")
}

func (f *Fixture) mint(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/play-url") {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+f.Admin {
		http.Error(w, "no", http.StatusUnauthorized)
		return
	}
	id := strings.Split(r.URL.Path, "/")[3]
	f.mu.Lock()
	relativeURL := f.MintRelativeURL
	f.mu.Unlock()
	if relativeURL == "" {
		relativeURL = "/v1/playout/hls/" + id + "/master.m3u8?exp=1&sig=signed-secret"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"relativeUrl": relativeURL})
	f.writePadding(w, r.URL.Path)
}

func (f *Fixture) hls(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("sig") != "signed-secret" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/master.m3u8") {
		id := strings.Split(r.URL.Path, "/")[4]
		if r.URL.Query().Get("mode") == "prepared" && f.PreparedMissChannels[id] {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(w, "#EXTM3U\n#EXTINF:2,\nsegment.m4s?exp=1&sig=signed-secret\n")
		f.writePadding(w, r.URL.Path)
		return
	}
	_, _ = w.Write([]byte("fixture-media-body"))
	f.writePadding(w, r.URL.Path)
}

func (f *Fixture) writePadding(w http.ResponseWriter, path string) {
	f.mu.Lock()
	padding := f.ResponsePadding[path]
	f.mu.Unlock()
	_, _ = io.WriteString(w, padding)
}

func (f *Fixture) stream(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("token") != f.Device {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/playout/stream/")
	f.mu.Lock()
	f.expireLocked(time.Now())
	item := f.sessions[id]
	if item != nil && item.viewers == 0 && sessionFaulted(item) {
		item = newSession()
		f.sessions[id] = item
		f.starts++
		if f.residualAfterRecovery == id {
			f.RetainSessions = true
		}
	}
	if item == nil && !f.ZeroCostChannels[id] && f.transcodeCostLocked() >= 4 && !f.AllowOverload {
		f.mu.Unlock()
		f.capacityOnce.Do(func() { close(f.capacityRejected) })
		if f.InterruptHeld {
			f.interruptOnce.Do(func() { close(f.interrupt) })
		} else if f.StallHeldAfterOverload {
			// Leave existing viewers connected but stop producing media.
			f.overloadOnce.Do(func() { close(f.overloadRejected) })
		} else {
			f.continueOnce.Do(func() { close(f.continueHeld) })
		}
		http.Error(w, "capacity", http.StatusServiceUnavailable)
		return
	}
	if item == nil && !f.ZeroCostChannels[id] && f.transcodeCostLocked() >= 4 && f.AllowOverload {
		f.continueOnce.Do(func() { close(f.continueHeld) })
	}
	if item == nil {
		item = newSession()
		f.sessions[id] = item
		f.starts++
	}
	if f.ContinuousRawChannels[id] {
		item.continueOnce.Do(func() { close(item.continued) })
	}
	f.activeRaw++
	item.viewers++
	if f.activeRaw > f.MaxConcurrentRaw {
		f.MaxConcurrentRaw = f.activeRaw
	}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.activeRaw--
		item.viewers--
		if item.viewers == 0 {
			item.graceEnds = time.Now().Add(20 * time.Millisecond)
		}
	}()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(make([]byte, 188))
	if f.HeldBufferedBytes > 0 {
		_, _ = w.Write(make([]byte, f.HeldBufferedBytes))
	}
	if flush, ok := w.(http.Flusher); ok {
		flush.Flush()
	}
	if f.RawOpened != nil {
		select {
		case f.RawOpened <- struct{}{}:
		default:
		}
	}
	if f.BoundaryEarlyBurstDelay > 0 {
		timer := time.NewTimer(f.BoundaryEarlyBurstDelay)
		select {
		case <-timer.C:
			_, _ = w.Write(make([]byte, 188))
			if flush, ok := w.(http.Flusher); ok {
				flush.Flush()
			}
		case <-r.Context().Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
		// Deliberately leave the HTTP body, reader, and decoder open after the
		// one post-boundary burst so certification can prove late continuity.
		<-r.Context().Done()
		return
	}
	select {
	case <-item.faulted:
		return
	default:
	}
	select {
	case <-r.Context().Done():
	case <-item.faulted:
	case <-item.continued:
		for range 1000 {
			_, _ = w.Write(make([]byte, 188))
			if flush, ok := w.(http.Flusher); ok {
				flush.Flush()
			}
			timer := time.NewTimer(10 * time.Millisecond)
			select {
			case <-timer.C:
			case <-item.faulted:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			case <-r.Context().Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			}
		}
		<-r.Context().Done()
	case <-f.interrupt:
	case <-f.continueHeld:
		// Span several client observation windows. This models continued live
		// production, rather than a single burst released by admission.
		for range 1000 {
			progress := make([]byte, 188)
			_, _ = w.Write(progress)
			if flush, ok := w.(http.Flusher); ok {
				flush.Flush()
			}
			f.mu.Lock()
			f.HeldProgressAfterAdmission += len(progress)
			f.mu.Unlock()
			timer := time.NewTimer(10 * time.Millisecond)
			select {
			case <-timer.C:
			case <-item.faulted:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			case <-r.Context().Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			}
		}
		select {
		case <-r.Context().Done():
		case <-f.interrupt:
		}
	}
}

// FailSession ends only the named public raw stream's current viewers. It is
// a test-target control seam; it does not expose a process identifier.
func (f *Fixture) FailSession(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	item := f.sessions[id]
	if item == nil {
		return false
	}
	item.faultOnce.Do(func() { close(item.faulted) })
	return true
}

// ContinueSession produces bounded post-event media for one named session.
func (f *Fixture) ContinueSession(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	item := f.sessions[id]
	if item == nil {
		return false
	}
	item.continueOnce.Do(func() { close(item.continued) })
	return true
}

// RetainReleasedSessions leaves the target's released-session evidence in
// place so a Run can exercise final cleanup disqualification.
func (f *Fixture) RetainReleasedSessions() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.RetainSessions = true
}

// RetainAfterRecovery retains only after the faulted stream is publicly
// re-admitted, so cleanup evidence follows a successful recovery attempt.
func (f *Fixture) RetainAfterRecovery(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.residualAfterRecovery = id
}

func newSession() *session {
	return &session{faulted: make(chan struct{}), continued: make(chan struct{})}
}

func sessionFaulted(item *session) bool {
	select {
	case <-item.faulted:
		return true
	default:
		return false
	}
}

func (f *Fixture) CapacityRejected() <-chan struct{} {
	return f.capacityRejected
}

// BackloggedHeldClient makes the raw-capacity cohort receive a finite backlog
// after overload is rejected. The bytes were already available at the transport
// boundary; no media is produced after the backlog drains.
func (f *Fixture) BackloggedHeldClient() *http.Client {
	base := f.Server.Client().Transport
	var mu sync.Mutex
	rawAttempts := 0
	return &http.Client{Transport: httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		isRaw := strings.HasPrefix(request.URL.Path, "/v1/playout/stream/")
		attempt := 0
		if isRaw {
			mu.Lock()
			rawAttempts++
			attempt = rawAttempts
			mu.Unlock()
		}
		response, err := base.RoundTrip(request)
		if err == nil && isRaw && attempt >= 9 && attempt <= 12 && response.StatusCode == http.StatusOK {
			response.Body = &finiteBacklogBody{
				source: response.Body, context: request.Context(), fixture: f,
				gate: f.overloadRejected, closed: make(chan struct{}), immediate: 188,
				remaining: 6 * 188, chunk: 188, interval: 15 * time.Millisecond,
			}
		}
		return response, err
	})}
}

// CancelOnRejectedOverloadClient cancels immediately after the target returns
// the expected raw-stream capacity rejection. It exercises cancellation before
// the held-viewer verification signal is delivered.
func (f *Fixture) CancelOnRejectedOverloadClient(cancel context.CancelFunc) *http.Client {
	base := f.Server.Client().Transport
	var once sync.Once
	return &http.Client{Transport: httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		response, err := base.RoundTrip(request)
		if err == nil && response.StatusCode == http.StatusServiceUnavailable && strings.HasPrefix(request.URL.Path, "/v1/playout/stream/") {
			once.Do(cancel)
		}
		return response, err
	})}
}

func (f *Fixture) BacklogReadsActive() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.backlogReads
}

type finiteBacklogBody struct {
	source               io.ReadCloser
	context              context.Context
	fixture              *Fixture
	gate, closed         chan struct{}
	closeOnce            sync.Once
	immediate, remaining int
	chunk                int
	interval             time.Duration
}

func (b *finiteBacklogBody) Read(buffer []byte) (int, error) {
	b.fixture.mu.Lock()
	b.fixture.backlogReads++
	b.fixture.mu.Unlock()
	defer func() {
		b.fixture.mu.Lock()
		b.fixture.backlogReads--
		b.fixture.mu.Unlock()
	}()
	if b.immediate > 0 {
		limit := min(len(buffer), b.immediate)
		n, err := b.source.Read(buffer[:limit])
		b.immediate -= n
		return n, err
	}
	select {
	case <-b.gate:
	case <-b.closed:
		return 0, io.ErrClosedPipe
	case <-b.context.Done():
		return 0, b.context.Err()
	}
	if b.remaining == 0 {
		select {
		case <-b.closed:
			return 0, io.ErrClosedPipe
		case <-b.context.Done():
			return 0, b.context.Err()
		}
	}
	timer := time.NewTimer(b.interval)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-b.closed:
		return 0, io.ErrClosedPipe
	case <-b.context.Done():
		return 0, b.context.Err()
	}
	limit := min(len(buffer), b.chunk, b.remaining)
	n, err := b.source.Read(buffer[:limit])
	b.remaining -= n
	return n, err
}

func (b *finiteBacklogBody) Close() error {
	b.closeOnce.Do(func() { close(b.closed) })
	return b.source.Close()
}

func (f *Fixture) expireLocked(now time.Time) {
	if f.RetainSessions {
		return
	}
	for id, item := range f.sessions {
		// A fault target marked for post-recovery retention must survive the
		// short warm grace long enough for its replacement admission. That
		// replacement is what activates final residual retention; preserving it
		// here does not retain unrelated released sessions.
		if id == f.residualAfterRecovery && sessionFaulted(item) {
			continue
		}
		if item.viewers == 0 && !item.graceEnds.IsZero() && !now.Before(item.graceEnds) {
			delete(f.sessions, id)
		}
	}
}
