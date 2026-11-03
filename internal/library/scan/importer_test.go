package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/mediaheader"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/nfo"
	"github.com/loomarr/loomarr/internal/metadataprovider/precedence"
	"github.com/loomarr/loomarr/internal/store"
)

// countingInventory counts snapshot writes on top of the real inventory.
type countingInventory struct {
	inventory.Service
	applied atomic.Int32
}

func (c *countingInventory) ApplySnapshot(ctx context.Context, s inventory.Snapshot) (inventory.ItemID, error) {
	c.applied.Add(1)
	return c.Service.ApplySnapshot(ctx, s)
}

// fakeLister is an arr's whole library.
type fakeLister struct {
	records []mp.Record
	err     error
}

func (l *fakeLister) List(context.Context) ([]mp.Record, error) { return l.records, l.err }

type harness struct {
	t      *testing.T
	root   string
	st     store.Store
	inv    *countingInventory
	im     *Importer
	reads  atomic.Int32
	busy   atomic.Int32
	enable atomic.Bool
}

func newHarness(t *testing.T, managers ...Manager) *harness {
	t.Helper()
	h := &harness{t: t, root: t.TempDir(), st: openStore(t)}
	h.enable.Store(true)
	h.inv = &countingInventory{Service: inventory.New(h.st)}
	resolver, err := precedence.New("US")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.st.AddLibraryRoot(context.Background(), store.LibraryRootCheck{Path: h.root, Readable: true}, time.Now()); err != nil {
		t.Fatal(err)
	}
	h.im = &Importer{
		Store: h.st, Inventory: h.inv, Disk: fakeDisk{readable: map[string]bool{h.root: true}},
		Managers: func() []Manager { return managers },
		NFO:      nfo.New(), Resolver: resolver,
		Header: func(_ context.Context, path string) (mediaheader.Header, error) {
			h.reads.Add(1)
			return mediaheader.Header{Container: "matroska,webm", DurationMillis: 1_320_000, Streams: []inventory.Stream{
				{Index: 0, Kind: inventory.StreamVideo, Codec: "h264", Width: 1920, Height: 1080},
				{Index: 1, Kind: inventory.StreamAudio, Codec: "aac", Channels: 2, Language: "eng"},
			}}, nil
		},
		Busy:    func() bool { return h.busy.Add(-1) >= 0 },
		Enabled: h.enable.Load,
		Poll:    time.Millisecond,
	}
	return h
}

func (h *harness) file(rel string, size int) string {
	h.t.Helper()
	p := filepath.Join(h.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
		h.t.Fatal(err)
	}
	return p
}

func (h *harness) run() {
	h.t.Helper()
	if err := h.im.Run(context.Background()); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) item(origin inventory.OriginKey) inventory.Item {
	h.t.Helper()
	item, ok, err := h.inv.Item(context.Background(), inventory.ItemRef{Origin: &origin})
	if err != nil || !ok {
		h.t.Fatalf("item %v: ok=%v err=%v", origin, ok, err)
	}
	return item
}

func (h *harness) origin(item inventory.Item, key inventory.OriginKey) inventory.ItemOrigin {
	h.t.Helper()
	for _, o := range item.Origins {
		if o.Key == key {
			return o
		}
	}
	h.t.Fatalf("origin %v not on item", key)
	return inventory.ItemOrigin{}
}

func hasID(ids []inventory.ExternalID, ns, value string) bool {
	for _, id := range ids {
		if id.Namespace == ns && id.Value == value {
			return true
		}
	}
	return false
}

