package playout

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A pending first segment is media startup, not an unfinished admission. The
// lifecycle writer must be able to stop that remux while the request is waiting.
func TestOriginQuiesceCancelsPendingHLSStartup(t *testing.T) {
	testOriginRetiresPendingHLSStartup(t, (*Origin).Quiesce, false)
}

func TestOriginStopAllCancelsPendingHLSStartupAndRemainsReusable(t *testing.T) {
	testOriginRetiresPendingHLSStartup(t, (*Origin).StopAll, true)
}

func TestOriginStopChannelCancelsPendingHLSStartupAndRemainsReusable(t *testing.T) {
	testOriginRetiresPendingHLSStartup(t, func(origin *Origin) { origin.StopChannel("pending") }, true)
}

func testOriginRetiresPendingHLSStartup(t *testing.T, stop func(*Origin), reusable bool) {
	t.Helper()
	m := newTestHLSManagerWithPlaylist(t, &fakeAttacher{}, "#EXTM3U\n")
	m.readyTimeout = time.Minute
	started := make(chan string, 1)
	publishNext := false
	spawn := m.spawn
	m.spawn = func(ctx context.Context, bin, dir string, plan EncodePlan, log *slog.Logger) (*hlsProcess, error) {
		process, err := spawn(ctx, bin, dir, plan, log)
		if err == nil {
			if publishNext {
				err = os.WriteFile(filepath.Join(dir, hlsPlaylistName), []byte("#EXTM3U\n#EXTINF:4,\nseg-0.ts\n"), 0o600)
			} else {
				started <- filepath.Join(dir, hlsPlaylistName)
			}
		}
		return process, err
	}
	origin := newOrigin(nil, nil, m)
	tuned := make(chan error, 1)
	go func() {
		presentation, err := origin.Tune(context.Background(), TuneRequest{
			ChannelID: "pending", Plan: PlanBaseline, Delivery: DeliveryHLS,
		})
		if presentation.Release != nil {
			presentation.Release()
		}
		tuned <- err
	}()
	playlist := <-started
	stopped := make(chan struct{})
	// Rescue a failed regression without waiting for the production startup bound
	// or leaving a subprocess behind. Success never needs this playable segment.
	t.Cleanup(func() {
		_ = os.WriteFile(playlist, []byte("#EXTM3U\n#EXTINF:4,\nseg-0.ts\n"), 0o600)
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			t.Error("quiesce did not finish after regression cleanup")
		}
	})
	go func() {
		stop(origin)
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("quiesce waited behind HLS first-segment readiness instead of cancelling it")
	}
	select {
	case err := <-tuned:
		if err == nil {
			t.Fatal("stopped HLS startup returned a playable presentation")
		}
	case <-time.After(time.Second):
		t.Fatal("retired HLS startup request did not return")
	}
	if _, ok := m.AssetPath("pending", PlanBaseline, "seg-0.ts"); ok {
		t.Fatal("retired HLS remux still exposes an asset")
	}
	if reusable {
		// The first request and retirement have both returned before a fresh spawn
		// reads this flag; no startup goroutine still accesses it.
		publishNext = true
		presentation, err := origin.Tune(context.Background(), TuneRequest{ChannelID: "pending", Plan: PlanBaseline, Delivery: DeliveryHLS})
		if err != nil {
			t.Fatalf("retirement prevented a fresh tune: %v", err)
		}
		presentation.Release()
		if len(presentation.Manifest) == 0 {
			t.Fatal("fresh remux returned no manifest")
		}
	}

}

func TestOriginCancelledHLSWaitPreservesSharedPeer(t *testing.T) {
	m := newTestHLSManagerWithPlaylist(t, &fakeAttacher{}, "#EXTM3U\n")
	m.readyTimeout = time.Minute
	started := make(chan string, 1)
	spawn := m.spawn
	m.spawn = func(ctx context.Context, bin, dir string, plan EncodePlan, log *slog.Logger) (*hlsProcess, error) {
		process, err := spawn(ctx, bin, dir, plan, log)
		if err == nil {
			started <- filepath.Join(dir, hlsPlaylistName)
		}
		return process, err
	}
	origin := newOrigin(nil, nil, m)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tuned := make(chan error, 1)
	go func() {
		presentation, err := origin.Tune(ctx, TuneRequest{
			ChannelID: "shared", Plan: PlanBaseline, Delivery: DeliveryHLS,
		})
		if presentation.Release != nil {
			presentation.Release()
		}
		tuned <- err
	}()
	playlist := <-started
	peer, err := m.acquirePlaylist("shared", PlanBaseline)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.release()
	cancel()
	select {
	case err := <-tuned:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled HLS tune = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("abandoned HLS request kept waiting for media")
	}
	m.mu.Lock()
	remux := m.remuxes[remuxKey{channel: "shared", plan: PlanBaseline}]
	m.mu.Unlock()
	if remux == nil {
		t.Fatal("one cancelled request removed the peer's shared remux")
	}
	remux.mu.Lock()
	viewers, closed := remux.viewers, remux.closed
	remux.mu.Unlock()
	if viewers != 1 || closed {
		t.Fatalf("shared remux viewers=%d closed=%v, want one live peer", viewers, closed)
	}
	if err := os.WriteFile(playlist, []byte("#EXTM3U\n#EXTINF:4,\nseg-0.ts\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, release, err := peer.read(context.Background())
	if err != nil {
		t.Fatalf("peer lost playable HLS after cancellation: %v", err)
	}
	release()
	if path != playlist {
		t.Fatal("peer switched to a replacement remux")
	}
}
