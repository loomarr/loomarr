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
	airing playout.Airing
	url    string
	err    error
	// audioTrack is the index AudioTrackFor returns — zero (the file's first track) unless a
	// test is specifically about audio selection.
	audioTrack int
	profile    playout.Profile
	calls      int
	mu         sync.Mutex
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

// AudioTrackFor returns whatever the test set, defaulting to the file's first track — the same
// answer the real resolver gives when no language preference is configured.
func (f *fakeResolver) AudioTrackFor(context.Context, string) int { return f.audioTrack }

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
	// progressScript emits ffmpeg-shaped progress on fd 3 (`>&3`) so the parser + the V16
	// reporting path can be tested end to end with `sh` standing in for ffmpeg.
	progressScript string
}

func (f *fakeEncoder) start(
	ctx context.Context, args []string, onProgress func(playout.Progress),
) (*playout.Process, error) {
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
	// `progressScript` (when set) writes ffmpeg-shaped key=value lines to fd 3 before the
	// output, so the REAL parser and the reporting path are exercised without ffmpeg.
	script := "printf %s " + shellQuote(f.output)
	if f.progressScript != "" {
		script = f.progressScript + "; " + script
	}
	return playout.Start(ctx, "sh", []string{"-c", script}, nil, onProgress)
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
	sessions api.PlayoutSessions
	// config overlays LiveConfig, for tests about a setting the handler reads live —
	// `filler.target_lufs` (§10 V40) is the first.
	config map[string]string
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
	for k, v := range o.config {
		cfg[k] = v
	}
	opts := api.Options{
		Store:           st,
		Auth:            api.NewTokenAuthorizer(adminToken),
		Log:             slog.New(slog.DiscardHandler),
		PlayoutResolver: o.resolver,
		PlayoutEncoder:  o.encoder,
		PlayoutSessions: o.sessions,
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

// The audio-language preference must reach the ENCODER, not just the resolver (§9.1). This is
// the seam the Russian-audio bug lived on: the selection can be perfectly correct and still not
// be applied, because the handler builds the args.
func TestPlayoutProgram_MapsTheResolvedAudioTrack(t *testing.T) {
	res := &fakeResolver{
		airing:     playableAiring(0, time.Minute),
		url:        "http://emby/v/1",
		audioTrack: 2,
	}
	enc := &fakeEncoder{output: "chunk"}
	srv := newProgramServer(t, programOpts{resolver: res, encoder: enc.start})

	resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	_, _ = io.ReadAll(resp.Body)

	args := strings.Join(enc.args(), " ")
	if !strings.Contains(args, "-map 0:a:2") {
		t.Errorf("args did not map the resolved audio track 2:\n%s", args)
	}
	if strings.Contains(args, "-map 0:a:0") {
		t.Errorf("args still map track 0 — the preference was ignored or double-mapped:\n%s", args)
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
	enc := api.PlayoutEncoder(func(
		ctx context.Context, _ []string, _ func(playout.Progress),
	) (*playout.Process, error) {
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
		PlayoutEncoder: func(ctx context.Context, _ []string, _ func(playout.Progress)) (*playout.Process, error) {
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

// --- V16: the per-program encoder's progress reaches the session ---------------

// END TO END through the REAL parser: `sh` writes ffmpeg-shaped `key=value` lines to fd 3,
// exactly as ffmpeg's `-progress pipe:3` does, and the assertion is what arrived at the session.
//
// This is the wiring V16 exists to fix. `playout.Start` has accepted an `onProgress` callback
// since the supervisor was written, and every caller passed nil — so each sample was parsed and
// discarded. Nothing downstream could tell the difference between "the encoder is at 12× and
// healthy" and "no telemetry exists", which is why the dashboard had no real numbers to show.
func TestProgramEncoder_ReportsProgressToTheSession(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	sessions := &fakePlayoutSessions{}
	enc := &fakeEncoder{
		output: "ts-bytes",
		// A whole ffmpeg progress block. `progress=continue` is the terminator the parser
		// emits on, so anything before it must arrive as ONE sample, never half-updated.
		progressScript: `{ printf 'frame=120\nspeed=12.4x\nout_time_ms=4000000\nprogress=continue\n' >&3; }`,
	}
	srv := newProgramServer(t, programOpts{
		resolver: &fakeResolver{airing: playableAiring(0, time.Hour), url: "http://emby/v/1"},
		encoder:  enc.start,
		sessions: sessions,
	})

	resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("program → %d, want 200", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)

	// ⚠ POLLED, not read once. `sh` writes to fd 3 on its own schedule, and draining the
	// response body does not wait for it — so a single read asserts that a subprocess won a
	// race. It usually does, which is worse than never: this test passed locally and on the
	// PR, then failed on main's post-merge run with "no progress reached the session", which
	// reads as the wiring bug V16 fixed rather than as the scheduling artefact it was.
	//
	// A deadline rather than a sleep: the fast path stays fast (it typically lands on the
	// first pass) and a real regression still fails, just a second later.
	var got []reportedProgram
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if got = sessions.reports(); len(got) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(got) == 0 {
		t.Fatal("no progress reached the session — the callback is nil somewhere in the chain")
	}
	last := got[len(got)-1]
	if last.channelID != "ch1" {
		t.Errorf("reported channel %q, want ch1 — telemetry keyed to the wrong channel is worse than none", last.channelID)
	}
	if last.progress.Speed != 12.4 {
		t.Errorf("speed = %v, want 12.4 parsed from the progress stream", last.progress.Speed)
	}
	// ffmpeg reports out_time_ms in MICROseconds despite the name; the parser divides.
	if last.progress.OutTimeMS != 4000 {
		t.Errorf("outTimeMs = %d, want 4000 (4s) — ffmpeg's out_time_ms is microseconds", last.progress.OutTimeMS)
	}
	if last.progress.Frame != 120 {
		t.Errorf("frame = %d, want 120", last.progress.Frame)
	}
}

// The encoder the program RESOLVED is what gets reported — not the session's own `-c copy`
// parent, which never encodes anything and whose "encoder" would be copy.
func TestProgramEncoder_ReportsTheResolvedEncoder(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	sessions := &fakePlayoutSessions{}
	enc := &fakeEncoder{
		output:         "ts",
		progressScript: `{ printf 'speed=1.0x\nprogress=continue\n' >&3; }`,
	}
	srv := newProgramServer(t, programOpts{
		resolver: &fakeResolver{airing: playableAiring(0, time.Hour), url: "http://emby/v/1"},
		encoder:  enc.start,
		sessions: sessions,
	})

	resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken)
	_, _ = io.Copy(io.Discard, resp.Body)

	got := sessions.reports()
	if len(got) == 0 {
		t.Fatal("no report reached the session")
	}
	// fakeResolver's Profile resolves software; the point is that SOME resolved encoder is
	// carried, not the copy-parent's absence of one.
	if got[len(got)-1].encoder == "" {
		t.Error("reported an empty encoder — the dashboard's hardware/software badge would be blank")
	}
}

// fillerAiring is a resolved commercial clip. ⚠ `Source` is what marks it as filler — set for a
// clip resolved to a local file under FILLER_DIR, empty for a library title (see Airing.Source).
// That existing field is the discriminator the loudness gate reads, so it needed no new plumbing.
func fillerAiring(remaining time.Duration) playout.Airing {
	return playout.Airing{
		Kind: schedule.SlotProgram, Source: "/filler/14/36/abc.mp4", Title: "Frosted Flakes",
		Remaining: remaining,
	}
}

// Loudness normalisation, FILLER ONLY (§10 V40).
//
// Measured across real fetched clips the spread was -21.8 to -32.6 LUFS — about 11 dB of
// clip-to-clip jump, which is what an operator hears as "some of these are too quiet".
func TestPlayoutProgram_NormalisesFillerLoudness(t *testing.T) {
	enc := &fakeEncoder{output: "ts"}
	srv := newProgramServer(t, programOpts{
		resolver: &fakeResolver{airing: fillerAiring(30 * time.Second), url: "http://emby/v/1"},
		encoder:  enc.start,
		config:   map[string]string{"filler.target_lufs": "-23"},
	})

	if resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken); resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if joined := strings.Join(enc.args(), " "); !strings.Contains(joined, "loudnorm=I=-23") {
		t.Errorf("filler was not normalised; args = %v", enc.args())
	}
}

// ⚠ **THE guard.** A feature film normalised to advert loudness loses its dynamic range — the
// quiet scenes come up and the loud ones come down, which is the opposite of what a film wants.
// The problem being solved is adverts recorded a decade apart; a library title must be untouched
// even with the setting on.
func TestPlayoutProgram_LeavesLibraryProgramsAlone(t *testing.T) {
	enc := &fakeEncoder{output: "ts"}
	srv := newProgramServer(t, programOpts{
		// A library title: LibraryItemID set, Source empty.
		resolver: &fakeResolver{airing: playableAiring(0, time.Hour), url: "http://emby/v/1"},
		encoder:  enc.start,
		config:   map[string]string{"filler.target_lufs": "-23"},
	})

	if resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken); resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if joined := strings.Join(enc.args(), " "); strings.Contains(joined, "loudnorm") {
		t.Errorf("a library program was normalised; args = %v", enc.args())
	}
}

// Empty target ⇒ no filter, even for filler. That is what "set it empty to disable" means in §15,
// and it keeps the pre-V40 behaviour reachable.
func TestPlayoutProgram_EmptyTargetDisablesNormalisation(t *testing.T) {
	enc := &fakeEncoder{output: "ts"}
	srv := newProgramServer(t, programOpts{
		resolver: &fakeResolver{airing: fillerAiring(30 * time.Second), url: "http://emby/v/1"},
		encoder:  enc.start,
		config:   map[string]string{"filler.target_lufs": ""},
	})

	if resp := getPlayout(t, srv, "/playout/program/ch1?token="+playoutToken); resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if joined := strings.Join(enc.args(), " "); strings.Contains(joined, "loudnorm") {
		t.Errorf("normalisation ran with the setting disabled; args = %v", enc.args())
	}
}
