package api_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeResolver answers "what's on" without a store or a media server.
type fakeResolver struct {
	airing  playout.Airing
	url     string
	err     error
	profile playout.Profile
	calls   int
	mu      sync.Mutex
}

func (f *fakeResolver) AiringNow(_ context.Context, _ string) (playout.Airing, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.airing, f.url, f.err
}

func (f *fakeResolver) Profile(context.Context) playout.Profile {
	if f.profile.Width == 0 {
		return playout.DefaultProfile()
	}
	return f.profile
}

func (f *fakeResolver) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeEncoder captures the args it was asked to run and serves canned bytes as the encoder's
// output, so the handler can be exercised without executing ffmpeg.
type fakeEncoder struct {
	mu      sync.Mutex
	gotArgs []string
	output  string
	failErr error
}

func (f *fakeEncoder) start(ctx context.Context, args []string) (*playout.Process, error) {
	f.mu.Lock()
	f.gotArgs = args
	f.mu.Unlock()
	if f.failErr != nil {
		return nil, f.failErr
	}
	// The REAL playout.Start, driven with `sh` instead of ffmpeg. That keeps the process
	// supervision under test (stdout piping, the context binding, Wait) while producing
	// deterministic bytes — and it needs no test-only export in the production package.
	//
	// printf then EXIT: a child's exit is the behaviour under test, since that EOF is what
	// advances the channel.
	return playout.Start(ctx, "sh", []string{"-c", "printf %s " + shellQuote(f.output)}, nil, nil)
}

// shellQuote wraps a string in single quotes for `sh -c`.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (f *fakeEncoder) args() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gotArgs
}

type programOpts struct {
	resolver api.PlayoutResolver
	encoder  api.PlayoutEncoder
	noToken  bool
}

func newProgramServer(t *testing.T, o programOpts) *httptest.Server {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/prog.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := map[string]string{
		"server.public_url": "http://loomarr.local:8080",
		"playout.backend":   "internal",
	}
	opts := api.Options{
		Store:           st,
		Auth:            api.NewTokenAuthorizer(adminToken),
		Log:             slog.New(slog.DiscardHandler),
		PlayoutResolver: o.resolver,
		PlayoutEncoder:  o.encoder,
		LiveConfig:      func(k string) string { return cfg[k] },
	}
	if !o.noToken {
		opts.PlayoutSecret = func() string { return playoutToken }
	}
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), opts))
	t.Cleanup(srv.Close)
	return srv
}

func playableAiring(offset, remaining time.Duration) playout.Airing {
	return playout.Airing{
		Kind: schedule.SlotProgram, LibraryItemID: "item-1", Title: "Heat",
		Offset: offset, Remaining: remaining,
	}
}

// The device token gates this route like every other playout route (§11).
func TestPlayoutProgram_RequiresTheDeviceToken(t *testing.T) {
	srv := newProgramServer(t, programOpts{
		resolver: &fakeResolver{airing: playableAiring(0, time.Hour), url: "http://emby/v/1"},
		encoder:  (&fakeEncoder{output: "ts"}).start,
	})
	for _, q := range []string{"", "?token=wrong", "?token=" + playoutToken[:8]} {
		resp := getPlayout(t, srv, "/playout/program/ch1"+q)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("token %q: status %d, want 404", q, resp.StatusCode)
		}
	}
}

