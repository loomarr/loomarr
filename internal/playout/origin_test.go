package playout

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type tuneSessions struct{ channel string }

func (s *tuneSessions) Attach(_ context.Context, channel string, _ EncodePlan) (<-chan []byte, func(), error) {
	s.channel = channel
	return make(chan []byte), func() {}, nil
}

func TestOriginTuneReportsUnavailableDelivery(t *testing.T) {
	t.Parallel()
	origin := newOrigin(nil, &tuneSessions{}, nil)
	_, err := origin.Tune(context.Background(), TuneRequest{
		ChannelID: "ch-one", Plan: PlanBaseline, Delivery: DeliveryHLS,
	})
	if !errors.Is(err, ErrUnsupportedDelivery) {
		t.Fatalf("Tune error = %v, want ErrUnsupportedDelivery", err)
	}
	if _, ok, err := origin.OpenAsset("ch-one", PlanBaseline, "segment.ts"); err != nil || ok {
		t.Fatal("asset resolved without an HLS delivery")
	}
}

type tuneHLS struct {
	channel string
	path    string
}

func (h *tuneHLS) Playlist(channel string, _ EncodePlan) (string, func(), error) {
	h.channel = channel
	return h.path, func() {}, nil
}

func (h *tuneHLS) AssetPath(string, EncodePlan, string) (string, bool) { return "", false }

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
}
