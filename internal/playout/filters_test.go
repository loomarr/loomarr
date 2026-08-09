package playout

import (
	"strings"
	"testing"
)

// `-filters` output, shaped exactly as ffmpeg prints it: flags, name, signature, description.
//
// The three tonemap rows are the reason this fixture exists. `tonemap`, `tonemap_opencl` and
// `tonemap_vaapi` are three DIFFERENT filters with different requirements, and only the plain one
// works in the CPU chain playout builds. A substring match would accept a build that has just the
// OpenCL variant and then fail at graph-init — the exact "filter not found kills the channel"
// class this probe exists to prevent.
const filtersListing = `Filters:
  T.. adrawgraph         A->V       Draw a graph using input audio metadata.
  ... anullsrc           |->A       Null audio source, return empty audio frames.
  TS. drawtext           V->V       Draw text on top of video frames.
  ... hwupload           V->V       Upload a normal frame to a hardware frame.
  ..C tonemap_opencl     V->V       Perform HDR to SDR conversion with tonemapping (OpenCL).
  ... tonemap_vaapi      V->V       VAAPI VPP for tone-mapping.
  .S. tonemap            V->V       Conversion to/from different dynamic ranges.
  .S. zscale             V->V       Apply resizing, colorspace and bit depth conversion.
`

func TestParseHasFilter_MatchesTheNameColumnExactly(t *testing.T) {
	raw := []byte(filtersListing)

	for _, name := range []string{"drawtext", "tonemap", "zscale", "hwupload", "anullsrc"} {
		if !parseHasFilter(raw, name) {
			t.Errorf("%s is in the listing but was not found", name)
		}
	}

	// Present in the listing only as a DESCRIPTION word or as part of a longer filter name.
	for _, name := range []string{"tonemapping", "HDR", "graph", "upload", "scale"} {
		if parseHasFilter(raw, name) {
			t.Errorf("%q is not a filter name in the listing but matched — a substring search "+
				"would accept a build that cannot run the chain", name)
		}
	}
}

// A build with ONLY the vendor tone-map variants must read as "cannot tone-map".
//
// This is the asymmetry that makes exact matching load-bearing rather than tidy: `tonemap_vaapi`
// needs VAAPI hardware frames, and playout's chain runs on CPU frames (capability.go refuses
// -hwaccel_output_format), so accepting it would emit a filter that cannot take the input it is
// given.
func TestParseHasFilter_VendorVariantsAreNotThePlainFilter(t *testing.T) {
	vendorOnly := []byte(`Filters:
  ..C tonemap_opencl     V->V       Perform HDR to SDR conversion with tonemapping (OpenCL).
  ... tonemap_vaapi      V->V       VAAPI VPP for tone-mapping.
`)
	if parseHasFilter(vendorOnly, "tonemap") {
		t.Error("a build with only tonemap_opencl/tonemap_vaapi reported the plain tonemap filter")
	}
}

// An empty or garbage listing must read as "no filters", never as an assumed yes — the probe's
// failure direction is what keeps a channel alive on a build we could not interrogate.
func TestParseHasFilter_UnreadableListingIsNo(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte(""), []byte("\n\n"), []byte("not filter output at all")} {
		if parseHasFilter(raw, "zscale") {
			t.Errorf("unreadable listing %q reported a filter as present", raw)
		}
	}
}

// THE TONE-MAP CHAIN'S SHAPE, asserted so a future edit cannot quietly break the colour pipeline.
//
// The three steps are order-dependent in a way that produces no error if they are wrong — just a
// worse picture — so nothing but an assertion protects it.
func TestHDRToSDRChain_ConvertsThroughLinearAndBackToBT709(t *testing.T) {
	iLinear := strings.Index(hdrToSDRChain, "zscale=t=linear")
	iTonemap := strings.Index(hdrToSDRChain, "tonemap=tonemap=")
	iBT709 := strings.Index(hdrToSDRChain, "zscale=p=bt709")

	if iLinear < 0 || iTonemap < 0 || iBT709 < 0 {
		t.Fatalf("chain is missing one of its three steps: %q", hdrToSDRChain)
	}
	if iLinear >= iTonemap || iTonemap >= iBT709 {
		t.Errorf("steps out of order (linear=%d tonemap=%d bt709=%d): tone-mapping anything but "+
			"linear light compresses the wrong quantity, and not converting back leaves the "+
			"encoder linear data: %q", iLinear, iTonemap, iBT709, hdrToSDRChain)
	}
	// desat=0 is deliberate: ffmpeg's default desaturation washes skin tones out, and a flat
	// picture is the complaint this whole change exists to fix.
	if !strings.Contains(hdrToSDRChain, "desat=0") {
		t.Errorf("lost desat=0 — the default desaturates and undoes the point: %q", hdrToSDRChain)
	}
	// No pixel format here: the chain must leave bit-depth reduction to the format/upload step
	// that follows, or a 10-bit source is truncated BEFORE it is tone-mapped.
	if strings.Contains(hdrToSDRChain, "format=") {
		t.Errorf("chain pins a pixel format; that belongs to the step after it: %q", hdrToSDRChain)
	}
}

// The chain's LAST step is what relabels the output, so it must name all three colour axes plus
// the range.
//
// ⚠ This replaced an assertion on four `-colorspace`/`-color_trc`/… OUTPUT flags that an earlier
// draft emitted. Measured against a real HDR10 source, this chain alone yields
// `yuv420p,tv,bt709,bt709,bt709` — identical to the same run with all four flags — so the flags
// were doing nothing, and the live test passed with them sabotaged out. They were removed rather
// than kept "for clarity": a redundant assertion about the pixels is exactly the write-only code
// this change exists to delete.
func TestHDRToSDRChain_LastStepRelabelsEveryColorAxis(t *testing.T) {
	last := hdrToSDRChain[strings.LastIndex(hdrToSDRChain, "zscale="):]
	for _, want := range []string{"p=bt709", "t=bt709", "m=bt709", "r=tv"} {
		if !strings.Contains(last, want) {
			t.Errorf("final zscale is missing %q — the output would keep the source's HDR tags "+
				"even though the pixels are SDR: %q", want, last)
		}
	}
}
