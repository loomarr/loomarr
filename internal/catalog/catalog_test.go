package catalog_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
	"github.com/loomarr/loomarr/internal/tmdb"
)

// realLibrary builds a library client pointed at the testkit media-server mock. Its
// search answers "marigold" with three owned films in the real Emby /Items shape,
// the same shape as the pinned search fixture, whose captured titles are real ones
// (library/search_test.go keeps exercising that capture).
func realLibrary(t *testing.T) *library.Client {
	ms := testkit.NewMediaServer(t)
	ms.SetSearchItems(
		testkit.SearchStub{Terms: []string{"marigold"}, LibraryItemID: "1000", Name: "Marigold Protocol", Type: "Movie", Year: 1999, TMDBID: 90000603},
		testkit.SearchStub{Terms: []string{"marigold"}, LibraryItemID: "1001", Name: "Marigold Protocol II", Type: "Movie", Year: 2003, TMDBID: 90000605},
		testkit.SearchStub{Terms: []string{"marigold"}, LibraryItemID: "1002", Name: "Marigold Protocol III", Type: "Movie", Year: 2021, TMDBID: 90624860},
	)
	return library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
}

func TestCatalogSearch_LibraryScope_SetsInLibrary(t *testing.T) {
	lib := realLibrary(t)
	c := catalog.New(lib, nil)

	got, err := c.Search(context.Background(), "marigold", catalog.ScopeLibrary, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("expected library matches for 'marigold'")
	}
	for _, cand := range got {
		if !cand.InLibrary {
			t.Errorf("library-scope candidate not marked in_library: %+v", cand)
		}
		if cand.LibraryItemID == "" {
			t.Errorf("library candidate missing item id: %+v", cand)
		}
	}
	// The library's first result is Marigold Protocol (tmdb 90000603).
	var foundMarigold bool
	for _, cand := range got {
		if cand.TMDBID == 90000603 && cand.Name == "Marigold Protocol" {
			foundMarigold = true
		}
	}
	if !foundMarigold {
		t.Error("expected Marigold Protocol (tmdb:90000603) from the library search")
	}
}

func TestCatalogSearch_MergesLibraryAndTMDB_ByID(t *testing.T) {
	lib := realLibrary(t)
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "test-key")
	c := catalog.New(lib, tm)

	// "marigold" is in BOTH the library (tmdb 90000603) and the TMDB mock (90000603).
	// The merged candidate must appear ONCE, in_library=true.
	got, err := c.Search(context.Background(), "marigold", catalog.ScopeAll, 20)
	if err != nil {
		t.Fatal(err)
	}
	marigoldCount := 0
	for _, cand := range got {
		if cand.TMDBID == 90000603 {
			marigoldCount++
			if !cand.InLibrary {
				t.Error("Marigold Protocol is in the library; merged candidate must be in_library")
			}
		}
	}
	if marigoldCount != 1 {
		t.Errorf("Marigold Protocol should appear once after dedupe, got %d", marigoldCount)
	}
}

// T1.1: genre/overview enrichment is additive — it flows through merge WITHOUT
// changing dedupe/ordering, and the merged candidate carries the theme signals the
// model reasons about. (Guards the risk that enrichment touches identity.)
func TestCatalogSearch_EnrichesGenreOverview_WithoutChangingDedupe(t *testing.T) {
	lib := realLibrary(t)
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "test-key")
	c := catalog.New(lib, tm)

	got, err := c.Search(context.Background(), "marigold", catalog.ScopeAll, 20)
	if err != nil {
		t.Fatal(err)
	}
	var marigold *catalog.Candidate
	count := 0
	for i := range got {
		if got[i].TMDBID == 90000603 {
			marigold = &got[i]
			count++
		}
	}
	if count != 1 || marigold == nil {
		t.Fatalf("Marigold Protocol should still dedupe to exactly one candidate, got %d", count)
	}
	// The merged candidate carries theme signals (genres from TMDB, overview from
	// TMDB) — the whole point of the enrichment.
	if len(marigold.Genres) == 0 {
		t.Error("merged candidate lost genres (enrichment didn't survive merge)")
	}
	if marigold.Overview == "" {
		t.Error("merged candidate lost overview")
	}
	// And it's STILL in-library (identity/merge semantics unchanged).
	if !marigold.InLibrary {
		t.Error("enrichment must not disturb in_library")
	}
}

