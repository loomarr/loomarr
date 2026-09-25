package clipfetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/storagegovernor"
)

// memSink is an in-memory fileSink for testing the walk without touching disk.
type memSink struct {
	files map[string][]byte
}

func newMemSink() *memSink { return &memSink{files: map[string][]byte{}} }

func (m *memSink) Exists(path string) bool { _, ok := m.files[path]; return ok }
func (m *memSink) WriteStream(_ context.Context, path string, r io.Reader, _ *writeGuard) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.files[path] = b
	return nil
}
func (m *memSink) WriteFile(path string, data []byte) error { m.files[path] = data; return nil }
func (m *memSink) Inspect(path string) (string, int64, string, error) {
	return inspectBytes(m.files[path])
}

// mockArchive serves the pinned metadata/search/download shapes.
func mockArchive(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// A single item: two video files (a big original + a small derivative) + a thumbnail.
	itemMeta := metadataResp{
		Server: "SELF", // replaced with the test server host at request time
		Dir:    "/0/items/test-ad",
		Metadata: archiveMetadata{
			MediaType: "movies", Title: "Test 90s Cereal Ad",
			Description: "A 1994 cereal commercial.",
		},
		Files: []archiveFile{
			{Name: "big.mp4", Format: "MPEG4", Size: "246000000", Source: "original"},
			{Name: "small.ia.mp4", Format: "h.264 IA", Size: "9000000", Source: "derivative"},
			{Name: "thumb.jpg", Format: "Thumbnail", Size: "12000", Source: "derivative"},
		},
	}

	mux.HandleFunc("/metadata/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/metadata/")
		switch id {
		case "test-ad":
			m := itemMeta
			m.Server = r.Host // download URL points back at this test server
			_ = json.NewEncoder(w).Encode(m)
		case "test-collection":
			_ = json.NewEncoder(w).Encode(metadataResp{
				Metadata: archiveMetadata{MediaType: "collection", Title: "Test Collection"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("/advancedsearch.php", func(w http.ResponseWriter, r *http.Request) {
		// The collection has one member item: test-ad.
		var sr searchResp
		sr.Response.NumFound = 1
		sr.Response.Docs = []searchDoc{{Identifier: "test-ad"}}
		_ = json.NewEncoder(w).Encode(sr)
	})
	// The download endpoint: /0/items/test-ad/<file> → some bytes.
	mux.HandleFunc("/0/items/test-ad/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake video bytes"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(t *testing.T, fs fileSink) *archiveClient {
	srv := mockArchive(t)
	return newArchiveClient(srv.URL, srv.Client(), fs)
}

func TestArchive_DownloadsItemAndSidecar(t *testing.T) {
	fs := newMemSink()
	c := newTestClient(t, fs)

	fetched, skipped, _, err := c.walk(context.Background(), "https://archive.org/details/test-ad", "/drop")
	if err != nil {
		t.Fatal(err)
	}
	if fetched != 1 || skipped != 0 {
		t.Fatalf("walk = fetched %d skipped %d, want 1/0", fetched, skipped)
	}

	// The source representation is chosen for retained evidence, not minimum download size.
	var mediaPath, sidecarPath string
	for p := range fs.files {
		if strings.HasSuffix(p, ".info.json") {
			sidecarPath = p
		} else {
			mediaPath = p
		}
	}
	if !strings.Contains(mediaPath, "big.mp4") {
		t.Errorf("should download the source original, got %q", mediaPath)
	}
	if strings.Contains(mediaPath, "small.ia.mp4") {
		t.Error("downloaded the playback-sized derivative instead of the source original")
	}
	// The sidecar preserves title/description (AI-tagging text signals, §10).
	if sidecarPath == "" {
		t.Fatal("no info-JSON sidecar written")
	}
	var sc map[string]any
	_ = json.Unmarshal(fs.files[sidecarPath], &sc)
	if sc["title"] != "Test 90s Cereal Ad" || sc["description"] != "A 1994 cereal commercial." {
		t.Errorf("sidecar lost text signals: %+v", sc)
	}

	// ⚠ **THE APPROVAL GATE.** A clip Loomarr DOWNLOADED must be marked as ours, or the sync
	// files it on sight instead of holding it in Incoming for a human (§10 V38c). Nothing wrote
	// this mark until V38c.8 — the `fetched=true` branch of `TakeIn` had no caller at all — so
	// every auto-fetched clip went straight to air unreviewed. Found by running auto-fetch
	// against real archive.org collections and reading `held=false` off every row.
	//
	// Asserted through `filler.SidecarFetchedMark()` rather than a literal, so this test and the
	// sync's `wasFetchedByUs` cannot drift apart into two spellings of the same key.
	ours, ok := sc[filler.SidecarLoomarrKey()].(map[string]any)
	if !ok {
		t.Fatalf("downloaded clip is not marked as ours — it would file WITHOUT REVIEW: %+v", sc)
	}
	for k, want := range filler.SidecarFetchedMark() {
		if ours[k] != want {
			t.Errorf("fetched mark %s = %v, want %v", k, ours[k], want)
		}
	}
}

func TestArchive_DownloadCarriesAcquisitionProvenance(t *testing.T) {
	fs := newMemSink()
	c := newTestClient(t, fs)
	ctx := withAcquisition(context.Background(), "archive:classic", "acq-17", "", nil)

	if _, _, _, err := c.walk(ctx, "test-ad", "/drop"); err != nil {
		t.Fatal(err)
	}
	for path, raw := range fs.files {
		if !strings.HasSuffix(path, ".info.json") {
			continue
		}
		var sc map[string]any
		if err := json.Unmarshal(raw, &sc); err != nil {
			t.Fatal(err)
		}
		ours, _ := sc[filler.SidecarLoomarrKey()].(map[string]any)
		if ours["sourceId"] != "archive:classic" || ours["acquisitionId"] != "acq-17" {
			t.Fatalf("loomarr sidecar = %#v, want source and acquisition provenance", ours)
		}
		return
	}
	t.Fatal("no info-JSON sidecar written")
}

func TestArchive_SkipsIfPresent(t *testing.T) {
	fs := newMemSink()
	c := newTestClient(t, fs)
	// First fetch.
	_, _, _, _ = c.walk(context.Background(), "test-ad", "/drop")
	// Second walk: the media file exists → skipped, not re-downloaded.
	fetched, skipped, _, err := c.walk(context.Background(), "test-ad", "/drop")
	if err != nil {
		t.Fatal(err)
	}
	if fetched != 0 || skipped != 1 {
		t.Errorf("re-walk = fetched %d skipped %d, want 0/1 (idempotent)", fetched, skipped)
	}
}

func TestArchive_WalksCollection(t *testing.T) {
	fs := newMemSink()
	c := newTestClient(t, fs)
	fetched, _, _, err := c.walk(context.Background(), "https://archive.org/details/test-collection", "/drop")
	if err != nil {
		t.Fatal(err)
	}
	if fetched != 1 {
		t.Errorf("collection walk fetched %d, want 1 (its one member item)", fetched)
	}
}

func TestArchiveEstimateBudgetsTheRepresentationDownloadWillSelect(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, newMemSink())
	item, err := c.estimate(context.Background(), "https://archive.org/details/test-ad")
	if err != nil {
		t.Fatal(err)
	}
	// 246 MB original plus the 25% staging margin; acquisition reserves exactly what it may write.
	if item.WriteCeilingBytes != 246_000_000+246_000_000/4 || item.ReservationBytes != item.WriteCeilingBytes {
		t.Fatalf("item estimate = %+v, want original bytes plus the staging margin, reserved 1:1", item)
	}
	collection, err := c.estimate(context.Background(), "test-collection")
	if err != nil {
		t.Fatal(err)
	}
	if collection != item {
		t.Fatalf("one-item collection estimate = %+v, want %+v", collection, item)
	}
}

func TestArchiveEstimateDoesNotNarrowProviderHeight(t *testing.T) {
	t.Parallel()
	budget, err := estimateArchiveItem(metadataResp{Files: []archiveFile{{
		Name: "corrupt-height.mp4", Format: "MPEG4", Source: "original",
		Length: "1", Height: "9223372036854775807",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	// Provider dimensions outside the portable positive-int range are corrupt metadata, not a
	// real resolution. Every architecture must therefore use the governor's bounded unknown-
	// resolution fallback: 20 Mbit/s for one second plus the fixed 32 MiB acquisition margin.
	// Direct int64-to-int narrowing made this architecture-dependent.
	const wantWriteCeiling = int64(2_500_000 + 32<<20)
	if budget.WriteCeilingBytes != wantWriteCeiling {
		t.Fatalf("write ceiling = %d, want %d without platform-sized narrowing", budget.WriteCeilingBytes, wantWriteCeiling)
	}
}

func TestDiskSinkStopsAndRemovesAnOverCeilingPartialDownload(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "clip.mp4")
	err := (diskSink{}).WriteStream(t.Context(), target, bytes.NewReader([]byte("12345")), newWriteGuard(nil, 4))
	if !errors.Is(err, ErrWriteCeilingExceeded) {
		t.Fatalf("WriteStream error = %v, want byte-ceiling refusal", err)
	}
	for _, path := range []string{target, target + ".part"} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("partial path %s remains after overflow: %v", path, statErr)
		}
	}
}

func TestPickVideoFile_PrefersMeasuredSourceRepresentation(t *testing.T) {
	files := []archiveFile{
		{Name: "orig-low.mp4", Format: "MPEG4", Size: "246000000", Source: "original", Length: "91", Width: "640", Height: "480"},
		{Name: "orig-best.mp4", Format: "MPEG4", Size: "490000000", Source: "original", Length: "91.00", Width: "1280", Height: "960"},
		{Name: "deriv.ia.mp4", Format: "h.264 IA", Size: "9000000", Source: "derivative", Length: "91", Width: "1920", Height: "1080"},
		{Name: "thumb.jpg", Format: "Thumbnail", Size: "12000"},
		{Name: "meta.xml", Format: "Metadata", Size: "500"},
	}
	f, ok := pickVideoFile(files)
	if !ok {
		t.Fatal("expected a video file")
	}
	if f.Name != "orig-best.mp4" {
		t.Errorf("picked %q, want the best measured original orig-best.mp4", f.Name)
	}
}

func TestPickVideoFile_UnknownNeverBeatsObservedWithinSourceClass(t *testing.T) {
	files := []archiveFile{
		{Name: "unknown.mp4", Format: "MPEG4", Source: "original", Size: "999999999"},
		{Name: "measured.mp4", Format: "MPEG4", Source: "original", Size: "1000000", Length: "30", Width: "640", Height: "480"},
	}
	f, ok := pickVideoFile(files)
	if !ok || f.Name != "measured.mp4" {
		t.Fatalf("picked %#v, want measured original", f)
	}
}

func TestPickVideoFile_UsesStableFilenameTieBreak(t *testing.T) {
	files := []archiveFile{
		{Name: "z.mp4", Format: "MPEG4", Source: "derivative", Size: "1000000", Length: "30", Width: "640", Height: "480"},
		{Name: "A.mp4", Format: "MPEG4", Source: "derivative", Size: "1000000", Length: "30", Width: "640", Height: "480"},
	}
	f, ok := pickVideoFile(files)
	if !ok || f.Name != "A.mp4" {
		t.Fatalf("picked %#v, want A.mp4", f)
	}
}

func TestPickVideoFile_NoneWhenNoVideo(t *testing.T) {
	if _, ok := pickVideoFile([]archiveFile{{Name: "x.jpg", Format: "Thumbnail"}}); ok {
		t.Error("a thumbnail-only item should yield no video file")
	}
}

func TestArchiveIDFromURL(t *testing.T) {
	cases := map[string]string{
		"https://archive.org/details/warning-cic-logo": "warning-cic-logo",
		"https://archive.org/metadata/some-item":       "some-item",
		"https://archive.org/details/classic-tv-ads/":  "classic-tv-ads", // trailing slash
		"bare-id-123": "bare-id-123",
	}
	for in, want := range cases {
		if got := archiveIDFromURL(in); got != want {
			t.Errorf("archiveIDFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestArchiveEstimateItemWithNoVideoFileIsNothingToFetch(t *testing.T) {
	t.Parallel()
	_, err := estimateArchiveItem(metadataResp{Files: []archiveFile{{Name: "track.mp3", Format: "VBR MP3", Source: "original"}}})
	if !errors.Is(err, ErrNothingToFetch) {
		t.Fatalf("audio-only item estimate error = %v, want ErrNothingToFetch (downloadItem skips it)", err)
	}
	_, err = estimateArchiveItem(metadataResp{Files: []archiveFile{{Name: "clip.mp4", Format: "MPEG4", Source: "original"}}})
	if !errors.Is(err, ErrEstimateUnavailable) {
		t.Fatalf("video with no size or length error = %v, want ErrEstimateUnavailable", err)
	}
}

// A collection mixing a sized video, an audio-only item, and an unsized video must budget what
// Download will fetch: the audio item is free, the unsized one takes the bounded fallback.
func TestArchiveCollectionEstimateSkipsUnfetchedItemsAndCapsUnsizedOnes(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/metadata/", func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/metadata/") {
		case "mixed":
			_ = json.NewEncoder(w).Encode(metadataResp{Metadata: archiveMetadata{MediaType: "collection"}})
		case "sized":
			_ = json.NewEncoder(w).Encode(metadataResp{Files: []archiveFile{{Name: "a.mp4", Format: "MPEG4", Size: "100000000", Source: "original"}}})
		case "audio":
			_ = json.NewEncoder(w).Encode(metadataResp{Files: []archiveFile{{Name: "a.mp3", Format: "VBR MP3", Source: "original"}}})
		case "unsized":
			_ = json.NewEncoder(w).Encode(metadataResp{Files: []archiveFile{{Name: "b.mp4", Format: "MPEG4", Source: "original"}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("/advancedsearch.php", func(w http.ResponseWriter, _ *http.Request) {
		var sr searchResp
		sr.Response.NumFound = 3
		sr.Response.Docs = []searchDoc{{Identifier: "sized"}, {Identifier: "audio"}, {Identifier: "unsized"}}
		_ = json.NewEncoder(w).Encode(sr)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := newArchiveClient(srv.URL, srv.Client(), newMemSink())
	budget, err := c.estimate(t.Context(), "mixed")
	if err != nil {
		t.Fatalf("one unsizable item failed the collection estimate: %v", err)
	}
	// 100 MB is under the 32 MiB-margin crossover, so the margin is the fixed floor.
	want := int64(100_000_000+32<<20) + storagegovernor.UnknownAcquisitionCeilingBytes
	if budget.WriteCeilingBytes != want || budget.ReservationBytes != want {
		t.Fatalf("collection budget = %+v, want sized item + fallback cap = %d", budget, want)
	}
}

// The reservation for N items is N x the documented per-item formula (source + 25% margin), and
// completed items give it back.
func TestPrepareReservationForNItemsMatchesTheDocumentedFormula(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, newMemSink())
	meter := &fakeMeter{total: 600 * storagegovernor.GiB, free: 400 * storagegovernor.GiB}
	governor := storagegovernor.New(meter, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: 20 * storagegovernor.GiB}
	})
	ingestor := New(nil, &ArchiveDownloader{client: c}, "/filler", slog.New(slog.NewTextHandler(io.Discard, nil))).WithStorageGovernor(governor)
	const items = 3
	sources := make([]Source, items)
	for index := range sources {
		sources[index] = Source{Kind: Archive, URL: "https://archive.org/details/test-ad"}
	}
	plan, err := ingestor.Prepare(t.Context(), sources, storagegovernor.Automatic)
	if err != nil {
		t.Fatal(err)
	}
	const perItem = int64(246_000_000 + 246_000_000/4)
	if got := governor.Snapshot(t.Context(), "/filler").Snapshot.ReservedBytes; got != items*perItem {
		t.Fatalf("reserved for %d items = %d, want %d x %d", items, got, items, perItem)
	}
	plan.Release()
	if got := governor.Snapshot(t.Context(), "/filler").Snapshot.ReservedBytes; got != 0 {
		t.Fatalf("reserved after release = %d, want zero", got)
	}
}

type fakeMeter struct{ total, free int64 }

func (m *fakeMeter) Measure(context.Context, string) (storagegovernor.Measurement, error) {
	return storagegovernor.Measurement{ID: "disk", TotalBytes: m.total, FreeBytes: m.free}, nil
}

func (m *fakeMeter) ManagedBytes(context.Context, string, storagegovernor.Domain) (int64, error) {
	return 0, nil
}
