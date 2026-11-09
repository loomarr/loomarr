package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/media/header"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/nfo"
	"github.com/loomarr/loomarr/internal/metadataprovider/precedence"
	"github.com/loomarr/loomarr/internal/platform/store"
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

// fakeLister is an arr's whole library. It counts listings and single-item reads; fetchErr fails
// the reads.
type fakeLister struct {
	records  []mp.Record
	err      error
	fetchErr error
	lists    int
	fetches  int
}

func (l *fakeLister) List(context.Context) ([]mp.Record, error) {
	l.lists++
	return l.records, l.err
}

func (l *fakeLister) Fetch(_ context.Context, ref mp.Ref) (mp.Record, error) {
	l.fetches++
	if l.fetchErr != nil {
		return mp.Record{}, l.fetchErr
	}
	for _, rec := range l.records {
		if rec.Kind == ref.Kind && slices.Contains(rec.IDs, ref.ID) {
			return rec, nil
		}
	}
	return mp.Record{}, mp.ErrNotFound
}

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
		NFO:      nfo.New(), Resolver: resolver, Keys: fakeCatalogKeys,
		Header: func(_ context.Context, path string) (header.Header, error) {
			h.reads.Add(1)
			return header.Header{Container: "matroska,webm", DurationMillis: 1_320_000, Streams: []inventory.Stream{
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

// fileRow is the library_files row the last pass left for path.
func (h *harness) fileRow(path string) store.LibraryFile {
	h.t.Helper()
	rows, err := h.st.LibraryFiles(context.Background(), h.root)
	if err != nil {
		h.t.Fatal(err)
	}
	for _, r := range rows {
		if r.Path == path {
			return r
		}
	}
	h.t.Fatalf("no file row for %s", path)
	return store.LibraryFile{}
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
	// The trailer is no title: nothing is written for it and its header is never read.
	trailer := fileOrigin(h.root, "Alpha Movie (2001) {tmdb-90000603}/Alpha Movie (2001)-trailer.mkv")
	if _, ok, err := h.inv.Item(context.Background(), inventory.ItemRef{Origin: &trailer}); err != nil || ok {
		t.Errorf("trailer written: ok=%v err=%v", ok, err)
	}
	if h.reads.Load() != 3 {
		t.Errorf("header reads = %d, want 3", h.reads.Load())
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

// Library → Items lists the movie and the series a pass wrote, with the series counting its
// episodes and a skipped trailer counting toward nothing; once headers are read nothing is getting
// ready.
func TestImportListsMoviesAndSeries(t *testing.T) {
	h := newHarness(t)
	h.file("Alpha Movie (2001) {tmdb-90000603}/Alpha Movie (2001).mkv", 10)
	h.file("Alpha Movie (2001) {tmdb-90000603}/Alpha Movie (2001)-trailer.mkv", 4)
	h.file("Bravo Show {tvdb-90081189}/Season 01/Bravo Show S01E02.mkv", 12)
	h.file("Bravo Show {tvdb-90081189}/Season 01/Bravo Show S01E03.mkv", 12)
	h.run()

	page, err := h.st.ListLibraryItems(context.Background(), store.LibraryItemQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("items = %+v, want the movie and the series", page.Items)
	}
	movie := h.item(fileOrigin(h.root, "Alpha Movie (2001) {tmdb-90000603}/Alpha Movie (2001).mkv"))
	series := h.item(seriesOrigin(h.root, "Bravo Show {tvdb-90081189}"))
	got := map[string]store.LibraryItemSummary{}
	for _, it := range page.Items {
		got[it.ID] = it
	}
	if m := got[string(movie.ID)]; m.Kind != "movie" || m.Name == "" || m.Files != 1 || m.Pending != 0 {
		t.Errorf("movie row = %+v", m)
	}
	if s := got[string(series.ID)]; s.Kind != "series" || s.Name != "Bravo Show" || s.Files != 2 || s.Pending != 0 {
		t.Errorf("series row = %+v", s)
	}

	// A row written before items were listed has no item: the next pass writes it again.
	rows, err := h.st.LibraryFiles(context.Background(), h.root)
	if err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		rows[i].ItemID, rows[i].SeriesID = "", ""
	}
	if err := h.st.SaveLibraryFiles(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	h.run()
	if again, err := h.st.ListLibraryItems(context.Background(), store.LibraryItemQuery{}); err != nil || again.Total != 2 {
		t.Fatalf("after relinking = %+v, %v", again, err)
	}
}

// A multi-episode file is one item that carries every episode's own ids, so a catalog that keys the
// file by any of its episodes finds it: from Sonarr's episode records, or an NFO's further
// episodedetails when no arr manages the show.
func TestImportMultiEpisodeFileCarriesEveryEpisodeID(t *testing.T) {
	sonarr := &fakeLister{}
	h := newHarness(t, Manager{Authority: "sonarr", Lister: sonarr})
	ctx := context.Background()
	if err := h.st.RecordLibraryRootDiscovery(ctx, store.LibraryRootDiscovery{
		Found: []store.LibraryRootPath{{Authority: "sonarr", AuthorityPath: "/tv", RootPath: h.root}}, Complete: []string{"sonarr"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	managedRel := "Echo Show/Season 01/Echo Show S01E03-E04.mkv"
	h.file(managedRel, 10)
	episode := func(n int, tvdb string) mp.Record {
		return mp.Record{
			Source: mp.Source{Provider: mp.IDSonarr, Basis: mp.BasisManager}, Kind: mp.KindEpisode,
			IDs:       []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: tvdb}},
			Numbering: &mp.Numbering{Season: 1, Episode: n},
			Files:     []mp.FileRef{{Path: "/tv/" + managedRel}},
		}
	}
	sonarr.records = []mp.Record{{
		Source: mp.Source{Provider: mp.IDSonarr, Basis: mp.BasisManager}, Kind: mp.KindSeries,
		IDs:      []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: "90081190"}},
		Children: []mp.Record{episode(3, "90090103"), episode(4, "90090104")},
	}}

	sidecarRel := "Foxtrot Show/Season 02/Foxtrot Show S02E05-E06.mkv"
	h.file(sidecarRel, 10)
	doc := `<episodedetails><title>Part One</title><season>2</season><episode>5</episode>` +
		`<uniqueid type="tvdb" default="true">90090205</uniqueid></episodedetails>` +
		`<episodedetails><title>Part Two</title><season>2</season><episode>6</episode>` +
		`<uniqueid type="tvdb" default="true">90090206</uniqueid></episodedetails>`
	if err := os.WriteFile(filepath.Join(h.root, "Foxtrot Show", "Season 02", "Foxtrot Show S02E05-E06.nfo"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	h.run()

	for _, tc := range []struct {
		rel         string
		first, last string
		from, to    int
	}{
		{managedRel, "90090103", "90090104", 3, 4},
		{sidecarRel, "90090205", "90090206", 5, 6},
	} {
		origin := fileOrigin(h.root, tc.rel)
		item := h.item(origin)
		if !hasID(item.ExternalIDs, "tvdb", tc.first) || !hasID(item.ExternalIDs, "tvdb", tc.last) {
			t.Errorf("%s ids = %+v, want both episodes' %s and %s", tc.rel, item.ExternalIDs, tc.first, tc.last)
		}
		facts := h.origin(item, origin).Observation.Facts
		if facts.EpisodeNumber != tc.from || facts.EpisodeEnd != tc.to {
			t.Errorf("%s numbering = E%d-E%d, want E%d-E%d", tc.rel, facts.EpisodeNumber, facts.EpisodeEnd, tc.from, tc.to)
		}
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

// An arr connected after the folders were set up has none of its folders recorded: the next full
// pass asks for them first, so its files meet its records without anyone pressing "Check again".
// Once they are recorded it does not ask again, and an arr with no connection is never asked.
func TestImportAsksForANewlyConnectedArrsFolders(t *testing.T) {
	radarr := &fakeLister{records: []mp.Record{{
		Source: mp.Source{Provider: mp.IDRadarr, Basis: mp.BasisManager}, Kind: mp.KindMovie,
		IDs:   []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000613"}},
		Files: []mp.FileRef{{Path: "/movies/Echo (2001)/Echo.mkv"}},
	}}}
	h := newHarness(t,
		Manager{Authority: "radarr", Lister: radarr, Address: func() string { return "http://radarr.invalid" }},
		Manager{Authority: "sonarr", Lister: &fakeLister{}, Address: func() string { return "" }},
	)
	h.file("Echo (2001)/Echo.mkv", 10)
	var asked int
	h.im.Discover = func(ctx context.Context) error {
		asked++
		return h.st.RecordLibraryRootDiscovery(ctx, store.LibraryRootDiscovery{
			Found: []store.LibraryRootPath{{Authority: "radarr", AuthorityPath: "/movies", RootPath: h.root}}, Complete: []string{"radarr"},
		}, time.Now())
	}
	h.run()
	if item := h.item(fileOrigin(h.root, "Echo (2001)/Echo.mkv")); asked != 1 || !hasID(item.ExternalIDs, "tmdb", "90000613") {
		t.Fatalf("asked %d times, ids %+v; want one ask and Radarr's identity", asked, item.ExternalIDs)
	}
	h.run()
	if asked != 1 {
		t.Errorf("asked %d times; an arr whose folders are recorded is not asked again", asked)
	}
}

// An NFO read in full gives the title its tags, audience score and people; an older scraper's
// bare <id> grounds it (decision 59).
func TestImportReadsTheWholeNFO(t *testing.T) {
	h := newHarness(t)
	h.file("Delta/Delta.mkv", 10)
	doc := `<movie><title>Delta Feature</title><id>90000612</id><tag>Heist</tag>` +
		`<ratings><rating name="imdb" max="10" default="true"><value>6.8</value></rating></ratings>` +
		`<actor><name>Invented Lead</name><role>The Planner</role><order>0</order></actor>` +
		`<director>Invented Director</director></movie>`
	if err := os.WriteFile(filepath.Join(h.root, "Delta", "Delta.nfo"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	h.run()
	origin := fileOrigin(h.root, "Delta/Delta.mkv")
	item := h.item(origin)
	facts := h.origin(item, origin).Observation.Facts
	wantPeople := []inventory.Person{{Name: "Invented Lead", Role: "actor", Character: "The Planner"},
		{Name: "Invented Director", Role: "director"}}
	if !hasID(item.ExternalIDs, "tmdb", "90000612") || !reflect.DeepEqual(facts.Tags, []string{"Heist"}) ||
		facts.CommunityScore != 6.8 || !reflect.DeepEqual(facts.People, wantPeople) {
		t.Fatalf("ids %+v, tags %v, score %v, people %+v", item.ExternalIDs, facts.Tags, facts.CommunityScore, facts.People)
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
		Page:        "http://radarr.invalid/movie/90000612",
	}}
	h.run()
	origin := fileOrigin(h.root, "Delta (1999)/Delta.mkv")
	item := h.item(origin)
	if !hasID(item.ExternalIDs, "tmdb", "90000612") {
		t.Fatalf("arr identity = %+v", item.ExternalIDs)
	}
	// Radarr decides what it is, and "Wrong title?" opens its page there.
	listed, err := h.st.LibraryItem(ctx, string(item.ID))
	if err != nil {
		t.Fatal(err)
	}
	want := store.LibraryMatch{Basis: "manager", App: "radarr", Page: "http://radarr.invalid/movie/90000612"}
	if listed.Match != want {
		t.Fatalf("match = %+v, want %+v", listed.Match, want)
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
	var heard []bool
	h.im.RootReadability = func(_ context.Context, root string, readable bool) {
		if root != h.root {
			t.Errorf("heard about %s", root)
		}
		heard = append(heard, readable)
	}
	h.file("Echo/Echo.mkv", 10)
	h.run()
	disk := h.im.Disk
	h.im.Disk = fakeDisk{}
	if err := os.Remove(filepath.Join(h.root, "Echo", "Echo.mkv")); err != nil {
		t.Fatal(err)
	}
	h.run()
	h.run() // still offline: heard once, not per pass
	origin := fileOrigin(h.root, "Echo/Echo.mkv")
	if o := h.origin(h.item(origin), origin); !o.MissingAt.IsZero() {
		t.Error("unreadable root marked a file missing")
	}
	if r := h.im.Progress().Roots; r[0].State != RootUnreadable {
		t.Errorf("state = %s", r[0].State)
	}
	if roots, _ := h.st.LibraryRoots(context.Background()); roots.Roots[0].Readable {
		t.Error("the status strip still reads the offline root as readable")
	}
	h.im.Disk = disk
	h.run()
	if !slices.Equal(heard, []bool{false, true}) {
		t.Errorf("heard %v, want offline then back", heard)
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

// While an arr is down a new file waits rather than being written without the arr's ids, and
// nothing is marked missing; once the arr answers, both happen.
func TestImportArrOutageDefersWritesAndRemovals(t *testing.T) {
	radarr := &fakeLister{}
	h := newHarness(t, Manager{Authority: "radarr", Lister: radarr})
	ctx := context.Background()
	if err := h.st.RecordLibraryRootDiscovery(ctx, store.LibraryRootDiscovery{
		Found: []store.LibraryRootPath{{Authority: "radarr", AuthorityPath: "/movies", RootPath: h.root}}, Complete: []string{"radarr"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	for i := range 11 {
		h.file(fmt.Sprintf("Filler %02d/Filler %02d.mkv", i, i), 5)
	}
	gone := h.file("Delta (1999)/Delta.mkv", 10)
	h.run()

	radarr.err = errors.New("radarr is down")
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	h.file("Echo (2003)/Echo.mkv", 10)
	h.run()
	echo := fileOrigin(h.root, "Echo (2003)/Echo.mkv")
	if _, ok, err := h.inv.Item(ctx, inventory.ItemRef{Origin: &echo}); err != nil || ok {
		t.Fatalf("a new file was written while the arr was down: ok=%v err=%v", ok, err)
	}
	delta := fileOrigin(h.root, "Delta (1999)/Delta.mkv")
	if o := h.origin(h.item(delta), delta); !o.MissingAt.IsZero() {
		t.Fatal("a file was marked missing while the arr was down")
	}
	// Its file row too: playout reads only present files, so a missing row would take the title
	// off the air.
	if r := h.fileRow(gone); !r.MissingAt.IsZero() {
		t.Fatal("a file row was marked missing while the arr was down")
	}

	radarr.err = nil
	radarr.records = []mp.Record{{
		Source: mp.Source{Provider: mp.IDRadarr, Basis: mp.BasisManager}, Kind: mp.KindMovie,
		IDs:   []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000613"}},
		Files: []mp.FileRef{{Path: "/movies/Echo (2003)/Echo.mkv"}},
	}}
	h.run()
	if item := h.item(echo); !hasID(item.ExternalIDs, "tmdb", "90000613") {
		t.Errorf("echo after recovery = %+v", item.ExternalIDs)
	}
	if o := h.origin(h.item(delta), delta); o.MissingAt.IsZero() {
		t.Error("the removal was not applied once the arr answered")
	}
	if r := h.fileRow(gone); r.MissingAt.IsZero() {
		t.Error("the file row's removal was not applied once the arr answered")
	}
}

// A down arr holds back only the files in the folders it manages: a new file in a folder no arr
// manages is written at once. Folder passes don't ask the down arr again until arrRetry has passed.
func TestImportArrOutageHoldsOnlyItsOwnFolders(t *testing.T) {
	radarr := &fakeLister{}
	h := newHarness(t, Manager{Authority: "radarr", Lister: radarr})
	ctx := context.Background()
	movies := t.TempDir()
	h.im.Disk = fakeDisk{readable: map[string]bool{h.root: true, movies: true}}
	if err := h.st.AddLibraryRoot(ctx, store.LibraryRootCheck{Path: movies, Readable: true}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := h.st.RecordLibraryRootDiscovery(ctx, store.LibraryRootDiscovery{
		Found: []store.LibraryRootPath{{Authority: "radarr", AuthorityPath: "/movies", RootPath: movies}}, Complete: []string{"radarr"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1_800_000_000, 0)
	h.im.Now = func() time.Time { return at }
	radarr.err = errors.New("radarr is down")

	h.file("Golf (2015)/Golf.mkv", 10)
	held := filepath.Join(movies, "Hotel (2016)", "Hotel.mkv")
	if err := os.MkdirAll(filepath.Dir(held), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(held, make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}
	h.run()
	golf := fileOrigin(h.root, "Golf (2015)/Golf.mkv")
	if _, ok, err := h.inv.Item(ctx, inventory.ItemRef{Origin: &golf}); err != nil || !ok {
		t.Fatalf("a new file in a folder no arr manages waited for the down arr: ok=%v err=%v", ok, err)
	}
	hotel := fileOrigin(movies, "Hotel (2016)/Hotel.mkv")
	if _, ok, err := h.inv.Item(ctx, inventory.ItemRef{Origin: &hotel}); err != nil || ok {
		t.Fatalf("a new file in the down arr's folder was written: ok=%v err=%v", ok, err)
	}

	lists := radarr.lists
	for range 5 {
		h.file("India (2017)/India.mkv", 10)
		if err := h.im.RunFolders(ctx, h.root, []string{filepath.Join(h.root, "India (2017)")}); err != nil {
			t.Fatal(err)
		}
	}
	if radarr.lists != lists {
		t.Errorf("folder passes asked the down arr %d more times within a minute; want none", radarr.lists-lists)
	}
	at = at.Add(arrRetry)
	if err := h.im.RunFolders(ctx, h.root, []string{filepath.Join(h.root, "India (2017)")}); err != nil {
		t.Fatal(err)
	}
	if radarr.lists != lists+1 {
		t.Errorf("after arrRetry, a folder pass asked the down arr %d times; want once", radarr.lists-lists)
	}
}

// An arr with no connection is not a manager: its silence holds nothing back.
func TestImportUnconfiguredArrIsNoOutage(t *testing.T) {
	sonarr := &fakeLister{err: fmt.Errorf("sonarr: %w", mp.ErrNotConfigured)}
	h := newHarness(t, Manager{Authority: "sonarr", Lister: sonarr})
	h.file("Foxtrot (2011)/Foxtrot.mkv", 10)
	h.run()
	foxtrot := fileOrigin(h.root, "Foxtrot (2011)/Foxtrot.mkv")
	if _, ok, err := h.inv.Item(context.Background(), inventory.ItemRef{Origin: &foxtrot}); err != nil || !ok {
		t.Fatalf("the file waited on an arr with no connection: ok=%v err=%v", ok, err)
	}
	if h.im.Progress().ArrIncomplete {
		t.Error("an arr with no connection reads as incomplete")
	}
}

// A file first written with no ids follows them to the item they name once an arr gives them,
// rather than becoming an identity conflict for good.
func TestImportOriginFollowsLaterIDs(t *testing.T) {
	radarr := &fakeLister{}
	h := newHarness(t, Manager{Authority: "radarr", Lister: radarr})
	ctx := context.Background()
	if err := h.st.RecordLibraryRootDiscovery(ctx, store.LibraryRootDiscovery{
		Found: []store.LibraryRootPath{{Authority: "radarr", AuthorityPath: "/movies", RootPath: h.root}}, Complete: []string{"radarr"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	h.file("Papa/Papa.mkv", 10)
	h.run()
	other, err := h.inv.ApplySnapshot(ctx, inventory.Snapshot{
		Origin: inventory.OriginKey{Authority: "test", ExternalItemID: "papa"}, Kind: kindMovie,
		Observation: inventory.Observation[inventory.ItemFacts]{SchemaVersion: 1, ObservedAt: time.Now(),
			Facts: inventory.ItemFacts{Name: "Papa"}},
		ExternalIDs: []inventory.ExternalID{{Namespace: "tmdb", Value: "90000614"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	radarr.records = []mp.Record{{
		Source: mp.Source{Provider: mp.IDRadarr, Basis: mp.BasisManager}, Kind: mp.KindMovie,
		IDs:   []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000614"}},
		Files: []mp.FileRef{{Path: "/movies/Papa/Papa.mkv"}},
	}}
	h.run()
	papa := fileOrigin(h.root, "Papa/Papa.mkv")
	if item := h.item(papa); item.ID != other {
		t.Errorf("file's item = %v, want the item its ids name, %v", item.ID, other)
	}
	if counts, err := h.st.LibraryFileCounts(ctx); err != nil || counts.Problems != 0 {
		t.Errorf("counts = %+v, %v", counts, err)
	}
}

// A root inside another in-use root is walked once, as part of the outer one.
func TestImportNestedRootIsWalkedOnce(t *testing.T) {
	h := newHarness(t)
	inner := filepath.Join(h.root, "Kids")
	h.file("Kids/Quebec/Quebec.mkv", 10)
	if err := h.st.AddLibraryRoot(context.Background(), store.LibraryRootCheck{Path: inner, Readable: true}, time.Now()); err != nil {
		t.Fatal(err)
	}
	h.run()
	h.item(fileOrigin(h.root, "Kids/Quebec/Quebec.mkv"))
	nested := fileOrigin(inner, "Quebec/Quebec.mkv")
	if _, ok, err := h.inv.Item(context.Background(), inventory.ItemRef{Origin: &nested}); err != nil || ok {
		t.Errorf("the nested root wrote its own origin: ok=%v err=%v", ok, err)
	}
	states := map[string]RootState{}
	for _, r := range h.im.Progress().Roots {
		states[r.Root] = r.State
	}
	if states[h.root] != RootScanned || states[inner] != RootNested {
		t.Errorf("roots = %v", states)
	}
}

// A header the share could not give back leaves the file pending for the next pass, rather than
// marking it failed for good.
func TestImportShareFaultKeepsHeaderPending(t *testing.T) {
	h := newHarness(t)
	h.file("Romeo/Romeo.mkv", 10)
	read := h.im.Header
	h.im.Header = func(context.Context, string) (header.Header, error) {
		return header.Header{}, &fs.PathError{Op: "read", Path: "Romeo.mkv", Err: syscall.EIO}
	}
	h.run()
	rows, err := h.st.LibraryFiles(context.Background(), h.root)
	if err != nil || len(rows) != 1 || rows[0].Header != store.LibraryHeaderPending {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	h.im.Header = read
	h.run()
	if h.reads.Load() != 1 {
		t.Errorf("reads after the share recovered = %d, want 1", h.reads.Load())
	}
}

// Header reads wait while someone watches, but the walk never does (decision 11): a folder pass
// made while the header stage waits writes its new file at once, and the waiting stage reads its
// header once playback ends.
func TestHeaderWaitHoldsUpNoPass(t *testing.T) {
	h := newHarness(t)
	var live atomic.Bool
	live.Store(true)
	h.im.Busy = live.Load
	h.file("Whiskey (2001)/Whiskey.mkv", 10)
	ctx := context.Background()
	full := make(chan error, 1)
	go func() { full <- h.im.Run(ctx) }()
	for deadline := time.Now().Add(5 * time.Second); !h.im.Progress().Waiting; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the header stage never waited for playback")
		}
	}

	added := h.file("Xray (2002)/Xray.mkv", 10)
	folders := make(chan error, 1)
	go func() { folders <- h.im.RunFolders(ctx, h.root, []string{filepath.Dir(added)}) }()
	select {
	case err := <-folders:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a folder pass waited for playback to end")
	}
	h.item(fileOrigin(h.root, "Xray (2002)/Xray.mkv"))
	if r := h.fileRow(added); r.Header != store.LibraryHeaderPending {
		t.Errorf("new file's header = %q while playback runs, want pending", r.Header)
	}

	live.Store(false)
	if err := <-full; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{added, filepath.Join(h.root, "Whiskey (2001)", "Whiskey.mkv")} {
		if r := h.fileRow(path); r.Header != store.LibraryHeaderRead {
			t.Errorf("%s header = %q after playback, want read", filepath.Base(path), r.Header)
		}
	}
	if n := h.reads.Load(); n != 2 {
		t.Errorf("header reads = %d, want 2", n)
	}
}

// A pass that arrives while a stage is still starting (ordering its files) hands its files over
// too, rather than waiting out the stage it would otherwise queue behind.
func TestAPassDuringAStageStartIsNotHeldUp(t *testing.T) {
	h := newHarness(t)
	var live atomic.Bool
	live.Store(true)
	h.im.Busy = live.Load
	ordering, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	h.im.Wanted = func(context.Context) []inventory.ExternalID {
		once.Do(func() {
			close(ordering)
			<-release
		})
		return nil
	}
	h.file("Zulu (2005)/Zulu.mkv", 10)
	ctx := context.Background()
	full := make(chan error, 1)
	go func() { full <- h.im.Run(ctx) }()
	<-ordering

	added := h.file("Alpha (2006)/Alpha.mkv", 10)
	folders := make(chan error, 1)
	go func() { folders <- h.im.RunFolders(ctx, h.root, []string{filepath.Dir(added)}) }()
	select {
	case err := <-folders:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("a folder pass waited for a starting stage")
	}
	close(release)
	live.Store(false)
	if err := <-full; err != nil {
		t.Fatal(err)
	}
	if r := h.fileRow(added); r.Header != store.LibraryHeaderRead {
		t.Errorf("handed-over file's header = %q, want read", r.Header)
	}
}

// A file replaced while the header stage waits is read once, as the newer import wrote it: the
// read the older import queued is dropped rather than written over it.
func TestHeaderReadOfAReplacedFileIsDropped(t *testing.T) {
	h := newHarness(t)
	var live atomic.Bool
	live.Store(true)
	h.im.Busy = live.Load
	path := h.file("Yankee (2004)/Yankee.mkv", 10)
	ctx := context.Background()
	full := make(chan error, 1)
	go func() { full <- h.im.Run(ctx) }()
	for deadline := time.Now().Add(5 * time.Second); !h.im.Progress().Waiting; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the header stage never waited for playback")
		}
	}
	h.file("Yankee (2004)/Yankee.mkv", 20)
	if err := h.im.RunFolders(ctx, h.root, []string{filepath.Dir(path)}); err != nil {
		t.Fatal(err)
	}
	live.Store(false)
	if err := <-full; err != nil {
		t.Fatal(err)
	}
	if r := h.fileRow(path); r.Size != 20 || r.Header != store.LibraryHeaderRead {
		t.Errorf("row = size %d header %q, want 20 read", r.Size, r.Header)
	}
	if n := h.reads.Load(); n != 1 {
		t.Errorf("header reads = %d, want 1", n)
	}
}

// A disc image that is gone stops being listed as a problem.
func TestImportGoneDiscImageIsNoLongerAProblem(t *testing.T) {
	h := newHarness(t)
	h.file("Sierra/Sierra.mkv", 10)
	iso := h.file("Tango/Tango.iso", 10)
	h.run()
	ctx := context.Background()
	if counts, err := h.st.LibraryFileCounts(ctx); err != nil || counts.Problems != 1 {
		t.Fatalf("counts = %+v, %v", counts, err)
	}
	if err := os.Remove(iso); err != nil {
		t.Fatal(err)
	}
	h.run()
	if counts, err := h.st.LibraryFileCounts(ctx); err != nil || counts.Problems != 0 || len(counts.Held) != 0 {
		t.Errorf("counts after removal = %+v, %v", counts, err)
	}
}

// A pass over one folder writes what is new in it and removes what is gone from it, and leaves
// everything outside it as it was, gone or not.
func TestImportFoldersPassStaysInItsFolders(t *testing.T) {
	h := newHarness(t)
	for i := range 6 {
		h.file(fmt.Sprintf("Uniform/Season 1/Uniform S01E%02d.mkv", i+1), 5)
		h.file(fmt.Sprintf("Victor/Season 1/Victor S01E%02d.mkv", i+1), 5)
	}
	h.run()
	ctx := context.Background()

	h.file("Uniform/Season 1/Uniform S01E07.mkv", 5)
	if err := os.Remove(filepath.Join(h.root, "Uniform", "Season 1", "Uniform S01E01.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(h.root, "Victor", "Season 1", "Victor S01E01.mkv")); err != nil {
		t.Fatal(err)
	}
	h.inv.applied.Store(0)
	if err := h.im.RunFolders(ctx, h.root, []string{filepath.Join(h.root, "Uniform", "Season 1")}); err != nil {
		t.Fatal(err)
	}
	h.item(fileOrigin(h.root, "Uniform/Season 1/Uniform S01E07.mkv"))
	gone := fileOrigin(h.root, "Uniform/Season 1/Uniform S01E01.mkv")
	if o := h.origin(h.item(gone), gone); o.MissingAt.IsZero() {
		t.Error("a file gone from the folder was not removed")
	}
	outside := fileOrigin(h.root, "Victor/Season 1/Victor S01E01.mkv")
	if o := h.origin(h.item(outside), outside); !o.MissingAt.IsZero() {
		t.Error("a pass over one folder removed a file outside it")
	}
	kept := fileOrigin(h.root, "Victor/Season 1/Victor S01E02.mkv")
	if o := h.origin(h.item(kept), kept); !o.MissingAt.IsZero() {
		t.Error("a pass over one folder marked a present file outside it missing")
	}
	series := seriesOrigin(h.root, "Victor")
	if o := h.origin(h.item(series), series); !o.MissingAt.IsZero() {
		t.Error("a pass over one folder marked another series missing")
	}
	// The new episode on the walk and again with its header, and its series: nothing outside.
	if got := h.inv.applied.Load(); got != 3 {
		t.Errorf("writes = %d, want 3", got)
	}

	// A folder deleted with everything in it is a change to the folder above.
	if err := os.RemoveAll(filepath.Join(h.root, "Victor")); err != nil {
		t.Fatal(err)
	}
	if err := h.im.RunFolders(ctx, h.root, []string{filepath.Join(h.root, "Victor", "Season 1")}); err != nil {
		t.Fatal(err)
	}
	if o := h.origin(h.item(kept), kept); o.MissingAt.IsZero() && h.im.Progress().Roots[0].State != RootHeld {
		t.Error("a deleted folder's files were neither removed nor held")
	}
}

func twoDigits(n int) string { return fmt.Sprintf("%02d", n) }