func TestCatalogSearch_TMDBOnly_NotInLibrary(t *testing.T) {
	// A TMDB title absent from the library ("Quiet Engines") → in_library=false.
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "k")
	c := catalog.New(realLibrary(t), tm)

	got, err := c.Search(context.Background(), "quiet", catalog.ScopeAll, 20)
	if err != nil {
		t.Fatal(err)
	}
	var engines *catalog.Candidate
	for i := range got {
		if got[i].Name == "Quiet Engines" {
			engines = &got[i]
		}
	}
	if engines == nil {
		t.Fatal("expected Quiet Engines from TMDB")
	}
	if engines.InLibrary {
		t.Error("Quiet Engines is not in the library; should be in_library=false")
	}
	if engines.TMDBID != 90000100 {
		t.Errorf("Quiet Engines TMDBID = %d, want 90000100", engines.TMDBID)
	}
}

func TestCatalogSearch_AllScopeSkipsAnUnconfiguredLibrary(t *testing.T) {
	tmdbServer := testkit.NewTMDB(t)
	tmdbClient := tmdb.NewWithBase(tmdbServer.URL, "test-key")
	unconfiguredLibrary := library.NewDynamic(func() library.Connection { return library.Connection{} }, "dev-1")
	c := catalog.New(unconfiguredLibrary, tmdbClient)

	got, err := c.Search(context.Background(), "marigold", catalog.ScopeAll, 20)
	if err != nil {
		t.Fatalf("all configured corpora: %v", err)
	}
	if len(got) == 0 || got[0].TMDBID != 90000603 {
		t.Fatalf("TMDB results after skipping unconfigured Library = %+v", got)
	}

	_, err = c.Search(context.Background(), "marigold", catalog.ScopeLibrary, 20)
	if !errors.Is(err, library.ErrConnectionRequired) {
		t.Fatalf("explicit Library search error = %v, want ErrConnectionRequired", err)
	}
}

func TestCatalogSearch_InLibraryOrderedFirst(t *testing.T) {
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "k")
	c := catalog.New(realLibrary(t), tm)

	// "marigold" matches an in-library title; add a TMDB-only via broad-ish search
	// isn't possible with distinct terms, so just assert in-library sorts first
	// among the marigold results (all in-library here) — and that ordering is stable.
	got, _ := c.Search(context.Background(), "marigold", catalog.ScopeAll, 20)
	seenNotInLib := false
	for _, cand := range got {
		if !cand.InLibrary {
			seenNotInLib = true
		} else if seenNotInLib {
			t.Error("in_library candidate appeared after a not-in-library one (ordering violated)")
		}
	}
}

// A full Library result page must not crowd every outside-Library candidate out of
// the federated result. The Catalog is the grounding corpus for a new Channel
// Proposal, so truncating only after an in-library-first sort makes acquisitions
// impossible whenever the Library alone fills the limit — even though TMDB returned
// relevant titles in the same search.
func TestCatalogSearch_AllScopeKeepsOutsideLibraryDiscoveryWhenLibraryFillsLimit(t *testing.T) {
	ms := testkit.NewMediaServer(t)
	items := make([]testkit.SearchStub, 24)
	for i := range items {
		items[i] = testkit.SearchStub{
			Terms: []string{"a"}, LibraryItemID: fmt.Sprintf("owned-%02d", i),
			Name: fmt.Sprintf("Archive Pick %02d", i), Type: "Movie", Year: 1990 + i%10,
			TMDBID: 90_010_000 + i,
		}
	}
	ms.SetSearchItems(items...)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
	tmdbServer := testkit.NewTMDB(t)
	tmdbClient := tmdb.NewWithBase(tmdbServer.URL, "test-key")

	got, err := catalog.New(lib, tmdbClient).Search(context.Background(), "a", catalog.ScopeAll, 24)
	if err != nil {
		t.Fatal(err)
	}
	var owned, outside int
	for _, candidate := range got {
		if candidate.InLibrary {
			owned++
		} else {
			outside++
		}
	}
	if owned == 0 {
		t.Fatal("federated search dropped every immediately playable Library candidate")
	}
	if outside == 0 {
		t.Fatal("a full Library page crowded every relevant outside-Library candidate out of discovery")
	}
}

