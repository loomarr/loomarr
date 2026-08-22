//go:build ffmpeg

package api_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

// This is the complete production transport shape: a real Go block supervisor repeatedly opens
// the authenticated finite-program endpoint, real child encoders end at their Airing boundaries,
// and one real copy mux keeps emitting. It guards the boundary handoff independently of browser HLS.
func TestLiveChain_BlockSupervisorAdvancesThroughPrograms(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg")
	}

	st := openTestStore(t, t.TempDir()+"/chain.db")
	t.Cleanup(func() { _ = st.Close() })
	var requests atomic.Int64
	profile := playout.DefaultProfile()
	profile.Width, profile.Height = 320, 180

	srcFile := t.TempDir() + "/src.mp4"
	if output, err := exec.Command(bin, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=25:duration=2",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		"-shortest", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac",
		srcFile).CombinedOutput(); err != nil {
		t.Fatalf("build source clip: %v\n%s", err, output)
	}

	opts := api.Options{
		Store: st, Auth: api.NewTokenAuthorizer(adminToken), Log: slog.New(slog.DiscardHandler),
		PlayoutSecret: func() string { return playoutToken }, Playout: &testkit.Playout{},
		PlayoutResolver: &chainResolver{profile: profile, requests: &requests, src: srcFile},
		PlayoutEncoder: func(ctx context.Context, args []string, progress func(playout.Progress)) (*playout.Process, error) {
			return playout.Start(ctx, bin, args, nil, progress)
		},
	}
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), opts))
	t.Cleanup(srv.Close)

	ch := store.Channel{Channel: schedule.Channel{ID: "ch1", Name: "Chain", Number: 1}}
	ch.Policy.Playout = &schedule.PlayoutPolicy{Backend: "internal"}
	if _, err := st.SaveChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	channel, err := playout.BlockSpawner(bin, liveHTTPBlockSource(srv), nil)(ctx, "ch1", playout.PlanBaseline)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	outPath := t.TempDir() + "/channel.ts"
	out, err := os.Create(outPath)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(out, channel.Stdout)
		copyDone <- copyErr
	}()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for requests.Load() < 3 && ctx.Err() == nil {
		<-ticker.C
	}
	cancel()
	_ = channel.Wait()
	if err := <-copyDone; err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got < 3 {
		t.Fatalf("program requests = %d, want at least three finite block advances", got)
	}
	if info, err := os.Stat(outPath); err != nil || info.Size() == 0 {
		t.Fatalf("continuous mux produced no MPEG-TS: info=%v err=%v", info, err)
	}
}

func liveHTTPBlockSource(srv *httptest.Server) playout.BlockSource {
	var broadcast string
	return func(ctx context.Context, channel string, plan playout.EncodePlan) (playout.Block, error) {
		query := url.Values{"token": {playoutToken}, "plan": {plan.String()}}
		if broadcast != "" {
			query.Set(api.PlayoutBroadcastFormatQuery, broadcast)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			srv.URL+"/v1/playout/program/"+url.PathEscape(channel)+"?"+query.Encode(), nil)
		if err != nil {
			return playout.Block{}, err
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			return playout.Block{}, err
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return playout.Block{}, fmt.Errorf("program status %s", resp.Status)
		}
		format, ok := playout.ParseBroadcastFormat(resp.Header.Get(api.PlayoutBroadcastFormatHeader))
		if !ok {
			_ = resp.Body.Close()
			return playout.Block{}, fmt.Errorf("invalid broadcast format")
		}
		if broadcast == "" {
			broadcast = format.String()
		}
		identity, ok := api.ParsePlayoutAiringIdentity(resp.Header)
		if !ok {
			_ = resp.Body.Close()
			return playout.Block{}, fmt.Errorf("invalid airing identity")
		}
		return playout.Block{Content: resp.Body, Identity: identity}, nil
	}
}

type chainResolver struct {
	profile  playout.Profile
	requests *atomic.Int64
	src      string
}

func (c *chainResolver) AiringNow(context.Context, string) (playout.Airing, string, error) {
	n := c.requests.Add(1)
	return playout.Airing{
		StartedAt: time.Unix(n, 0).UTC(), Identity: fmt.Sprintf("block-%d", n),
		Kind: schedule.SlotProgram, LibraryItemID: "local", Title: "Short",
		Remaining: 2 * time.Second,
	}, c.src, nil
}

func (c *chainResolver) Profile(context.Context) playout.Profile           { return c.profile }
func (c *chainResolver) AudioTrackFor(context.Context, string, string) int { return 0 }
func (c *chainResolver) Tracks(context.Context, string) (playout.MediaTracks, error) {
	return playout.MediaTracks{}, nil
}
func (c *chainResolver) PlanFor(context.Context, string, playout.EncodePlan) (playout.CopyPlan, playout.MediaFormat) {
	return playout.CopyPlan{}, playout.MediaFormat{}
}
func (c *chainResolver) ChannelCodec(context.Context, string) string { return "h264" }
