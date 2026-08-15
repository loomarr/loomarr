package playout

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type tuneSessions struct {
	channel string
	stopped string
}

func (s *tuneSessions) Attach(_ context.Context, channel string, _ EncodePlan) (<-chan []byte, func(), error) {
	s.channel = channel
	return make(chan []byte), func() {}, nil
}

func (s *tuneSessions) StopChannel(channel string) { s.stopped = channel }
func (s *tuneSessions) Stop()                      { s.stopped = "*" }

func TestOriginTuneReportsUnavailableDelivery(t *testing.T) {
	t.Parallel()
	origin := newOrigin(nil, &tuneSessions{}, nil)
	_, err := origin.Tune(context.Background(), TuneRequest{
		ChannelID: "ch-one", Plan: PlanBaseline, Delivery: DeliveryHLS,
	})
	if !errors.Is(err, ErrUnsupportedDelivery) {
		t.Fatalf("Tune error = %v, want ErrUnsupportedDelivery", err)
	}
	if _, ok, err := origin.OpenAsset(context.Background(), "ch-one", PlanBaseline, "segment.ts"); err != nil || ok {
		t.Fatal("asset resolved without an HLS delivery")
	}
}

type tuneHLS struct {
	channel string
	path    string
	stopped string
}

func (h *tuneHLS) Playlist(channel string, _ EncodePlan) (string, func(), error) {
	h.channel = channel
	return h.path, func() {}, nil
}

func (h *tuneHLS) AssetPath(string, EncodePlan, string) (string, bool) { return "", false }

func (h *tuneHLS) StopChannel(channel string) { h.stopped = channel }
func (h *tuneHLS) StopAll()                   { h.stopped = "*" }

func TestOriginTuneHidesLiveDeliveryMechanisms(t *testing.T) {
	t.Parallel()
	sessions := &tuneSessions{}
	manifestPath := filepath.Join(t.TempDir(), "master.m3u8")
	if err := os.WriteFile(manifestPath, []byte("#EXTM3U\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hls := &tuneHLS{path: manifestPath}
	origin := newOrigin(nil, sessions, hls)

	stream, err := origin.Tune(context.Background(), TuneRequest{
		ChannelID: "ch-one", Plan: PlanFull, Delivery: DeliveryMPEGTS,
	})
	if err != nil || stream.Stream == nil || stream.Manifest != nil {
		t.Fatalf("MPEG-TS tune = (%#v, %v)", stream, err)
	}
	manifest, err := origin.Tune(context.Background(), TuneRequest{
		ChannelID: "ch-two", Plan: PlanBaseline, Delivery: DeliveryHLS,
	})
	if err != nil || manifest.Stream != nil || string(manifest.Manifest) != "#EXTM3U\n" {
		t.Fatalf("HLS tune = (%#v, %v)", manifest, err)
	}
	if sessions.channel != "ch-one" || hls.channel != "ch-two" {
		t.Fatalf("adapters saw sessions=%q hls=%q", sessions.channel, hls.channel)
	}
	origin.StopChannel("ch-one")
	if sessions.stopped != "ch-one" || hls.stopped != "ch-one" {
		t.Fatalf("stop adapters saw sessions=%q hls=%q", sessions.stopped, hls.stopped)
	}
}

func TestOriginLifecycleGateFailsClosedAndStopAllIsReusable(t *testing.T) {
	t.Parallel()
	available := false
	sessions := &tuneSessions{}
	hls := &tuneHLS{}
	origin := NewOrigin(OriginDependencies{
		LiveSessions: nil, LiveHLS: nil, Available: func() bool { return available },
	})
	// Use the internal constructor's test adapters so StopAll is observable without real ffmpeg.
	origin.sessions = sessions
	origin.hls = hls

	if _, err := origin.Tune(context.Background(), TuneRequest{
		ChannelID: "ch-one", Plan: PlanFull, Delivery: DeliveryMPEGTS,
	}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Tune while lifecycle unavailable error = %v, want ErrUnavailable", err)
	}
	if _, ok, err := origin.OpenAsset(context.Background(), "ch-one", PlanBaseline, "segment.ts"); !errors.Is(err, ErrUnavailable) || ok {
		t.Fatalf("OpenAsset while lifecycle unavailable = (_, %v, %v), want false, ErrUnavailable", ok, err)
	}
	origin.StopAll()
	if sessions.stopped != "*" || hls.stopped != "*" {
		t.Fatalf("StopAll adapters saw sessions=%q hls=%q", sessions.stopped, hls.stopped)
	}

	available = true
	stream, err := origin.Tune(context.Background(), TuneRequest{
		ChannelID: "ch-one", Plan: PlanFull, Delivery: DeliveryMPEGTS,
	})
	if err != nil || stream.Stream == nil {
		t.Fatalf("Tune after lifecycle recovery = (%#v, %v)", stream, err)
	}
}

type blockingTuneSessions struct {
	entered chan struct{}
	release chan struct{}
	stopped atomic.Bool
}

func (s *blockingTuneSessions) Attach(context.Context, string, EncodePlan) (<-chan []byte, func(), error) {
	close(s.entered)
	<-s.release
	return make(chan []byte), func() {}, nil
}

func (*blockingTuneSessions) StopChannel(string) {}
func (s *blockingTuneSessions) Stop()            { s.stopped.Store(true) }

func TestOriginStopAllOrdersAgainstTuneAdmission(t *testing.T) {
	t.Parallel()
	available := atomic.Bool{}
	available.Store(true)
	sessions := &blockingTuneSessions{entered: make(chan struct{}), release: make(chan struct{})}
	origin := newOrigin(nil, sessions, nil)
	origin.available = available.Load
	tuned := make(chan error, 1)
	go func() {
		_, err := origin.Tune(context.Background(), TuneRequest{
			ChannelID: "ch-one", Plan: PlanFull, Delivery: DeliveryMPEGTS,
		})
		tuned <- err
	}()
	<-sessions.entered
	available.Store(false)
	stopped := make(chan struct{})
	go func() {
		origin.StopAll()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("StopAll passed an in-flight attachment instead of ordering after it")
	case <-time.After(20 * time.Millisecond):
	}
	close(sessions.release)
	if err := <-tuned; err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("StopAll did not complete after attachment finished")
	}
	if !sessions.stopped.Load() {
		t.Fatal("session attached during admission close escaped StopAll")
	}
}