func TestCandidateKey(t *testing.T) {
	c := catalog.Candidate{MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol"}
	k, err := c.Key()
	if err != nil || k != "movie:tmdb:90000603" {
		t.Errorf("Key() = %q,%v want movie:tmdb:90000603", k, err)
	}
	// A candidate with no usable id errors (grounding guarantee).
	if _, err := (catalog.Candidate{MediaType: provision.Movie, Name: "Ghost"}).Key(); err == nil {
		t.Error("expected Key() to error on an id-less candidate")
	}
}

// OfficialRating is ADDITIVE metadata: it must never enter identity, or the
// grounding gate / in-library-first ordering could shift. Two otherwise-identical
// candidates that differ ONLY in OfficialRating must produce the same Key.
func TestCandidateKey_IgnoresOfficialRating(t *testing.T) {
	rated := catalog.Candidate{MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol", OfficialRating: "R"}
	unrated := catalog.Candidate{MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol"}
	rk, _ := rated.Key()
	uk, _ := unrated.Key()
	if rk != uk {
		t.Errorf("OfficialRating leaked into identity: %q != %q", rk, uk)
	}
}

// fakePresence marks a fixed set of tmdb ids as in-library, each with its library
// item id and — critically — its content rating, so the backfill-carries-the-rating
// behavior can be asserted (a rating dropped here is dead air under a kids ceiling).
type fakePresence struct {
	owned   map[int]string
	ratings map[int]string
}

func (f fakePresence) Present(_ context.Context, _ provision.MediaType, tmdbID, _ int) (catalog.Presence, bool, error) {
	id, ok := f.owned[tmdbID]
	if !ok {
		return catalog.Presence{}, false, nil
	}
	return catalog.Presence{LibraryItemID: id, OfficialRating: f.ratings[tmdbID]}, true, nil
}

// In-library backfill: a DISCOVERED title the library already owns comes back
// InLibrary=true (a lineup pick), not an acquisition. Closes the discovery gap.
func TestCatalogDiscover_BackfillsInLibrary(t *testing.T) {
	lib := realLibrary(t)
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "test-key")
	// The mock owns Marigold Protocol (90000603), rated R → discovery of Action marks it
	// in-library AND carries the rating, so audience enforcement has something to judge.
	c := catalog.New(lib, tm).WithPresence(fakePresence{
		owned:   map[int]string{90000603: "lib-90000603"},
		ratings: map[int]string{90000603: "R"},
	})

	got, err := c.Discover(context.Background(), catalog.DiscoveryQuery{
		MediaType: provision.Movie, Genres: []string{"Action"}, YearFrom: 1990, YearTo: 1999,
	}, 20)
	if err != nil {
		t.Fatal(err)
	}
	var marigold *catalog.Candidate
	for i := range got {
		if got[i].TMDBID == 90000603 {
			marigold = &got[i]
		}
	}
	if marigold == nil {
		t.Fatal("Marigold Protocol (action) should be discovered")
	}
	if !marigold.InLibrary || marigold.LibraryItemID != "lib-90000603" {
		t.Errorf("owned discovered title should be backfilled in-library, got InLibrary=%v id=%q", marigold.InLibrary, marigold.LibraryItemID)
	}
	// The rating MUST ride along. Dropping it here is FINDING 6: a discovered-then-
	// owned title reaches the scheduler unrated, and an audience ceiling silently
	// excludes it — a channel that goes live playing nothing (§9 dead air).
	if marigold.OfficialRating != "R" {
		t.Errorf("backfill dropped the rating: got %q, want R — an owned title reaches the scheduler unrated", marigold.OfficialRating)
	}
	// A discovered title NOT owned stays an acquisition candidate.
	for _, c := range got {
		if c.TMDBID == 90000100 && c.InLibrary { // Quiet Engines, not in the owned set
			t.Error("un-owned discovered title should stay not-in-library")
		}
	}
}

// An owned title's file size rides from the library row onto the candidate and survives
// the merge with the same title's TMDB row, which has no size (#1817). It is display
// metadata from the response already in hand: no lookup is added to read it.
func TestCatalogSearch_CarriesLibrarySizeThroughMerge(t *testing.T) {
	lib := &catalogfixture.Library{Results: []library.SearchResult{{
		LibraryItemID: "lib-90000603", Name: "Marigold Protocol", Year: 1999, MediaType: library.Movie,
		TMDBID: 90000603, SizeBytes: 21_914_038_799,
	}}}
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{{
		MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol", Year: 1999,
		Overview: "a florist", Source: catalog.ScopeTMDB,
	}}}

	got, err := catalog.New(lib, corpus).Search(context.Background(), "marigold", catalog.ScopeAll, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want the one merged title: %+v", len(got), got)
	}
	if got[0].SizeBytes != 21_914_038_799 {
		t.Errorf("SizeBytes = %d, want the library row's 21914038799", got[0].SizeBytes)
	}
}

// A discovered title the library owns gets its size from the same presence hit that
// confirms ownership.
func TestCatalogDiscover_BackfillCarriesLibrarySize(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{
		{MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol", Source: catalog.ScopeTMDB},
		{MediaType: provision.Movie, TMDBID: 90000604, Name: "Marigold Protocol II", Source: catalog.ScopeTMDB},
	}}
	presence := &catalogfixture.Presence{Hits: map[int]catalog.Presence{
		90000603: {LibraryItemID: "lib-90000603", SizeBytes: 63_546_611_800},
	}}

	got, err := catalog.New(nil, corpus).WithPresence(presence).Discover(
		context.Background(), catalog.DiscoveryQuery{MediaType: provision.Movie}, 10)
	if err != nil {
		t.Fatal(err)
	}
	sizes := map[int]int64{}
	for _, c := range got {
		sizes[c.TMDBID] = c.SizeBytes
	}
	if sizes[90000603] != 63_546_611_800 {
		t.Errorf("owned title SizeBytes = %d, want 63546611800", sizes[90000603])
	}
	if sizes[90000604] != 0 {
		t.Errorf("un-owned title SizeBytes = %d, want 0 (no library file)", sizes[90000604])
	}
}

// Presence backfill has to happen before the owned/missing blend is truncated.
// Otherwise a popular themed corpus whose first page is already owned becomes a
// self-sealing result: the model sees no relevant title it could acquire even when
// TMDB returned missing candidates in the same bounded pool.
func TestCatalogDiscover_KeepsOutsideLibraryCandidatesAfterPresenceBackfill(t *testing.T) {
	candidates := make([]catalog.Candidate, 32)
	owned := make(map[int]catalog.Presence, 24)
	for i := range candidates {
		candidates[i] = catalog.Candidate{
			MediaType: provision.Movie, TMDBID: 90_020_000 + i,
			Name:   fmt.Sprintf("%c Holiday Pick %02d", 'A'+rune(i/24)*25, i),
			Genres: []string{"Family"}, Source: catalog.ScopeTMDB,
		}
		if i < 24 {
			owned[candidates[i].TMDBID] = catalog.Presence{LibraryItemID: fmt.Sprintf("owned-%02d", i)}
		}
	}
	corpus := &catalogfixture.Corpus{Candidates: candidates}
	c := catalog.New(nil, corpus).WithPresence(&catalogfixture.Presence{Hits: owned})

	got, err := c.Discover(context.Background(), catalog.DiscoveryQuery{
		MediaType: provision.Movie, Genres: []string{"Family"},
	}, 24)
	if err != nil {
		t.Fatal(err)
	}
	var outside int
	for _, candidate := range got {
		if !candidate.InLibrary {
			outside++
		}
	}
	if outside == 0 {
		t.Fatal("presence backfill filled the discovery window entirely with owned titles")
	}
}

// The production TMDB API returns twenty discovery rows per page. Catalog's
// larger pre-backfill pool therefore has to make the client advance pages; asking
// for 48 and reading only page one still lets twenty owned titles hide every new
// result.
func TestCatalogDiscover_ReachesOutsideLibraryCandidatesBeyondTMDBFirstPage(t *testing.T) {
	tmdbServer := testkit.NewTMDB(t)
	owned := make(map[int]catalog.Presence, 20)
	for i := range 28 {
		id := 90_030_000 + i
		tmdbServer.AddMovie(id, fmt.Sprintf("Holiday Pick %02d", i), 1990+i%10, []int{10_751}, "A family holiday gathering.")
		if i < 20 {
			owned[id] = catalog.Presence{LibraryItemID: fmt.Sprintf("owned-%02d", i)}
		}
	}
	tmdbClient := tmdb.NewWithBase(tmdbServer.URL, "test-key")
	c := catalog.New(nil, tmdbClient).WithPresence(&catalogfixture.Presence{Hits: owned})

	got, err := c.Discover(context.Background(), catalog.DiscoveryQuery{
		MediaType: provision.Movie, Genres: []string{"Family"},
	}, 24)
	if err != nil {
		t.Fatal(err)
	}
	var outside int
	for _, candidate := range got {
		if candidate.TMDBID >= 90_030_000 && !candidate.InLibrary {
			outside++
		}
	}
	if outside == 0 {
		t.Fatal("discovery never reached the missing holiday candidates on TMDB page two")
	}
}

// An unpinned media-type request means movies AND series. TMDB exposes them on
// separate endpoints, so filling the movie limit before appending TV must not
// silently turn a mixed discovery request into a movies-only corpus.
func TestCatalogDiscover_UnpinnedMediaTypeKeepsMoviesAndSeries(t *testing.T) {
	tmdbServer := testkit.NewTMDB(t)
	for i := range 55 {
		tmdbServer.AddMovie(90_050_000+i, fmt.Sprintf("Action Movie %02d", i), 2000+i%20,
			[]int{28}, "An action movie.")
	}
	// TMDB's TV genre namespace uses 10759 (Action & Adventure), not the movie
	// namespace's 28 (Action). The adapter must translate the human genre per endpoint.
	tmdbServer.AddSeries(90_051_000, "Action Dispatch", 2018, []int{10759}, "An action series.")
	c := catalog.New(nil, tmdb.NewWithBase(tmdbServer.URL, "test-key"))

	got, err := c.Discover(context.Background(), catalog.DiscoveryQuery{Genres: []string{"Action"}}, 24)
	if err != nil {
		t.Fatal(err)
	}
	var movies, series int
	for _, candidate := range got {
		switch candidate.MediaType {
		case provision.Movie:
			movies++
		case provision.Series:
			series++
		}
	}
	if movies == 0 || series == 0 {
		t.Fatalf("untyped discovery collapsed to one media type: movies=%d series=%d", movies, series)
	}
}

func TestCatalogDiscoverKeywords_FindsThematicTitleAndKeepsItOutsideLibrary(t *testing.T) {
	tmdbServer := testkit.NewTMDB(t)
	// Register a broader substring first. The adapter must prefer the exact
	// "Christmas" keyword rather than blindly taking TMDB's first search hit.
	tmdbServer.AddKeywordMovie(90_041_000, "Midnight Eve", 2020, []int{35},
		"A New Year's party goes sideways.", "Christmas Eve")
	tmdbServer.AddKeywordMovie(90_041_001, "Snowbound Reunion", 2021, []int{35, 10_751},
		"Estranged siblings reunite during Christmas week.", "Christmas")
	tmdbClient := tmdb.NewWithBase(tmdbServer.URL, "test-key")
	c := catalog.New(nil, tmdbClient).WithPresence(&catalogfixture.Presence{})

	got, err := c.Discover(context.Background(), catalog.DiscoveryQuery{
		MediaType: provision.Movie, Keywords: []string{"Christmas"}, Genres: []string{"Comedy", "Family"},
		YearFrom: 2015, YearTo: 2025,
	}, 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TMDBID != 90_041_001 {
		t.Fatalf("keyword discovery = %+v, want Snowbound Reunion", got)
	}
	if got[0].InLibrary {
		t.Fatal("a TMDB keyword discovery absent from the Library was not exposed as a possible acquisition")
	}
	var searchedKeyword, discoveredByKeyword bool
	for _, request := range tmdbServer.Requests() {
		searchedKeyword = searchedKeyword || request.Path == "/search/keyword"
		discoveredByKeyword = discoveredByKeyword ||
			(request.Path == "/discover/movie" && strings.Contains(request.RawQuery, "with_keywords="))
	}
	if !searchedKeyword || !discoveredByKeyword {
		t.Fatalf("TMDB keyword path was not used: %+v", tmdbServer.Requests())
	}
}

func TestCatalogDiscoverSnapshotsPresenceOnceAcrossBackfillBatch(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{
		{Name: "One", MediaType: provision.Movie, TMDBID: 90000101},
		{Name: "Two", MediaType: provision.Movie, TMDBID: 90000202},
	}}
	primary := &catalogfixture.Presence{Hits: map[int]catalog.Presence{
		90000101: {LibraryItemID: "item-101"},
		90000202: {LibraryItemID: "item-202"},
	}}
	rotated := &catalogfixture.Presence{}
	snapshots := 0
	c := catalog.New(nil, corpus).WithPresenceSource(func() catalog.LibraryPresence {
		snapshots++
		if snapshots == 1 {
			return primary
		}
		return rotated
	})

	got, err := c.Discover(context.Background(), catalog.DiscoveryQuery{MediaType: provision.Movie}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if snapshots != 1 {
		t.Fatalf("presence snapshots = %d, want one for the backfill batch", snapshots)
	}
	if calls := primary.Calls(); len(calls) != 2 {
		t.Fatalf("primary presence calls = %v, want both candidates", calls)
	}
	if calls := rotated.Calls(); len(calls) != 0 {
		t.Fatalf("rotated presence adapter received in-flight calls: %v", calls)
	}
	for _, candidate := range got {
		if !candidate.InLibrary {
			t.Fatalf("candidate %d was not backfilled from the bound adapter", candidate.TMDBID)
		}
	}
}

func TestCatalogDiscoverPreservesUpstreamRelevanceWithinOwnedOutsideBlend(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{
		{MediaType: provision.Movie, TMDBID: 90000004, Name: "Zulu Outside", Source: catalog.ScopeTMDB},
		{MediaType: provision.Movie, TMDBID: 90000003, Name: "Zulu Owned", Source: catalog.ScopeTMDB},
		{MediaType: provision.Movie, TMDBID: 90000002, Name: "Alpha Outside", Source: catalog.ScopeTMDB},
		{MediaType: provision.Movie, TMDBID: 90000001, Name: "Alpha Owned", Source: catalog.ScopeTMDB},
	}}
	c := catalog.New(nil, corpus).WithPresence(&catalogfixture.Presence{Hits: map[int]catalog.Presence{
		90000003: {LibraryItemID: "owned-3"}, 90000001: {LibraryItemID: "owned-1"},
	}})

	got, err := c.Discover(context.Background(), catalog.DiscoveryQuery{MediaType: provision.Movie}, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{90000003, 90000001, 90000004, 90000002}
	if len(got) != len(want) {
		t.Fatalf("discovery count = %d, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].TMDBID != id {
			t.Fatalf("discovery order = %+v, want upstream-ranked owned %v then outside", got, want)
		}
	}
}

func TestCatalogDiscoverCandidateBlendPolicyControlsOutsideReserve(t *testing.T) {
	candidates := make([]catalog.Candidate, 8)
	hits := make(map[int]catalog.Presence, 6)
	for i := range candidates {
		id := i + 1
		candidates[i] = catalog.Candidate{
			MediaType: provision.Movie,
			TMDBID:    id,
			Name:      fmt.Sprintf("Candidate %d", id),
			Source:    catalog.ScopeTMDB,
		}
		if id <= 6 {
			hits[id] = catalog.Presence{LibraryItemID: fmt.Sprintf("owned-%d", id)}
		}
	}

	for _, tc := range []struct {
		name           string
		configure      bool
		outsidePercent int
		wantOwned      int
		wantOutside    int
	}{
		{name: "production default", wantOwned: 3, wantOutside: 1},
		{name: "owned-first baseline", configure: true, outsidePercent: 0, wantOwned: 4, wantOutside: 0},
		{name: "half reserved", configure: true, outsidePercent: 50, wantOwned: 2, wantOutside: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			corpus := &catalogfixture.Corpus{Candidates: candidates}
			presence := &catalogfixture.Presence{Hits: hits}
			cat := catalog.New(nil, corpus).WithPresence(presence)
			if tc.configure {
				policy, err := catalog.NewCandidateBlendPolicy(tc.outsidePercent)
				if err != nil {
					t.Fatal(err)
				}
				cat.WithCandidateBlendPolicy(policy)
			}
			got, err := cat.Discover(context.Background(), catalog.DiscoveryQuery{MediaType: provision.Movie}, 4)
			if err != nil {
				t.Fatal(err)
			}
			var owned int
			for _, candidate := range got {
				if candidate.InLibrary {
					owned++
				}
			}
			if owned != tc.wantOwned || len(got)-owned != tc.wantOutside {
				t.Fatalf("blend = %d owned/%d outside, want %d/%d", owned, len(got)-owned, tc.wantOwned, tc.wantOutside)
			}
		})
	}
}

func TestNewCandidateBlendPolicyRejectsInvalidPercentage(t *testing.T) {
	for _, percent := range []int{-1, 101} {
		if _, err := catalog.NewCandidateBlendPolicy(percent); err == nil {
			t.Fatalf("NewCandidateBlendPolicy(%d) succeeded, want validation error", percent)
		}
	}
}

func TestCatalogDiscoverBreaksEqualSourceRanksByCanonicalIdentity(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{
		{MediaType: provision.Movie, TMDBID: 90000020, Name: "Earlier Alphabetically", RelevanceRank: 1},
		{MediaType: provision.Movie, TMDBID: 90000010, Name: "Later Alphabetically", RelevanceRank: 1},
	}}
	got, err := catalog.New(nil, corpus).Discover(context.Background(), catalog.DiscoveryQuery{MediaType: provision.Movie}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].TMDBID != 90000010 || got[1].TMDBID != 90000020 {
		t.Fatalf("equal-rank order = %+v, want canonical identity tie-break", got)
	}
}

