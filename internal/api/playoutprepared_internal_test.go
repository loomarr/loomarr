package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

type preparedProbePlayout struct {
	request      playout.TuneRequest
	presentation playout.Presentation
	err          error
	asset        playout.Asset
	assetOK      bool
}

func preparedHandlerServer(t *testing.T, probe *preparedProbePlayout) *Server {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/prepared.db", true)
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

func (p *preparedProbePlayout) Tune(_ context.Context, request playout.TuneRequest) (playout.Presentation, error) {
	p.request = request
	return p.presentation, p.err
}

func (p *preparedProbePlayout) OpenAsset(context.Context, string, playout.EncodePlan, string) (playout.Asset, bool, error) {
	return p.asset, p.assetOK, nil
}

func (*preparedProbePlayout) AcquireAdmission(ctx context.Context, _ string) (playout.Admission, error) {
	return playout.Admission{Context: ctx}, nil
}

func (*preparedProbePlayout) StopChannel(string) {}

func TestHLSPreparedModeReturnsNoContentWithoutLiveFallback(t *testing.T) {
	probe := &preparedProbePlayout{err: playout.ErrPreparedUnavailable}
	s := preparedHandlerServer(t, probe)
	req := httptest.NewRequest(http.MethodGet, "/v1/playout/hls/ch-one/master.m3u8?mode=prepared", nil)
	req.SetPathValue("id", "ch-one")
	w := httptest.NewRecorder()

	s.hlsPlaylistHandler(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if !probe.request.PreparedOnly || probe.request.Delivery != playout.DeliveryHLS {
		t.Fatalf("Tune request = %#v, want prepared-only HLS", probe.request)
	}
}

func TestHLSWarmModeMarksSpeculativeLiveAdmission(t *testing.T) {
	probe := &preparedProbePlayout{presentation: playout.Presentation{
		Manifest: []byte("#EXTM3U\n#EXTINF:4,\nseg-0.ts\n"), Release: func() {},
	}}
	s := preparedHandlerServer(t, probe)
	req := httptest.NewRequest(http.MethodGet, "/v1/playout/hls/ch-one/master.m3u8?mode=warm", nil)
	req.SetPathValue("id", "ch-one")
	w := httptest.NewRecorder()

	s.hlsPlaylistHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !probe.request.Speculative || probe.request.PreparedOnly || probe.request.Delivery != playout.DeliveryHLS {
		t.Fatalf("Tune request = %#v, want speculative live HLS", probe.request)
	}
	if strings.Contains(w.Body.String(), "mode=warm") {
		t.Fatalf("warm hint leaked into asset URL: %q", w.Body.String())
	}
}

func TestHLSPreparedModeIsNotCopiedOntoImmutableAssetURLs(t *testing.T) {
	probe := &preparedProbePlayout{presentation: playout.Presentation{
		Manifest: []byte("#EXTM3U\np-publication-segment\n"), Release: func() {},
	}}
	s := preparedHandlerServer(t, probe)
	req := httptest.NewRequest(http.MethodGet,
		"/v1/playout/hls/ch-one/master.m3u8?mode=prepared&sig=signed&plan=hevc8", nil)
	req.SetPathValue("id", "ch-one")
	w := httptest.NewRecorder()

	s.hlsPlaylistHandler(w, req)

	body := w.Body.String()
	if strings.Contains(body, "mode=prepared") {
		t.Fatalf("prepared-only hint leaked into asset URL: %q", body)
	}
	if !strings.Contains(body, "plan=hevc8") || !strings.Contains(body, "sig=signed") {
		t.Fatalf("asset URL lost auth or rendition selectors: %q", body)
	}
}

func TestHLSPreparedAssetsArePrivateImmutable(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "prepared-*.m4s")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("prepared bytes"); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	probe := &preparedProbePlayout{asset: playout.Asset{
		Content: file, Modified: time.Unix(1_000, 0), Immutable: true,
	}, assetOK: true}
	s := &Server{playout: probe}
	req := httptest.NewRequest(http.MethodGet, "/v1/playout/hls/ch-one/p-publication-segment", nil)
	req.SetPathValue("id", "ch-one")
	req.SetPathValue("asset", "p-publication-segment")
	w := httptest.NewRecorder()

	s.hlsAssetHandler(w, req)

	if got := w.Header().Get("Cache-Control"); got != "private, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestHLSAssetQueryDropsOnlyTheMasterMode(t *testing.T) {
	got := hlsAssetQuery(url.Values{
		"mode": {"prepared"}, "sig": {"signed"}, "quality": {"720"},
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
	probe := &preparedProbePlayout{asset: playout.Asset{
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
