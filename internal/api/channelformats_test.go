package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/playout"
)

type channelFormatsBody struct {
	Baseline struct {
		Class, Codec, DynamicRange string
		Width, Height              int
	} `json:"baseline"`
	Premium *struct {
		Class, Codec, DynamicRange string
		Width, Height              int
	} `json:"premium"`
	LineupPremium  string `json:"lineupPremium"`
	PremiumDropped string `json:"premiumDropped"`
	Titles         int    `json:"titles"`
	MeasuredTitles int    `json:"measuredTitles"`
	UHDTitles      int    `json:"uhdTitles"`
	HDRTitles      int    `json:"hdrTitles"`
}

func getChannelFormats(t *testing.T, encoder playout.Encoder, gpu playout.GPUFilters, lineup []playout.MediaFormat, channel string) (int, channelFormatsBody) {
	t.Helper()
	resolver := &fakeResolver{lineup: lineup, profile: playout.DefaultProfile()}
	resolver.profile.Encoder = encoder
	harness := startAPIHarness(t, func(d apiHarnessDefaults) http.Handler {
		return api.Router(d.Log, api.Options{
			Store: d.Store, Auth: api.NewTokenAuthorizer(adminToken), Log: d.Log,
			PlayoutResolver:   resolver,
			PlayoutTonemap:    func() bool { return true },
			PlayoutGPUTonemap: func() playout.GPUFilters { return gpu },
		})
	})
	seedChannel(t, harness.Store, "ch1", "Channel One", 1, "internal")
	resp := do(t, harness.Server, http.MethodGet, "/v1/channels/"+channel+"/formats", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	var body channelFormatsBody
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode, body
}

// TestChannelFormats_DerivedFromLineupAndGatedByHost: a lineup with a 1080p HDR title and a 4K SDR
// title warrants 4K HDR (independent axes); an NVENC host with libplacebo airs it, a software host
// drops it and says why. The baseline is always there.
func TestChannelFormats_DerivedFromLineupAndGatedByHost(t *testing.T) {
	lineup := []playout.MediaFormat{
		{VideoCodec: "hevc", Width: 1920, Height: 1080, ColorTransfer: "smpte2084"},
		{VideoCodec: "hevc", Width: 3840, Height: 2160, ColorTransfer: "bt709"},
		{VideoCodec: "h264", Width: 1920, Height: 1080},
		{}, // not yet measured
	}
	status, got := getChannelFormats(t, playout.EncoderNVENC, playout.GPUFilters{Libplacebo: true}, lineup, "ch1")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if got.Baseline.Class != "1080p-h264-sdr" || got.Baseline.Codec != "h264" || got.Baseline.Height != 1080 || got.Baseline.DynamicRange != "sdr" {
		t.Errorf("baseline = %+v", got.Baseline)
	}
	if got.Premium == nil || got.Premium.Class != "4k-hevc-hdr" || got.Premium.Codec != "hevc" ||
		got.Premium.Width != 3840 || got.Premium.DynamicRange != "hdr10" {
		t.Errorf("premium = %+v, want 4k-hevc-hdr hevc 3840 hdr10", got.Premium)
	}
	if got.LineupPremium != "4k-hevc-hdr" || got.PremiumDropped != "" {
		t.Errorf("lineupPremium %q, dropped %q", got.LineupPremium, got.PremiumDropped)
	}
	if got.Titles != 4 || got.MeasuredTitles != 3 || got.UHDTitles != 1 || got.HDRTitles != 1 {
		t.Errorf("counts = %d/%d/%d/%d, want 4/3/1/1", got.Titles, got.MeasuredTitles, got.UHDTitles, got.HDRTitles)
	}

	_, sw := getChannelFormats(t, playout.EncoderSoftware, playout.GPUFilters{Libplacebo: true}, lineup, "ch1")
	if sw.Premium != nil || sw.LineupPremium != "4k-hevc-hdr" || sw.PremiumDropped == "" {
		t.Errorf("software host: premium %+v, lineup %q, dropped %q; want none, 4k-hevc-hdr, a reason", sw.Premium, sw.LineupPremium, sw.PremiumDropped)
	}
}

func TestChannelFormats_UnmeasuredLineupIsBaselineOnly(t *testing.T) {
	status, got := getChannelFormats(t, playout.EncoderNVENC, playout.GPUFilters{Libplacebo: true}, []playout.MediaFormat{{}, {}}, "ch1")
	if status != http.StatusOK || got.Premium != nil || got.LineupPremium != "" || got.Baseline.Class != "1080p-h264-sdr" {
		t.Errorf("status %d, body %+v: want baseline only", status, got)
	}
}

func TestChannelFormats_UnknownChannelIs404(t *testing.T) {
	if status, _ := getChannelFormats(t, playout.EncoderNVENC, playout.GPUFilters{}, nil, "nope"); status != http.StatusNotFound {
		t.Errorf("status %d, want 404", status)
	}
}