func TestCatalogDiscoverUnion_AggregatesWindowsBeforeFinalization(t *testing.T) {
	queries := []catalog.DiscoveryQuery{
		{MediaType: provision.Movie, YearFrom: 1990},
		{MediaType: provision.Movie, YearFrom: 1991},
		{MediaType: provision.Movie, YearFrom: 1992},
	}
	corpus := &catalogfixture.Corpus{DiscoverFunc: func(_ context.Context, q catalog.DiscoveryQuery, limit int) ([]catalog.Candidate, error) {
		if limit != 48 {
			t.Fatalf("source limit = %d, want 48", limit)
		}
		switch q.YearFrom {
		case 1990: // An empty window is not an error.
			return nil, nil
		case 1991:
			return []catalog.Candidate{{MediaType: provision.Movie, TMDBID: 90000001, Name: "Shared"}}, nil
		case 1992:
			return []catalog.Candidate{
				{MediaType: provision.Movie, TMDBID: 90000001, Name: "Shared", Overview: "merged"},
				{MediaType: provision.Movie, TMDBID: 90000002, Name: "Later Window"},
			}, nil
		default:
			return nil, nil
		}
	}}

	got, err := catalog.New(nil, corpus).DiscoverUnion(context.Background(), queries)
	if err != nil {
		t.Fatal(err)
	}
	if got.WindowsCompleted != 3 || got.SourceQueriesDispatched != 3 {
		t.Fatalf("counts = %+v, want three completed/dispatched", got)
	}
	if len(got.Candidates) != 2 || got.Candidates[0].TMDBID != 90000001 || got.Candidates[0].Overview != "merged" || got.Candidates[1].TMDBID != 90000002 {
		t.Fatalf("union candidates = %+v, want deduplicated aggregate", got.Candidates)
	}
}