// A pass writes each media file once, with its identity from its names and its header facts; a
// second pass over unchanged files opens nothing and writes nothing.
func TestImportWritesOnceAndSkipsUnchanged(t *testing.T) {
	h := newHarness(t)
	h.file("Alpha Movie (2001) {tmdb-90000603}/Alpha Movie (2001).mkv", 10)
	h.file("Alpha Movie (2001) {tmdb-90000603}/Alpha Movie (2001)-trailer.mkv", 4)
	h.file("Bravo Show {tvdb-90081189}/Season 01/Bravo Show S01E02.mkv", 12)
	h.file("Bravo Show {tvdb-90081189}/Season 01/Bravo Show S01E03-E04.mkv", 12)
	h.run()

	movie := h.item(fileOrigin(h.root, "Alpha Movie (2001) {tmdb-90000603}/Alpha Movie (2001).mkv"))
	if movie.Kind != kindMovie || !hasID(movie.ExternalIDs, "tmdb", "90000603") {
		t.Fatalf("movie = %+v", movie)
	}
	if len(movie.Sources) != 1 || movie.Sources[0].Origins[0].Observation.Coverage["streams"] != inventory.CoveragePresent {
		t.Fatalf("movie sources = %+v", movie.Sources)
	}
	if got := movie.Sources[0].Origins[0].Observation.Facts.DurationMillis; got != 1_320_000 {
		t.Errorf("duration = %d", got)
	}

	episode := h.item(fileOrigin(h.root, "Bravo Show {tvdb-90081189}/Season 01/Bravo Show S01E03-E04.mkv"))
	facts := h.origin(episode, fileOrigin(h.root, "Bravo Show {tvdb-90081189}/Season 01/Bravo Show S01E03-E04.mkv")).Observation.Facts
	if episode.Kind != kindEpisode || facts.SeasonNumber != 1 || facts.EpisodeNumber != 3 || facts.EpisodeEnd != 4 {
		t.Fatalf("episode = %+v %+v", episode.Kind, facts)
	}
	if len(facts.Relationships) != 1 || facts.Relationships[0].ExternalID != "tvdb:90081189" {
		t.Errorf("episode relationships = %+v", facts.Relationships)
	}
	series := h.item(seriesOrigin(h.root, "Bravo Show {tvdb-90081189}"))
	if series.Kind != kindSeries || !hasID(series.ExternalIDs, "tvdb", "90081189") {
		t.Errorf("series = %+v", series)
	}
	trailer := h.item(fileOrigin(h.root, "Alpha Movie (2001) {tmdb-90000603}/Alpha Movie (2001)-trailer.mkv"))
	if trailer.Kind != kindExtra || trailer.ID == movie.ID {
		t.Errorf("trailer = %+v", trailer)
	}
	if h.reads.Load() != 4 {
		t.Errorf("header reads = %d, want 4", h.reads.Load())
	}

	h.inv.applied.Store(0)
	h.reads.Store(0)
	h.run()
	if h.inv.applied.Load() != 0 || h.reads.Load() != 0 {
		t.Errorf("unchanged pass: %d writes, %d reads", h.inv.applied.Load(), h.reads.Load())
	}

	// New bytes are a new revision: written and read again.
	time.Sleep(10 * time.Millisecond)
	h.file("Alpha Movie (2001) {tmdb-90000603}/Alpha Movie (2001).mkv", 11)
	h.run()
	if h.reads.Load() != 1 {
		t.Errorf("changed file reads = %d, want 1", h.reads.Load())
	}
}