// THE CORE BEHAVIOUR: one program's bytes, then the response ENDS. That EOF is what makes the
// concat demuxer advance to the next program — a response that never ended would pin the
// channel to one program forever.
func TestPlayoutProgram_StreamsOneProgramThenEnds(t *testing.T) {
	enc := &fakeEncoder{output: "program-bytes"}
	srv := newProgramServer(t, programOpts{
		resolver: &fakeResolver{airing: playableAiring(0, time.Hour), url: "http://emby/v/1"},
		encoder:  enc.start,
	})

	resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "video/mp2t" {
		t.Errorf("Content-Type = %q, want video/mp2t", ct)
	}

	// The body must terminate on its own — read it fully with a deadline.
	done := make(chan []byte, 1)
	go func() { b, _ := io.ReadAll(resp.Body); done <- b }()
	select {
	case body := <-done:
		if string(body) != "program-bytes" {
			t.Errorf("body = %q, want the encoder's output", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the response never ended — the channel would be stuck on one program")
	}
}

// The seek offset and the slot's remaining time must reach ffmpeg, or a mid-program tune-in
// restarts the show and a program overruns its slot.
func TestPlayoutProgram_PassesTheSeekAndTheSlotBound(t *testing.T) {
	enc := &fakeEncoder{output: "x"}
	srv := newProgramServer(t, programOpts{
		resolver: &fakeResolver{
			airing: playableAiring(40*time.Minute, 20*time.Minute),
			url:    "http://emby/Videos/abc/stream?static=true",
		},
		encoder: enc.start,
	})

	resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken)
	_, _ = io.ReadAll(resp.Body)

	got := strings.Join(enc.args(), " ")
	if !strings.Contains(got, "-ss 2400.000") {
		t.Errorf("no 40-minute seek — the joiner would restart the show: %q", got)
	}
	if !strings.Contains(got, "-t 1200.000") {
		t.Errorf("no slot bound — the program would overrun its slot: %q", got)
	}
	if !strings.Contains(got, "http://emby/Videos/abc/stream") {
		t.Errorf("the resolved stream URL did not reach ffmpeg: %q", got)
	}
}

// Nothing airing ⇒ the offline CARD, not an empty 200. An empty body EOFs the demuxer
// instantly and it re-requests in a tight loop, spinning a core on an empty channel.
func TestPlayoutProgram_NothingAiringServesABoundedCard(t *testing.T) {
	enc := &fakeEncoder{output: "card"}
	srv := newProgramServer(t, programOpts{
		// A flex Airing with no item: "nothing is on".
		resolver: &fakeResolver{airing: playout.Airing{Kind: schedule.SlotFlex}},
		encoder:  enc.start,
	})

	resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200 — an unplayable channel still gets a card", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("empty body — the demuxer would re-request in a tight loop")
	}

	got := strings.Join(enc.args(), " ")
	if !strings.Contains(got, "color=c=black") {
		t.Errorf("not the synthetic card: %q", got)
	}
	// BOUNDED is the point: the card args loop forever by design, so without -t the channel
	// could never pick up content that later lands.
	if !strings.Contains(got, "-t 30.000") {
		t.Errorf("the card is not bounded — the channel could never pick up new content: %q", got)
	}
	// And the silent audio track must survive: a video-only MPEG-TS is a classic cause of a
	// player refusing to play.
	if !strings.Contains(got, "anullsrc") {
		t.Errorf("the card lost its silent audio track: %q", got)
	}
}

// A resolver failure is a RETRYABLE 502, not a 500. The usual cause is the media server being
// unreachable, which is upstream of us, and the demuxer will come back.
func TestPlayoutProgram_ResolverFailureIsRetryable(t *testing.T) {
	srv := newProgramServer(t, programOpts{
		resolver: &fakeResolver{err: errors.New("emby unreachable")},
		encoder:  (&fakeEncoder{}).start,
	})
	resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken)
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status %d, want 502", resp.StatusCode)
	}
}

// A channel that does not exist is a 404, so the demuxer stops rather than retrying forever.
func TestPlayoutProgram_UnknownChannelIs404(t *testing.T) {
	srv := newProgramServer(t, programOpts{
		resolver: &fakeResolver{err: store.ErrNotFound},
		encoder:  (&fakeEncoder{}).start,
	})
	resp := getPlayout(t, srv, "/playout/program/nope?token="+playoutToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

// An encoder that fails to start must report it, not hang or serve a truncated 200.
func TestPlayoutProgram_EncoderStartFailureIsReported(t *testing.T) {
	srv := newProgramServer(t, programOpts{
		resolver: &fakeResolver{airing: playableAiring(0, time.Hour), url: "http://emby/v/1"},
		encoder:  (&fakeEncoder{failErr: errors.New("no such binary")}).start,
	})
	resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken)
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status %d, want 502", resp.StatusCode)
	}
}

