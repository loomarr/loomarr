package playout

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildVideoToolbox_UnavailableDeinterlacer(t *testing.T) {
	src := testSources()["h264-1080p-sdr-29.97"]
	src.Interlaced = true
	p, err := Build(HostFor(EncoderVideoToolbox, true, GPUFilters{}), src, testOutput)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.VideoFilter, "yadif_videotoolbox") {
		t.Fatalf("build without VideoToolbox deinterlacing emitted an unavailable filter: %s", p.VideoFilter)
	}
	if slices.Contains(p.PreInput, "videotoolbox_vld") || !strings.HasPrefix(p.VideoFilter, "bwdif=mode=send_frame,scale=") {
		t.Fatalf("expected CPU deinterlacing before scaling: %v %s", p.PreInput, p.VideoFilter)
	}
	if !strings.Contains(strings.Join(p.Fallbacks, ";"), "deinterlace:") {
		t.Fatalf("CPU deinterlacing must be a declared fallback: %v", p.Fallbacks)
	}
	if !slices.Contains(p.VideoEncode, "h264_videotoolbox") {
		t.Fatalf("deinterlacing fallback changed the encoder: %v", p.VideoEncode)
	}
}

func TestBuildVideoToolbox_DownloadTenBitBeforeSDRConversion(t *testing.T) {
	p, err := Build(testHosts()["videotoolbox"], testSources()["hevc10-1080p"], testOutput)
	if err != nil {
		t.Fatal(err)
	}
	// hwdownload transfers the decoded P010 surface; only a subsequent format filter may
	// convert it to the baseline's NV12. scale_vt preserves the decoded software format.
	if !strings.Contains(p.VideoFilter, "hwdownload,format=p010le,format=nv12") {
		t.Fatalf("10-bit download negotiated the output format instead of the decoded surface: %s", p.VideoFilter)
	}
}

func TestBuildVideoToolbox_AvailableDeinterlacer(t *testing.T) {
	src := testSources()["h264-1080p-sdr-29.97"]
	src.Interlaced = true
	host := HostFor(EncoderVideoToolbox, true, GPUFilters{VideoToolboxDeinterlace: true})
	p, err := Build(host, src, testOutput)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(p.PreInput, "videotoolbox_vld") ||
		!strings.HasPrefix(p.VideoFilter, "yadif_videotoolbox=mode=send_frame,scale_vt=") {
		t.Fatalf("available GPU deinterlacing must precede scaling at the channel cadence: %v %s", p.PreInput, p.VideoFilter)
	}
	if len(p.Fallbacks) != 0 {
		t.Fatalf("available GPU deinterlacing unexpectedly fell back: %v", p.Fallbacks)
	}
}