func TestCatalogDiscoverUnion_FinalizesAggregateOnce(t *testing.T) {
	const perWindow = 24
	owned := make(map[int]catalog.Presence, perWindow)
	corpus := &catalogfixture.Corpus{DiscoverFunc: func(_ context.Context, q catalog.DiscoveryQuery, _ int) ([]catalog.Candidate, error) {
		out := make([]catalog.Candidate, perWindow)
		for i := range out {
			id := i + 1
			if q.YearFrom == 2001 {
				id += perWindow
			} else {
				owned[id] = catalog.Presence{LibraryItemID: fmt.Sprintf("owned-%d", id)}
			}
			out[i] = catalog.Candidate{MediaType: provision.Movie, TMDBID: id, Name: fmt.Sprintf("Candidate %d", id)}
		}
		return out, nil
	}}
	presence := &catalogfixture.Presence{Hits: owned}
	got, err := catalog.New(nil, corpus).WithPresence(presence).DiscoverUnion(context.Background(), []catalog.DiscoveryQuery{
		{MediaType: provision.Movie, YearFrom: 2000}, {MediaType: provision.Movie, YearFrom: 2001},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Candidates) != 24 {
		t.Fatalf("candidate count = %d, want one final limit of 24", len(got.Candidates))
	}
	var outside int
	for _, candidate := range got.Candidates {
		if !candidate.InLibrary {
			outside++
		}
	}
	if outside == 0 {
		t.Fatalf("aggregate blend omitted later-window outside candidates: %+v", got.Candidates)
	}
	if calls := presence.Calls(); len(calls) != 48 {
		t.Fatalf("presence calls = %d, want aggregate pool bounded to 48", len(calls))
	}
}

func TestCatalogDiscoverUnion_PreservesWindowPoolsUntilPresence(t *testing.T) {
	const (
		perWindow = 48
		missing   = 24
	)
	owned := make(map[int]catalog.Presence, perWindow)
	corpus := &catalogfixture.Corpus{DiscoverFunc: func(_ context.Context, q catalog.DiscoveryQuery, limit int) ([]catalog.Candidate, error) {
		if limit != perWindow {
			t.Fatalf("source limit = %d, want %d", limit, perWindow)
		}
		out := make([]catalog.Candidate, perWindow)
		for i := range out {
			id := (q.YearFrom-2000)*perWindow + i + 1
			if i >= missing {
				owned[id] = catalog.Presence{LibraryItemID: fmt.Sprintf("owned-%d", id)}
			}
			out[i] = catalog.Candidate{
				MediaType:     provision.Movie,
				TMDBID:        id,
				Name:          fmt.Sprintf("Candidate %d", id),
				RelevanceRank: i + 1,
			}
		}
		return out, nil
	}}
	presence := &catalogfixture.Presence{Hits: owned}
	got, err := catalog.New(nil, corpus).WithPresence(presence).DiscoverUnion(context.Background(), []catalog.DiscoveryQuery{
		{MediaType: provision.Movie, YearFrom: 2000}, {MediaType: provision.Movie, YearFrom: 2001},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Candidates) != 24 {
		t.Fatalf("candidate count = %d, want one final limit of 24", len(got.Candidates))
	}
	var inLibrary int
	for _, candidate := range got.Candidates {
		if candidate.InLibrary {
			inLibrary++
		}
	}
	if inLibrary == 0 {
		t.Fatalf("union discarded every owned candidate before presence: %+v", got.Candidates)
	}
	if calls := presence.Calls(); len(calls) > 2*perWindow {
		t.Fatalf("presence calls = %d, want at most %d", len(calls), 2*perWindow)
	}
}

func TestCatalogDiscoverUnion_AssignsFallbackRanksPerWindow(t *testing.T) {
	const perWindow = 48
	corpus := &catalogfixture.Corpus{DiscoverFunc: func(_ context.Context, q catalog.DiscoveryQuery, limit int) ([]catalog.Candidate, error) {
		if limit != perWindow {
			t.Fatalf("source limit = %d, want %d", limit, perWindow)
		}
		out := make([]catalog.Candidate, perWindow)
		for i := range out {
			id := i + 1
			if q.YearFrom == 2000 {
				id += 1000
			}
			out[i] = catalog.Candidate{MediaType: provision.Movie, TMDBID: id, Name: fmt.Sprintf("Candidate %d", id)}
		}
		return out, nil
	}}
	got, err := catalog.New(nil, corpus).DiscoverUnion(context.Background(), []catalog.DiscoveryQuery{
		{MediaType: provision.Movie, YearFrom: 2000}, {MediaType: provision.Movie, YearFrom: 2001},
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, candidate := range got.Candidates {
		seen[candidate.TMDBID/1000] = true
	}
	if !seen[0] || !seen[1] {
		t.Fatalf("unranked windows were not comparable: %+v", got.Candidates)
	}
}

func TestCatalogDiscoverUnion_FailureAndCancellationAreAtomic(t *testing.T) {
	t.Run("pre-cancellation dispatches nothing", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		corpus := &catalogfixture.Corpus{}
		got, err := catalog.New(nil, corpus).DiscoverUnion(ctx, []catalog.DiscoveryQuery{{YearFrom: 2000}})
		if !errors.Is(err, context.Canceled) || got.Candidates != nil || got.WindowsCompleted != 0 || got.SourceQueriesDispatched != 0 {
			t.Fatalf("result = %+v, err = %v", got, err)
		}
		if calls := corpus.Discoveries(); len(calls) != 0 {
			t.Fatalf("dispatches = %d, want zero", len(calls))
		}
	})

	t.Run("source failure returns no partial candidates", func(t *testing.T) {
		corpus := &catalogfixture.Corpus{DiscoverFunc: func(_ context.Context, q catalog.DiscoveryQuery, _ int) ([]catalog.Candidate, error) {
			if q.YearFrom == 2001 {
				return nil, catalogfixture.ErrDiscover
			}
			return []catalog.Candidate{{MediaType: provision.Movie, TMDBID: q.YearFrom, Name: "partial"}}, nil
		}}
		got, err := catalog.New(nil, corpus).DiscoverUnion(context.Background(), []catalog.DiscoveryQuery{{YearFrom: 2000}, {YearFrom: 2001}, {YearFrom: 2002}})
		if !errors.Is(err, catalogfixture.ErrDiscover) || got.Candidates != nil || got.WindowsCompleted != 1 || got.SourceQueriesDispatched != 2 {
			t.Fatalf("result = %+v, err = %v", got, err)
		}
		if calls := corpus.Discoveries(); len(calls) != 2 {
			t.Fatalf("dispatches = %d, want no later dispatch", len(calls))
		}
	})

	t.Run("cancellation prevents remaining dispatch", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		corpus := &catalogfixture.Corpus{DiscoverFunc: func(_ context.Context, _ catalog.DiscoveryQuery, _ int) ([]catalog.Candidate, error) {
			cancel()
			return nil, nil
		}}
		got, err := catalog.New(nil, corpus).DiscoverUnion(ctx, []catalog.DiscoveryQuery{{YearFrom: 2000}, {YearFrom: 2001}})
		if !errors.Is(err, context.Canceled) || got.Candidates != nil || got.WindowsCompleted != 1 || got.SourceQueriesDispatched != 1 {
			t.Fatalf("result = %+v, err = %v", got, err)
		}
	})
}

func TestCatalogDiscoverUnion_OneWindowMatchesDiscover(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{
		{MediaType: provision.Movie, TMDBID: 90000001, Name: "One"}, {MediaType: provision.Movie, TMDBID: 90000002, Name: "Two"},
	}}
	query := catalog.DiscoveryQuery{MediaType: provision.Movie, YearFrom: 2000, YearTo: 2001}
	want, err := catalog.New(nil, corpus).Discover(context.Background(), query, 24)
	if err != nil {
		t.Fatal(err)
	}
	got, err := catalog.New(nil, corpus).DiscoverUnion(context.Background(), []catalog.DiscoveryQuery{query})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Candidates) != len(want) {
		t.Fatalf("union length = %d, want %d", len(got.Candidates), len(want))
	}
	for i := range want {
		if got.Candidates[i].TMDBID != want[i].TMDBID {
			t.Fatalf("union[%d] = %d, want %d", i, got.Candidates[i].TMDBID, want[i].TMDBID)
		}
	}
}
