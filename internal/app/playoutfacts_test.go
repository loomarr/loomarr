package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/testkit"
)

const (
	factsItemH264 = `{"Id":"item-1","Type":"Movie","DateLastSaved":"2026-09-04T12:00:00Z",
		"MediaSources":[{"Id":"source-1","ETag":"rev-1","Container":"mkv","MediaStreams":[
			{"Index":0,"Type":"Video","Codec":"h264","Width":1920,"Height":1080}]}]}`
	factsItemHEVC = `{"Id":"item-1","Type":"Movie","DateLastSaved":"2026-10-01T12:00:00Z",
		"MediaSources":[{"Id":"source-1","ETag":"rev-2","Container":"mkv","MediaStreams":[
			{"Index":0,"Type":"Video","Codec":"hevc","Width":3840,"Height":2160}]}]}`
)

// A library file that is replaced keeps its item id; the stream facts the lineup derives the
// channel format from must follow the file, not the first observation. Within the freshness window
// the stored facts answer without touching the library.
func TestPlayoutResolver_LineupFormatRefreshesWhenLibraryFileChanges(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	server := testkit.NewMediaServer(t)
	server.InventoryItems = map[string]json.RawMessage{"item-1": json.RawMessage(factsItemH264)}
	now := time.Now()
	r := &playoutResolver{
		lib: newTestLibraryClient(server), inventory: inventory.New(st), now: func() time.Time { return now },
	}

	if got := r.inventoryFormat(t.Context(), "item-1"); got.VideoCodec != "h264" || got.Height != 1080 {
		t.Fatalf("cold format = %+v, want h264 1080", got)
	}
	firstRequests := len(server.Requests())

	server.InventoryItems = map[string]json.RawMessage{"item-1": json.RawMessage(factsItemHEVC)}
	now = now.Add(time.Hour)
	if got := r.inventoryFormat(t.Context(), "item-1"); got.VideoCodec != "h264" {
		t.Fatalf("fresh format = %+v, want the stored h264 facts", got)
	}
	if got := len(server.Requests()); got != firstRequests {
		t.Fatalf("fresh read hit the library: requests %d→%d", firstRequests, got)
	}

	now = now.Add(inventoryFreshness)
	if got := r.inventoryFormat(t.Context(), "item-1"); got.VideoCodec != "hevc" || got.Height != 2160 {
		t.Fatalf("format after the file changed = %+v, want hevc 2160", got)
	}
}

