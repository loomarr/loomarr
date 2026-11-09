package playoutresolve

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout"
)

type libraryVideoFake map[string]store.LibraryStream

func (f libraryVideoFake) LibraryPlayingVideo(_ context.Context, itemID string) (store.LibraryStream, bool, error) {
	v, ok := f[itemID]
	return v, ok, nil
}

// An HDR10 Matroska file whose Colour element carries only its matrix coefficients signals PQ in the HEVC
// parameter set alone, so ffprobe's stream summary reads its transfer as unknown and the airing
// skipped tone mapping (#2173). The scanner's header read takes the parameter set's transfer, so a
// library item's airtime format takes its colour transfer from there; any other id keeps the probe's.
func TestPlayoutResolver_FormatOfTakesALibraryItemsTransferFromItsHeader(t *testing.T) {
	r := &Resolver{
		libraryVideo: libraryVideoFake{
			"lib-hdr": {Kind: "video", Codec: "hevc", Width: 3840, Height: 2160, PixelFormat: "yuv420p10le",
				ColorTransfer: "smpte2084"},
			"lib-untagged": {Kind: "video", Codec: "hevc", Width: 3840, Height: 2160, PixelFormat: "yuv420p10le"},
		},
		probeFormat: func(context.Context, string) (playout.MediaFormat, error) {
			return playout.MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 2160, PixelFormat: "yuv420p10le",
				ColorTransfer: "unknown", Container: "matroska,webm"}, nil
		},
	}
	got := r.FormatOf(t.Context(), "lib-hdr", "/media/hdr.mkv")
	if !got.PQ() || got.VideoCodec != "hevc" || got.Container != "matroska,webm" {
		t.Errorf("library item format = %+v, want the probe's facts with the header's PQ transfer", got)
	}
	for _, id := range []string{"lib-untagged", "90000001", ""} {
		if got := r.FormatOf(t.Context(), id, "/media/hdr.mkv"); got.ColorTransfer != "unknown" {
			t.Errorf("item %q transfer = %q, want the probe's", id, got.ColorTransfer)
		}
	}
}

// A library item's format and HDR come only from its playing file's header as the scanner read it
// (#2151 step 16): no media server is asked, so any other id is unknown, which is not HDR.
func TestPlayoutResolver_LibraryItemFormatFromTheTables(t *testing.T) {
	r := &Resolver{libraryVideo: libraryVideoFake{
		"lib-hdr": {Kind: "video", Codec: "HEVC", Width: 3840, Height: 2160, FrameRate: "24000/1001",
			PixelFormat: "yuv420p10le", ColorTransfer: "smpte2084"},
		"lib-hlg": {Kind: "video", Codec: "hevc", Width: 3840, Height: 2160, ColorTransfer: "arib-std-b67"},
		"lib-sdr": {Kind: "video", Codec: "h264", Width: 1920, Height: 1080, ColorTransfer: "bt709"},
	}}
	if got, ok := r.libraryFormat(t.Context(), "lib-hdr"); !ok || got.VideoCodec != "hevc" || got.Height != 2160 || !got.HDR() {
		t.Errorf("lib-hdr format = %+v, %v; want hevc 2160 HDR", got, ok)
	}
	for id, want := range map[string]bool{"lib-hdr": true, "lib-hlg": true, "lib-sdr": false, "90000001": false} {
		if got := r.inventoryHDR(t.Context(), id); got != want {
			t.Errorf("%s HDR = %v, want %v", id, got, want)
		}
	}
	if got, ok := r.libraryFormat(t.Context(), "90000001"); ok || got != (playout.MediaFormat{}) {
		t.Errorf("a media-server id = %+v, %v; want unknown", got, ok)
	}
}