// Playout not running is a 501 that explains itself, not a 404 that reads as a wiring mistake.
func TestPlayoutProgram_NotRunningExplainsItself(t *testing.T) {
	srv := newProgramServer(t, programOpts{resolver: nil, encoder: nil})
	resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken)
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status %d, want 501", resp.StatusCode)
	}
}

// REPEATED requests are the normal case — the demuxer opens this once per program, forever. Each
// must resolve independently, and none may leak state into the next.
func TestPlayoutProgram_IsCalledRepeatedlyAndStaysConsistent(t *testing.T) {
	res := &fakeResolver{airing: playableAiring(10*time.Second, time.Minute), url: "http://emby/v/1"}
	enc := &fakeEncoder{output: "chunk"}
	srv := newProgramServer(t, programOpts{resolver: res, encoder: enc.start})

	for i := 0; i < 5; i++ {
		resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: status %d", i, resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "chunk" {
			t.Errorf("request %d: body = %q", i, body)
		}
	}
	if got := res.callCount(); got != 5 {
		t.Errorf("resolver called %d times for 5 requests — the handler is caching or skipping", got)
	}
}

// A client disconnecting mid-program must not leave the encoder running. The child's lifetime is
// bound to the request, and a leaked child is a core burned until the process dies.
func TestPlayoutProgram_DisconnectStopsTheEncoder(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	// An encoder that would run for a long time, so the disconnect is what ends it.
	marker := t.TempDir() + "/alive"
	enc := api.PlayoutEncoder(func(ctx context.Context, _ []string) (*playout.Process, error) {
		// Removes the marker on SIGTERM, so the assertion below observes the real teardown
		// path (Stop signals the process GROUP) rather than just the process disappearing.
		script := "trap 'rm -f " + marker + "; exit 0' TERM; touch " + marker +
			"; while :; do printf x; sleep 0.05; done"
		return playout.Start(ctx, "sh", []string{"-c", script}, nil, nil)
	})
	srv := newProgramServer(t, programOpts{
		resolver: &fakeResolver{airing: playableAiring(0, time.Hour), url: "http://emby/v/1"},
		encoder:  enc,
	})

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		srv.URL+"/playout/program/ch1?token="+playoutToken, nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatalf("stream did not start: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the encoder never started: %v", err)
	}

	cancel()
	_ = resp.Body.Close()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			return // the child was signalled and cleaned up
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the encoder survived the client disconnect — an orphan burning a core")
}

// A channel that produces NO BYTES must say so loudly.
//
// This is the bug that took a live channel down silently: a misconfigured hardware encoder died
// at startup, which closes stdout — so the copy saw a clean EOF, copyErr was nil, and the old
// `copyErr != nil && n == 0` guard never fired. The viewer's player buffered forever with not
// one line in the log at INFO, because ffmpeg's stderr goes to DEBUG.
func TestPlayoutProgram_ZeroBytesIsLoggedAsAWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/zero.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := map[string]string{"server.public_url": "http://loomarr.local:8080", "playout.backend": "internal"}
	srv := httptest.NewServer(api.Router(logger, api.Options{
		Store: st, Auth: api.NewTokenAuthorizer(adminToken), Log: logger,
		PlayoutSecret:   func() string { return playoutToken },
		PlayoutResolver: &fakeResolver{airing: playableAiring(0, time.Hour), url: "http://emby/v/1"},
		// An "encoder" that exits immediately without writing anything — exactly what a
		// hardware encoder does when its device is missing.
		PlayoutEncoder: func(ctx context.Context, _ []string) (*playout.Process, error) {
			return playout.Start(ctx, "sh", []string{"-c", "exit 0"}, nil, nil)
		},
		LiveConfig: func(k string) string { return cfg[k] },
	}))
	t.Cleanup(srv.Close)

	resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken)
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	got := buf.String()
	if !strings.Contains(got, "NO OUTPUT") {
		t.Errorf("a channel that produced zero bytes logged nothing at INFO — the operator "+
			"sees a buffering player and an empty log. Got:\n%s", got)
	}
}
