package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

const playoutToken = "playout-token-abcdefghijklmnop"

// fakePlayoutSessions stands in for playout.Manager. The session lifecycle is tested in
// internal/playout; here the question is what the HTTP layer does with it.
type fakePlayoutSessions struct {
	mu       sync.Mutex
	attached []string
	err      error
	chunks   chan []byte
	detached int
	// V16 telemetry: what the handlers reported, and what Stats hands back.
	stats    []playout.SessionStat
	capacity int
	reported []reportedProgram
}

// reportedProgram is one ReportProgram call, so a test can assert the per-program encode path
// actually reports its telemetry rather than silently dropping it.
type reportedProgram struct {
	channelID string
	encoder   playout.Encoder
	progress  playout.Progress
}

func (f *fakePlayoutSessions) Stats(time.Time) []playout.SessionStat {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stats
}

func (f *fakePlayoutSessions) Capacity() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.capacity
}

func (f *fakePlayoutSessions) ReportProgram(channelID string, enc playout.Encoder, p playout.Progress) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reported = append(f.reported, reportedProgram{channelID: channelID, encoder: enc, progress: p})
}

func (f *fakePlayoutSessions) Attach(_ context.Context, channelID string) (<-chan []byte, func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, nil, f.err
	}
	f.attached = append(f.attached, channelID)
	if f.chunks == nil {
		f.chunks = make(chan []byte, 8)
	}
	return f.chunks, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.detached++
	}, nil
}

// reports returns the ReportProgram calls seen so far.
func (f *fakePlayoutSessions) reports() []reportedProgram {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]reportedProgram(nil), f.reported...)
}

func (f *fakePlayoutSessions) detachCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.detached
}

type playoutOpts struct {
	sessions   api.PlayoutSessions
	token      string
	publicURL  string
	backend    string
	noSecret   bool
	skipConfig bool
}

func newPlayoutServer(t *testing.T, o playoutOpts) (*httptest.Server, store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/playout.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if o.token == "" {
		o.token = playoutToken
	}
	if o.publicURL == "" {
		o.publicURL = "http://loomarr.local:8080"
	}
	if o.backend == "" {
		o.backend = "internal"
	}

	opts := api.Options{
		Store:           st,
		Auth:            api.NewTokenAuthorizer(adminToken),
		Log:             slog.New(slog.DiscardHandler),
		PlayoutSessions: o.sessions,
	}
	if !o.noSecret {
		opts.PlayoutSecret = func() string { return o.token }
	}
	if !o.skipConfig {
		cfg := map[string]string{
			"server.public_url": o.publicURL,
			"playout.backend":   o.backend,
		}
		opts.LiveConfig = func(k string) string { return cfg[k] }
	}

	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), opts))
	t.Cleanup(srv.Close)
	return srv, st
}

func getPlayout(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func seedChannel(t *testing.T, st store.Store, id, name string, number int, backend string) {
	t.Helper()
	ch := store.Channel{Channel: schedule.Channel{ID: id, Name: name, Number: number}}
	if backend != "" {
		ch.Policy.Playout = &schedule.PlayoutPolicy{Backend: backend}
	}
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

// --- Device auth (§11). These are the negative cases CLAUDE.md §19 requires. ---

// EVERY playout route must reject a missing or wrong token. A television is the client, so
// there is no session to fall back on — the token is the only auth these routes have.
func TestPlayout_RejectsMissingOrWrongToken(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{sessions: &fakePlayoutSessions{}})
	seedChannel(t, st, "ch1", "Channel One", 1, "internal")

	for _, path := range []string{
		"/playout/tuner.m3u",
		"/playout/stream/ch1",
		"/playout/playlist/ch1",
	} {
		for name, q := range map[string]string{
			"no token":    "",
			"empty token": "?token=",
			"wrong token": "?token=nope",
			// A prefix of the real token must not pass — it would if the comparison were
			// truncating or prefix-based.
			"token prefix": "?token=" + playoutToken[:10],
		} {
			resp := getPlayout(t, srv, path+q)
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s with %s: status %d, want 404", path, name, resp.StatusCode)
			}
		}
	}
}

// A wrong token gets 404, NOT 401/403. These URLs are pasted into a media server's config and
// leak into logs and screenshots; an enumerable "real channel, wrong password" tells an
// attacker where to aim.
func TestPlayout_WrongTokenIsIndistinguishableFromNoRoute(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{sessions: &fakePlayoutSessions{}})
	seedChannel(t, st, "ch1", "Channel One", 1, "internal")

	real := getPlayout(t, srv, "/playout/stream/ch1?token=wrong")
	fake := getPlayout(t, srv, "/playout/stream/does-not-exist?token=wrong")

	if real.StatusCode != fake.StatusCode {
		t.Errorf("a real channel with a bad token (%d) is distinguishable from a "+
			"nonexistent one (%d) — that is an enumeration oracle",
			real.StatusCode, fake.StatusCode)
	}
}

