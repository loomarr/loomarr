package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

type modeProbePlayout struct {
	tuned        bool
	request      playout.TuneRequest
	presentation playout.Presentation
	err          error
	asset        playout.Asset
	assetOK      bool
}

func modeHandlerServer(t *testing.T, probe *modeProbePlayout) *Server {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/mode.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ch := store.Channel{Channel: schedule.Channel{
		ID: "ch-one", Name: "Channel One", Number: 1, Status: schedule.StatusLive,
	}}
	ch.Policy.Playout = &schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal}
	if _, err := st.SaveChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	return &Server{playout: probe, store: st}
}

func (p *modeProbePlayout) Tune(_ context.Context, request playout.TuneRequest) (playout.Presentation, error) {
	p.tuned, p.request = true, request
	return p.presentation, p.err
}

func (p *modeProbePlayout) OpenAsset(context.Context, string, playout.EncodePlan, string) (playout.Asset, bool, error) {
	return p.asset, p.assetOK, nil
}

func (*modeProbePlayout) StopChannel(string) {}

func liveManifest() playout.Presentation {
	return playout.Presentation{Manifest: []byte("#EXTM3U\n#EXTINF:4,\nseg-0.ts\n"), Release: func() {}}
}

// Prepared media is retired (#1512 phase 4), but a client built before that still probes adjacent
// channels with mode=prepared. The answer stays "no prepared presentation" and the channel is never
// tuned: a live start there would run an encoder for a channel nobody watches (G1).
func TestHLSRetiredPreparedModeNeverStartsPlayout(t *testing.T) {
	probe := &modeProbePlayout{presentation: liveManifest()}
	s := modeHandlerServer(t, probe)
	req := httptest.NewRequest(http.MethodGet, "/v1/playout/hls/ch-one/master.m3u8?mode=prepared", nil)
	req.SetPathValue("id", "ch-one")
	w := httptest.NewRecorder()

	s.hlsPlaylistHandler(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if probe.tuned {
		t.Fatalf("mode=prepared tuned the channel: %#v", probe.request)
	}
}

func TestHLSWarmModeMarksSpeculativeLiveAdmission(t *testing.T) {
	probe := &modeProbePlayout{presentation: liveManifest()}
	s := modeHandlerServer(t, probe)
	req := httptest.NewRequest(http.MethodGet, "/v1/playout/hls/ch-one/master.m3u8?mode=warm&sig=signed&plan=hevc8", nil)
	req.SetPathValue("id", "ch-one")
	w := httptest.NewRecorder()

	s.hlsPlaylistHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !probe.request.Speculative || probe.request.Delivery != playout.DeliveryHLS {
		t.Fatalf("Tune request = %#v, want speculative live HLS", probe.request)
	}
	body := w.Body.String()
	if strings.Contains(body, "mode=warm") {
		t.Fatalf("warm hint leaked into asset URL: %q", body)
	}
	if !strings.Contains(body, "plan=hevc8") || !strings.Contains(body, "sig=signed") {
		t.Fatalf("asset URL lost auth or rendition selectors: %q", body)
	}
}

func TestHLSAssetQueryDropsOnlyTheMasterMode(t *testing.T) {
	got := hlsAssetQuery(url.Values{
		"mode": {"warm"}, "sig": {"signed"}, "quality": {"720"},
	})
	if got != "quality=720&sig=signed" {
		t.Fatalf("hlsAssetQuery() = %q", got)
	}
}

type stringAsset struct{ *strings.Reader }

func (stringAsset) Close() error { return nil }

// A packager variant playlist (#1512 phase 2b) is fetched as an asset of the master. Its URIs are
// bare, so without the rewrite a native player's init and segment fetches carry no credential.
func TestHLSVariantPlaylistAssetIsAuthRewritten(t *testing.T) {
	body := "#EXTM3U\n#EXT-X-MAP:URI=\"1080p-h264-sdr-init.mp4\"\n#EXTINF:1.00000,\n1080p-h264-sdr-seg00000001.m4s\n"
	probe := &modeProbePlayout{asset: playout.Asset{
		Content: stringAsset{strings.NewReader(body)}, Modified: time.Unix(1_000, 0), Playlist: true,
	}, assetOK: true}
	s := &Server{playout: probe}
	req := httptest.NewRequest(http.MethodGet, "/v1/playout/hls/ch-one/1080p-h264-sdr.m3u8?sig=abc", nil)
	req.SetPathValue("id", "ch-one")
	req.SetPathValue("asset", "1080p-h264-sdr.m3u8")
	w := httptest.NewRecorder()

	s.hlsAssetHandler(w, req)

	got := w.Body.String()
	for _, want := range []string{`URI="1080p-h264-sdr-init.mp4?sig=abc"`, "\n1080p-h264-sdr-seg00000001.m4s?sig=abc\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("playlist lacks %q:\n%s", want, got)
		}
	}
	if ct, cc := w.Header().Get("Content-Type"), w.Header().Get("Cache-Control"); ct != "application/vnd.apple.mpegurl" || cc != "no-store" {
		t.Fatalf("Content-Type %q, Cache-Control %q", ct, cc)
	}
}
