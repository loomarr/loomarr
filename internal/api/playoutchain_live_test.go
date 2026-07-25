//go:build ffmpeg

package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"log/slog"
)

// THE WHOLE MECHANISM (make test-ffmpeg), end to end, with a real ffmpeg parent.
//
// A real concat demuxer reads our real playlist endpoint, opens our real program endpoint,
// gets a finite encode, sees EOF, ADVANCES, and re-requests. This is the only test that proves
// programs actually sequence — every other test asserts one request in isolation.
func TestLiveChain_RealFfmpegAdvancesThroughPrograms(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg")
	}

	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/chain.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// Each program request returns a SHORT synthetic encode. Counting requests is how we
	// observe the demuxer advancing.
	var requests int64
	profile := playout.DefaultProfile()
	profile.Width, profile.Height = 320, 180

	// A 10s local clip standing in for a library item, so this needs no media server.
	srcFile := t.TempDir() + "/src.mp4"
	if o, err := exec.Command(bin, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=25:duration=10",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		"-shortest", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac",
		srcFile).CombinedOutput(); err != nil {
		t.Fatalf("could not build the source clip: %v\n%s", err, o)
	}

	var srv *httptest.Server
	opts := api.Options{
		Store:           st,
		Auth:            api.NewTokenAuthorizer(adminToken),
		Log:             slog.New(slog.DiscardHandler),
		PlayoutSecret:   func() string { return playoutToken },
		PlayoutResolver: &chainResolver{profile: profile, n: &requests, src: srcFile},
		PlayoutEncoder: func(ctx context.Context, args []string) (*playout.Process, error) {
			return playout.Start(ctx, bin, args, nil, nil)
		},
	}
	cfg := map[string]string{"playout.backend": "internal"}
	opts.LiveConfig = func(k string) string {
		if k == "server.public_url" {
			return srv.URL
		}
		return cfg[k]
	}
	srv = httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), opts))
	t.Cleanup(srv.Close)

	ch := store.Channel{Channel: schedule.Channel{ID: "ch1", Name: "Chain", Number: 1}}
	ch.Policy.Playout = &schedule.PlayoutPolicy{Backend: "internal"}
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}

	// The REAL parent args against the REAL playlist endpoint.
	playlistURL := srv.URL + "/playout/playlist/ch1?token=" + playoutToken
	args := playout.ConcatArgs(playlistURL)
	// Bound the infinite parent and write to a file we can probe.
	out := t.TempDir() + "/joined.ts"
	var bounded []string
	for _, a := range args {
		if a == "pipe:1" {
			continue
		}
		bounded = append(bounded, a)
	}
	bounded = append(bounded, "-t", "12", out)
	// NOTE: offlineCardDuration is 30s, so a 12s parent cannot span a boundary — the
	// resolver below returns a SHORT program instead.

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	proc, err := playout.Start(ctx, bin, bounded, nil, nil)
	if err != nil {
		t.Fatalf("parent start: %v", err)
	}
	go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
	if err := proc.Wait(); err != nil {
		t.Fatalf("the parent failed: %v\nlast stderr: %s", err, proc.LastError())
	}

	got := atomic.LoadInt64(&requests)
	t.Logf("the demuxer requested %d programs in 12s of playout", got)
	// Each card is ~4s, so 12s of output requires the demuxer to have advanced at least twice.
	if got < 2 {
		t.Errorf("only %d program request(s) — the demuxer did not ADVANCE, so programs are "+
			"not sequencing", got)
	}

	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		return
	}
	o, err := exec.CommandContext(ctx, probe, "-v", "error",
		"-show_entries", "format=duration,nb_streams", "-of", "csv=p=0", out).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	t.Logf("joined output: %s", strings.TrimSpace(string(o)))
	if strings.HasPrefix(strings.TrimSpace(string(o)), "0") {
		t.Error("the joined stream has no duration — nothing was concatenated")
	}
}

type chainResolver struct {
	profile playout.Profile
	n       *int64
	src     string // a local file standing in for a library item
}

func (c *chainResolver) AiringNow(context.Context, string) (playout.Airing, string, error) {
	atomic.AddInt64(c.n, 1)
	// A real PLAYABLE program, short, from a local file — so the child exits quickly and the
	// demuxer has to advance. The offline card is hardcoded to 30s, which cannot span a
	// boundary inside a short test.
	return playout.Airing{
		Kind: schedule.SlotProgram, LibraryItemID: "local", Title: "Short",
		Offset: 0, Remaining: 3 * time.Second,
	}, c.src, nil
}

func (c *chainResolver) Profile(context.Context) playout.Profile { return c.profile }

var _ = http.MethodGet