// No token configured ⇒ fail CLOSED. Serving streams unauthenticated because a secret failed
// to mint would silently remove the only auth these routes have.
func TestPlayout_NoConfiguredTokenRefusesEverything(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{sessions: &fakePlayoutSessions{}, noSecret: true})
	seedChannel(t, st, "ch1", "Channel One", 1, "internal")

	for _, path := range []string{"/playout/tuner.m3u", "/playout/stream/ch1", "/playout/playlist/ch1"} {
		// Even presenting an empty token — which would "match" an unset secret under a naive
		// equality check — must be refused.
		for _, q := range []string{"", "?token=", "?token=" + playoutToken} {
			if resp := getPlayout(t, srv, path+q); resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s%s: status %d with no token configured, want 404",
					path, q, resp.StatusCode)
			}
		}
	}
}

// The admin API token must NOT work on playout routes, and vice versa. §11: they are separate
// secrets with opposite authority — playout_token grants no API access, api_token is
// break-glass admin.
func TestPlayout_AdminTokenIsNotAPlayoutToken(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{sessions: &fakePlayoutSessions{}})
	seedChannel(t, st, "ch1", "Channel One", 1, "internal")

	if resp := getPlayout(t, srv, "/playout/stream/ch1?token="+adminToken); resp.StatusCode != http.StatusNotFound {
		t.Errorf("the admin token authorized a playout route: status %d", resp.StatusCode)
	}
	// And the playout token grants no PRIVILEGED API access. POST /v1/channels is admin-only
	// (requireAdmin); GET /v1/channels is deliberately NOT, so asserting against a read route
	// would prove only that the route is public — which an earlier version of this test did.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/channels",
		strings.NewReader(`{"name":"Sneaky","number":99}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+playoutToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		t.Errorf("the playout token created a channel (status %d) — §11 says it grants no "+
			"write of any kind", resp.StatusCode)
	}
}

// A playout path that matches no route must 404, not fall through to the SPA. Without the
// /playout/ prefix guard, ffmpeg would read index.html as a transport stream and report a
// corrupt stream naming neither the URL nor the typo.
func TestPlayout_UnknownPathDoesNotServeTheSPA(t *testing.T) {
	srv, _ := newPlayoutServer(t, playoutOpts{sessions: &fakePlayoutSessions{}})

	resp := getPlayout(t, srv, "/playout/not-a-route?token="+playoutToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(strings.ToLower(string(body)), "<!doctype html") {
		t.Error("an unknown playout path served the SPA; ffmpeg would read HTML as MPEG-TS")
	}
}

// --- The stream endpoint ---

// The response must look like a LIVE stream, not a file. A Content-Length promises an end that
// never comes, and advertising ranges invites a seek that is meaningless here.
func TestPlayoutStream_LooksLikeALiveStreamNotAFile(t *testing.T) {
	f := &fakePlayoutSessions{}
	srv, st := newPlayoutServer(t, playoutOpts{sessions: f})
	seedChannel(t, st, "ch1", "Channel One", 1, "internal")

	// A real request would never end, so drive it with a cancellable context and stop after
	// the headers arrive.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		srv.URL+"/playout/stream/ch1?token="+playoutToken, nil)

	// Feed one chunk so the handler writes something and we know it is streaming.
	f.chunks = make(chan []byte, 1)
	f.chunks <- []byte("ts-bytes")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "video/mp2t" {
		t.Errorf("Content-Type = %q, want video/mp2t", ct)
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		t.Errorf("Content-Length = %q — a live stream has no length", cl)
	}
	if ar := resp.Header.Get("Accept-Ranges"); ar != "none" {
		t.Errorf("Accept-Ranges = %q, want none (a seek is meaningless on a live stream)", ar)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	// The bytes must actually arrive — proving the handler flushes rather than buffering.
	buf := make([]byte, len("ts-bytes"))
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatalf("no stream bytes: %v", err)
	}
	if string(buf) != "ts-bytes" {
		t.Errorf("got %q, want the session's chunk", buf)
	}
}

// A viewer disconnecting MUST detach. Nothing else reports it — the tuner path never
// re-requests — and a leaked viewer keeps the channel encoding forever.
func TestPlayoutStream_ClientDisconnectDetaches(t *testing.T) {
	f := &fakePlayoutSessions{chunks: make(chan []byte, 1)}
	srv, st := newPlayoutServer(t, playoutOpts{sessions: f})
	seedChannel(t, st, "ch1", "Channel One", 1, "internal")

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		srv.URL+"/playout/stream/ch1?token="+playoutToken, nil)
	f.chunks <- []byte("x")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatalf("stream did not start: %v", err)
	}

	cancel() // the television goes away
	_ = resp.Body.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if f.detachCount() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("a disconnected viewer was never detached — the channel would encode forever")
}

// The session ending (encoder exited, channel stopped, viewer dropped for falling behind) must
// end the response rather than hanging the client.
func TestPlayoutStream_SessionEndEndsTheResponse(t *testing.T) {
	f := &fakePlayoutSessions{chunks: make(chan []byte, 1)}
	srv, st := newPlayoutServer(t, playoutOpts{sessions: f})
	seedChannel(t, st, "ch1", "Channel One", 1, "internal")

	f.chunks <- []byte("y")
	resp := getPlayout(t, srv, "/playout/stream/ch1?token="+playoutToken)
	buf := make([]byte, 1)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatalf("stream did not start: %v", err)
	}

	close(f.chunks) // the encoder died

	done := make(chan struct{})
	go func() { _, _ = io.ReadAll(resp.Body); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("the response did not end when the session ended — the client hangs")
	}
}

// At capacity must be a 503 with Retry-After, not a generic error. The operator's fix (raise
// the cap, lower the tier) is only discoverable if we say which wall was hit, and Retry-After
// is what makes a media server back off instead of hammering.
func TestPlayoutStream_AtCapacityIsActionable(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{
		sessions: &fakePlayoutSessions{err: playout.ErrAtCapacity},
	})
	seedChannel(t, st, "ch1", "Channel One", 1, "internal")

	resp := getPlayout(t, srv, "/playout/stream/ch1?token="+playoutToken)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("no Retry-After — a media server would retry immediately and hammer")
	}
	body, _ := io.ReadAll(resp.Body)
	// The message must name the fix, not just the failure.
	if !strings.Contains(string(body), "channel limit") {
		t.Errorf("the 503 body does not tell the operator how to fix it: %s", body)
	}
}

// Playout not running is a 501 with an explanation, not a 404 that looks like a wiring bug.
func TestPlayoutStream_NotRunningExplainsItself(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{sessions: nil})
	seedChannel(t, st, "ch1", "Channel One", 1, "internal")

	resp := getPlayout(t, srv, "/playout/stream/ch1?token="+playoutToken)
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status %d, want 501", resp.StatusCode)
	}
}

// --- The ffconcat playlist ---

// TWO identical entries pointing at the program endpoint. That is the mechanism: the demuxer
// needs a second entry to advance to when the first EOFs (prior-art §1).
func TestPlayoutPlaylist_IsTheTwoLineFfconcat(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{sessions: &fakePlayoutSessions{}})
	seedChannel(t, st, "ch1", "Channel One", 1, "internal")

	resp := getPlayout(t, srv, "/playout/playlist/ch1?token="+playoutToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	got := string(body)

	if !strings.HasPrefix(got, "ffconcat version 1.0\n") {
		t.Errorf("missing the ffconcat header: %q", got)
	}
	if n := strings.Count(got, "file '"); n != 2 {
		t.Errorf("%d entries, want 2 — one entry ends the channel after the first program", n)
	}
	// Entries must be ABSOLUTE and carry the token: ffmpeg is a separate process with no
	// notion of "the origin this came from", and it cannot set headers.
	if !strings.Contains(got, "http://loomarr.local:8080/playout/program/ch1") {
		t.Errorf("entries are not absolute URLs built from server.public_url: %q", got)
	}
	if !strings.Contains(got, "token="+playoutToken) {
		t.Errorf("entries carry no token; ffmpeg cannot authenticate any other way: %q", got)
	}
}

// The URL must come from server.public_url, NEVER from the Host header. The playlist URL is
// what the parent ffmpeg re-opens forever, so a spoofed Host points a long-lived channel at an
// attacker's server for as long as it runs.
func TestPlayoutPlaylist_IgnoresHostHeaderSpoofing(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{sessions: &fakePlayoutSessions{}})
	seedChannel(t, st, "ch1", "Channel One", 1, "internal")

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/playout/playlist/ch1?token="+playoutToken, nil)
	req.Host = "evil.example.com"
	req.Header.Set("X-Forwarded-Host", "evil.example.com")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "evil.example.com") {
		t.Errorf("a spoofed Host reached the playlist URL — the channel would stream from "+
			"an attacker's server: %s", body)
	}
	if !strings.Contains(string(body), "loomarr.local") {
		t.Errorf("want the configured public_url: %s", body)
	}
}

// No public_url ⇒ say so. There is no safe relative fallback: ffmpeg resolves these URLs
// itself and a relative one is simply not fetchable.
func TestPlayoutPlaylist_UnsetPublicURLIsExplained(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{sessions: &fakePlayoutSessions{}, publicURL: " "})
	seedChannel(t, st, "ch1", "Channel One", 1, "internal")

	resp := getPlayout(t, srv, "/playout/playlist/ch1?token="+playoutToken)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503 when public_url is unset", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "public address") {
		t.Errorf("the error does not name the missing setting: %s", body)
	}
}

// --- The tuner M3U ---

// tvg-id must match the guide's channel id, tvg-chno carries the operator's numbering, and the
// stream URL must be absolute + tokenized.
func TestPlayoutTuner_CarriesGuideCorrelationAttributes(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{sessions: &fakePlayoutSessions{}})
	seedChannel(t, st, "ch1", "Channel One", 3, "internal")

	resp := getPlayout(t, srv, "/playout/tuner.m3u?token="+playoutToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "mpegurl") {
		t.Errorf("Content-Type = %q, want an m3u type", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	got := string(body)

	if !strings.HasPrefix(got, "#EXTM3U\n") {
		t.Errorf("missing the M3U header: %q", got)
	}
	// tvg-id ties the entry to its XMLTV <channel id>; a mismatch means the channel plays
	// with an EMPTY guide, which is silent.
	if !strings.Contains(got, `tvg-id="ch1"`) {
		t.Errorf("no tvg-id — the channel would appear with no listings: %q", got)
	}
	if !strings.Contains(got, `tvg-chno="3"`) {
		t.Errorf("no tvg-chno — the media server would impose its own numbering: %q", got)
	}
	if !strings.Contains(got, "http://loomarr.local:8080/playout/stream/ch1?token="+playoutToken) {
		t.Errorf("stream URL is not absolute + tokenized: %q", got)
	}
}

// A channel on the TUNARR backend must not appear in Loomarr's tuner, or the media server has
// two tuners offering the same channel and picks unpredictably — presenting as a channel that
// plays sometimes and not others.
func TestPlayoutTuner_ExcludesTunarrBackedChannels(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{sessions: &fakePlayoutSessions{}})
	seedChannel(t, st, "mine", "Internal Channel", 1, "internal")
	seedChannel(t, st, "theirs", "Tunarr Channel", 2, "tunarr")

	resp := getPlayout(t, srv, "/playout/tuner.m3u?token="+playoutToken)
	body, _ := io.ReadAll(resp.Body)
	got := string(body)

	if !strings.Contains(got, "Internal Channel") {
		t.Errorf("the internal channel is missing: %q", got)
	}
	if strings.Contains(got, "Tunarr Channel") {
		t.Errorf("a Tunarr-backed channel appeared in Loomarr's tuner — two tuners would "+
			"offer the same channel: %q", got)
	}
}

// A channel with no explicit backend INHERITS the global (§15 nil-means-inherit). With the
// global set to tunarr, an unconfigured channel must not be served.
func TestPlayoutTuner_InheritsTheGlobalBackend(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{sessions: &fakePlayoutSessions{}, backend: "tunarr"})
	seedChannel(t, st, "inherits", "Inherits Global", 1, "")  // no policy
	seedChannel(t, st, "opted-in", "Opted In", 2, "internal") // explicit override

	resp := getPlayout(t, srv, "/playout/tuner.m3u?token="+playoutToken)
	body, _ := io.ReadAll(resp.Body)
	got := string(body)

	if strings.Contains(got, "Inherits Global") {
		t.Errorf("a channel with no backend set was served while the global is tunarr: %q", got)
	}
	// …but an explicit per-channel override still wins over the global.
	if !strings.Contains(got, "Opted In") {
		t.Errorf("an explicitly-internal channel was not served: %q", got)
	}
}

// A channel name is operator text and lands in an M3U attribute. A quote must not escape it.
func TestPlayoutTuner_QuotesInChannelNamesDoNotBreakTheM3U(t *testing.T) {
	srv, st := newPlayoutServer(t, playoutOpts{sessions: &fakePlayoutSessions{}})
	seedChannel(t, st, "ch1", `Bob's "Best" Movies`, 1, "internal")

	resp := getPlayout(t, srv, "/playout/tuner.m3u?token="+playoutToken)
	body, _ := io.ReadAll(resp.Body)
	got := string(body)

	// Each attribute must remain a single well-formed key="value" pair: an unescaped quote
	// would terminate tvg-name early and the rest would parse as new attributes.
	for _, attr := range []string{"tvg-id=", "tvg-name=", "tvg-chno="} {
		if !strings.Contains(got, attr) {
			t.Errorf("missing %s after a name with quotes: %q", attr, got)
		}
	}
	if strings.Contains(got, `tvg-name="Bob's "Best" Movies"`) {
		t.Errorf("the quotes in the name were not escaped: %q", got)
	}
}