// A changed NFO rewrites the item without reading the header again; its ids join the item.
func TestImportSidecarChangeRewritesIdentityOnly(t *testing.T) {
	h := newHarness(t)
	h.file("Charlie/Charlie.mkv", 10)
	h.run()
	origin := fileOrigin(h.root, "Charlie/Charlie.mkv")
	if ids := h.item(origin).ExternalIDs; len(ids) != 0 {
		t.Fatalf("ids before sidecar = %+v", ids)
	}

	nfoPath := filepath.Join(h.root, "Charlie", "Charlie.nfo")
	doc := `<movie><title>Charlie Feature</title><uniqueid type="tmdb" default="true">90000611</uniqueid></movie>`
	if err := os.WriteFile(nfoPath, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	h.reads.Store(0)
	h.run()
	item := h.item(origin)
	if !hasID(item.ExternalIDs, "tmdb", "90000611") || h.origin(item, origin).Observation.Facts.Name != "Charlie Feature" {
		t.Fatalf("item after sidecar = %+v", item)
	}
	if h.reads.Load() != 0 {
		t.Errorf("identity-only change read %d headers", h.reads.Load())
	}
	if item.Sources[0].Origins[0].Observation.Coverage["streams"] != inventory.CoveragePresent {
		t.Errorf("identity rewrite dropped header facts: %+v", item.Sources[0].Origins[0].Observation.Coverage)
	}
}

// An arr's file is identified by the arr, through the path map; an arr that fails to list leaves
// the last identity in place rather than demoting the file.
func TestImportArrIdentityThroughPathMap(t *testing.T) {
	radarr := &fakeLister{}
	h := newHarness(t, Manager{Authority: "radarr", Lister: radarr})
	h.file("Delta (1999)/Delta.mkv", 10)
	ctx := context.Background()
	if err := h.st.RecordLibraryRootDiscovery(ctx, store.LibraryRootDiscovery{
		Found: []store.LibraryRootPath{{Authority: "radarr", AuthorityPath: "/movies", RootPath: h.root}}, Complete: []string{"radarr"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	radarr.records = []mp.Record{{
		Source: mp.Source{Provider: mp.IDRadarr, Basis: mp.BasisManager}, Kind: mp.KindMovie,
		IDs:         []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000612"}},
		Files:       []mp.FileRef{{Path: "/movies/Delta (1999)/Delta.mkv"}},
		Descriptive: &mp.Descriptive{Title: "Delta", Year: 1999},
	}}
	h.run()
	origin := fileOrigin(h.root, "Delta (1999)/Delta.mkv")
	if item := h.item(origin); !hasID(item.ExternalIDs, "tmdb", "90000612") {
		t.Fatalf("arr identity = %+v", item.ExternalIDs)
	}

	radarr.err = errors.New("radarr is down")
	radarr.records = nil
	h.inv.applied.Store(0)
	h.run()
	if h.inv.applied.Load() != 0 {
		t.Errorf("arr outage rewrote %d items", h.inv.applied.Load())
	}
	if !h.im.Progress().ArrIncomplete {
		t.Error("progress does not report the arr outage")
	}
}

// A walk that would remove more than a tenth of a root's files is held; confirming applies it.
// A small removal applies at once.
func TestImportRemovalGuard(t *testing.T) {
	h := newHarness(t)
	var paths []string
	for i := range 20 {
		paths = append(paths, h.file("Show/Season 1/Show S01E"+twoDigits(i+1)+".mkv", 5))
	}
	h.run()

	if err := os.Remove(paths[0]); err != nil {
		t.Fatal(err)
	}
	h.run()
	gone := fileOrigin(h.root, "Show/Season 1/Show S01E01.mkv")
	if o := h.origin(h.item(gone), gone); o.MissingAt.IsZero() {
		t.Fatal("a 1-in-20 removal was not applied")
	}

	for _, p := range paths[1:6] {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	h.run()
	held := fileOrigin(h.root, "Show/Season 1/Show S01E02.mkv")
	if o := h.origin(h.item(held), held); !o.MissingAt.IsZero() {
		t.Fatal("a 5-in-19 removal was applied without confirmation")
	}
	if r := h.im.Progress().Roots; len(r) != 1 || r[0].State != RootHeld || r[0].Held != 5 {
		t.Fatalf("roots = %+v", r)
	}
	counts, err := h.st.LibraryFileCounts(context.Background())
	if err != nil || len(counts.Held) != 1 || counts.Held[0].Count != 5 || counts.Held[0].Kind != "episode" {
		t.Fatalf("counts = %+v, %v", counts, err)
	}

	lib := &Library{Scanner: NewScanner(h.im, 0)}
	if err := lib.ConfirmRemovals(context.Background(), h.root); err != nil {
		t.Fatal(err)
	}
	h.run() // the pass the confirmation asked for
	if o := h.origin(h.item(held), held); o.MissingAt.IsZero() {
		t.Error("confirmed removal not applied")
	}
	kept := fileOrigin(h.root, "Show/Season 1/Show S01E10.mkv")
	if o := h.origin(h.item(kept), kept); !o.MissingAt.IsZero() {
		t.Error("a present file was marked missing")
	}
	if err := lib.ConfirmRemovals(context.Background(), h.root); !errors.Is(err, ErrNothingHeld) {
		t.Errorf("second confirm = %v", err)
	}
}

// An unreadable root is an incomplete scan: nothing under it is marked missing.
func TestImportUnreadableRootMarksNothing(t *testing.T) {
	h := newHarness(t)
	h.file("Echo/Echo.mkv", 10)
	h.run()
	h.im.Disk = fakeDisk{}
	if err := os.Remove(filepath.Join(h.root, "Echo", "Echo.mkv")); err != nil {
		t.Fatal(err)
	}
	h.run()
	origin := fileOrigin(h.root, "Echo/Echo.mkv")
	if o := h.origin(h.item(origin), origin); !o.MissingAt.IsZero() {
		t.Error("unreadable root marked a file missing")
	}
	if r := h.im.Progress().Roots; r[0].State != RootUnreadable {
		t.Errorf("state = %s", r[0].State)
	}
}

// Header reads wait while playout is live, and the kill switch stops a pass before it starts.
func TestImportYieldsAndKillSwitch(t *testing.T) {
	h := newHarness(t)
	h.file("Foxtrot/Foxtrot.mkv", 10)
	h.busy.Store(3)
	h.run()
	if h.reads.Load() != 1 || h.busy.Load() >= 0 {
		t.Fatalf("reads = %d, busy polls left = %d", h.reads.Load(), h.busy.Load())
	}

	h.enable.Store(false)
	h.file("Golf/Golf.mkv", 10)
	h.inv.applied.Store(0)
	h.run()
	if h.inv.applied.Load() != 0 {
		t.Error("a disabled scanner wrote")
	}
}

// Releasing a root forgets its files and marks them unseen.
func TestImportReleasedRootIsForgotten(t *testing.T) {
	h := newHarness(t)
	h.file("Hotel/Hotel.mkv", 10)
	h.run()
	if err := h.st.SetLibraryRootInUse(context.Background(), h.root, false); err != nil {
		t.Fatal(err)
	}
	h.run()
	origin := fileOrigin(h.root, "Hotel/Hotel.mkv")
	if o := h.origin(h.item(origin), origin); o.MissingAt.IsZero() {
		t.Error("released root's file not marked missing")
	}
	if roots, err := h.st.LibraryFileRoots(context.Background()); err != nil || len(roots) != 0 {
		t.Errorf("file roots = %v, %v", roots, err)
	}
}

func twoDigits(n int) string { return fmt.Sprintf("%02d", n) }