// Replacing the file also retires what Loomarr measured from the old bytes: the measurement is
// filed under the old revision and the next import of the new revision drops it.
func TestPlayoutResolver_LibraryFileChangeRetiresItsMeasurement(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	server := testkit.NewMediaServer(t)
	server.InventoryItems = map[string]json.RawMessage{"item-1": json.RawMessage(factsItemH264)}
	now := time.Now()
	r := &playoutResolver{
		lib: newTestLibraryClient(server), inventory: inventory.New(st), now: func() time.Time { return now },
	}
	if got := r.inventoryFormat(t.Context(), "item-1"); got.VideoCodec != "h264" {
		t.Fatalf("cold format = %+v", got)
	}
	origin, err := r.lib.InventoryOrigin("item-1")
	if err != nil {
		t.Fatal(err)
	}
	item, ok, err := r.inventory.Item(t.Context(), inventory.ItemRef{Origin: &origin})
	if err != nil || !ok || len(item.Sources) != 1 {
		t.Fatalf("cold item = (%+v, %v, %v)", item, ok, err)
	}
	if err := r.inventory.RecordMeasurement(t.Context(), inventory.Measurement{
		SourceID: item.Sources[0].ID, Revision: item.Sources[0].Revision,
		Observation: inventory.Observation[inventory.SourceFacts]{
			SchemaVersion: 1, ObservedAt: now,
			Coverage: map[string]inventory.Coverage{"streams": inventory.CoveragePresent},
			Facts: inventory.SourceFacts{Streams: []inventory.Stream{
				{Index: 0, Kind: inventory.StreamVideo, Codec: "h264"},
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	server.InventoryItems = map[string]json.RawMessage{"item-1": json.RawMessage(factsItemHEVC)}
	now = now.Add(inventoryFreshness + time.Minute)
	if got := r.inventoryFormat(t.Context(), "item-1"); got.VideoCodec != "hevc" {
		t.Fatalf("format after the file changed = %+v, want hevc", got)
	}
	item, ok, err = r.inventory.Item(t.Context(), inventory.ItemRef{Origin: &origin})
	if err != nil || !ok || len(item.Sources) != 1 || item.Sources[0].Revision != "etag:rev-2" ||
		item.Sources[0].Measurement != nil {
		t.Fatalf("stored item = (%+v, %v, %v), want one source at etag:rev-2 with no measurement", item, ok, err)
	}
}

// An unchanged file past the freshness window is revalidated by one import and keeps its facts.
func TestPlayoutResolver_LineupFormatUnchangedFileKeepsFacts(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	server := testkit.NewMediaServer(t)
	server.InventoryItems = map[string]json.RawMessage{"item-1": json.RawMessage(factsItemH264)}
	now := time.Now()
	r := &playoutResolver{
		lib: newTestLibraryClient(server), inventory: inventory.New(st), now: func() time.Time { return now },
	}
	r.inventoryFormat(t.Context(), "item-1")
	firstRequests := len(server.Requests())

	now = now.Add(inventoryFreshness + time.Minute)
	if got := r.inventoryFormat(t.Context(), "item-1"); got.VideoCodec != "h264" || got.Height != 1080 {
		t.Fatalf("revalidated format = %+v, want h264 1080", got)
	}
	if got := len(server.Requests()); got != firstRequests+1 {
		t.Fatalf("revalidation requests %d→%d, want exactly one import", firstRequests, got)
	}
	// The import stamps the wall clock, so the next read is judged against it.
	now = time.Now()
	r.inventoryFormat(t.Context(), "item-1")
	if got := len(server.Requests()); got != firstRequests+1 {
		t.Fatalf("read after revalidation hit the library again: %d", got)
	}
}

func TestPlayoutResolver_InventoryHDRFollowsTheLibraryTransfer(t *testing.T) {
	hdr10 := `{"Id":"item-1","Type":"Movie","MediaSources":[{"Id":"s","ETag":"a","MediaStreams":[
		{"Index":0,"Type":"Video","Codec":"hevc","ColorTransfer":"smpte2084","ColorPrimaries":"bt2020"}]}]}`
	hlg := `{"Id":"item-1","Type":"Movie","MediaSources":[{"Id":"s","ETag":"a","MediaStreams":[
		{"Index":0,"Type":"Video","Codec":"hevc","ColorTransfer":"arib-std-b67","ColorPrimaries":"bt2020"}]}]}`
	sdr := `{"Id":"item-1","Type":"Movie","MediaSources":[{"Id":"s","ETag":"a","MediaStreams":[
		{"Index":0,"Type":"Video","Codec":"h264","ColorTransfer":"bt709","ColorPrimaries":"bt709"}]}]}`
	for name, tc := range map[string]struct {
		item string
		want bool
	}{"hdr10": {hdr10, true}, "hlg": {hlg, true}, "sdr": {sdr, false}} {
		t.Run(name, func(t *testing.T) {
			server := testkit.NewMediaServer(t)
			server.InventoryItems = map[string]json.RawMessage{"item-1": json.RawMessage(tc.item)}
			r := &playoutResolver{
				lib: newTestLibraryClient(server), inventory: inventory.New(testkit.MigratedSQLiteStore(t)),
				now: time.Now,
			}
			if got := r.inventoryHDR(t.Context(), "item-1"); got != tc.want {
				t.Fatalf("inventoryHDR = %v, want %v", got, tc.want)
			}
		})
	}
	// An item the inventory and library both lack is unknown, which is not HDR.
	r := &playoutResolver{
		lib: newTestLibraryClient(testkit.NewMediaServer(t)), inventory: inventory.New(testkit.MigratedSQLiteStore(t)),
		now: time.Now,
	}
	if r.inventoryHDR(t.Context(), "missing") {
		t.Fatal("unknown item reported HDR")
	}
}

type libraryVideoFake map[string]store.LibraryStream

func (f libraryVideoFake) LibraryPlayingVideo(_ context.Context, itemID string) (store.LibraryStream, bool, error) {
	v, ok := f[itemID]
	return v, ok, nil
}

// An HDR10 Matroska file whose Colour element carries only the matrix signals PQ in the HEVC
// parameter set alone, so ffprobe's stream summary reads its transfer as unknown and the airing
// skipped tone mapping (#2173). The scanner's header read takes the parameter set's transfer, so a
// library item's airtime format takes its colour transfer from there; any other id keeps the probe's.
func TestPlayoutResolver_FormatOfTakesALibraryItemsTransferFromItsHeader(t *testing.T) {
	r := &playoutResolver{
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

// A library item's format and HDR come from its playing file's header as the scanner read it, with
// no media server or inventory; any other id is unknown without them.
func TestPlayoutResolver_LibraryItemFormatFromTheTables(t *testing.T) {
	r := &playoutResolver{libraryVideo: libraryVideoFake{
		"lib-hdr": {Kind: "video", Codec: "HEVC", Width: 3840, Height: 2160, FrameRate: "24000/1001",
			PixelFormat: "yuv420p10le", ColorTransfer: "smpte2084"},
		"lib-sdr": {Kind: "video", Codec: "h264", Width: 1920, Height: 1080, ColorTransfer: "bt709"},
	}}
	if got := r.inventoryFormat(t.Context(), "lib-hdr"); got.VideoCodec != "hevc" || got.Height != 2160 || !got.HDR() {
		t.Errorf("lib-hdr format = %+v, want hevc 2160 HDR", got)
	}
	if !r.inventoryHDR(t.Context(), "lib-hdr") || r.inventoryHDR(t.Context(), "lib-sdr") {
		t.Error("HDR should follow the playing file's color transfer")
	}
	if got := r.inventoryFormat(t.Context(), "90000001"); got != (playout.MediaFormat{}) {
		t.Errorf("a media-server id with no inventory = %+v, want unknown", got)
	}
}
