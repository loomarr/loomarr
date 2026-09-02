package catalog_test

import (
	"context"
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

// realLibrary builds a library client pointed at the testkit media-server mock,
// exercising the pinned Emby search fixture.
func realLibrary(t *testing.T) *library.Client {
	ms := testkit.NewMediaServer(t)
	return library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
}

func TestCatalogSearch_LibraryScope_SetsInLibrary(t *testing.T) {
	lib := realLibrary(t)
	c := catalog.New(lib, nil)

	got, err := c.Search(context.Background(), "matrix", catalog.ScopeLibrary, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("expected library matches for 'matrix'")
	}
	for _, cand := range got {
		if !cand.InLibrary {
			t.Errorf("library-scope candidate not marked in_library: %+v", cand)
		}
		if cand.LibraryItemID == "" {
			t.Errorf("library candidate missing item id: %+v", cand)
		}
	}
	// The pinned fixture's first result is The Matrix (tmdb 603).
	var foundMatrix bool
	for _, cand := range got {
		if cand.TMDBID == 603 && cand.Name == "The Matrix" {
			foundMatrix = true
		}
	}
	if !foundMatrix {
		t.Error("expected The Matrix (tmdb:603) from the pinned search fixture")
	}
}

func TestCatalogSearch_MergesLibraryAndTMDB_ByID(t *testing.T) {
	lib := realLibrary(t)
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "test-key")
	c := catalog.New(lib, tm)

	// "matrix" is in BOTH the library fixture (tmdb 603) and the TMDB mock (603).
	// The merged candidate must appear ONCE, in_library=true.
	got, err := c.Search(context.Background(), "matrix", catalog.ScopeAll, 20)
	if err != nil {
		t.Fatal(err)
	}
	matrixCount := 0
	for _, cand := range got {
		if cand.TMDBID == 603 {
			matrixCount++
			if !cand.InLibrary {
				t.Error("The Matrix is in the library fixture; merged candidate must be in_library")
			}
		}
	}
	if matrixCount != 1 {
		t.Errorf("The Matrix should appear once after dedupe, got %d", matrixCount)
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

	got, err := c.Search(context.Background(), "matrix", catalog.ScopeAll, 20)
	if err != nil {
		t.Fatal(err)
	}
	var matrix *catalog.Candidate
	count := 0
	for i := range got {
		if got[i].TMDBID == 603 {
			matrix = &got[i]
			count++
		}
	}
	if count != 1 || matrix == nil {
		t.Fatalf("The Matrix should still dedupe to exactly one candidate, got %d", count)
	}
	// The merged candidate carries theme signals (genres from TMDB, overview from
	// TMDB) — the whole point of the enrichment.
	if len(matrix.Genres) == 0 {
		t.Error("merged candidate lost genres (enrichment didn't survive merge)")
	}
	if matrix.Overview == "" {
		t.Error("merged candidate lost overview")
	}
	// And it's STILL in-library (identity/merge semantics unchanged).
	if !matrix.InLibrary {
		t.Error("enrichment must not disturb in_library")
	}
}

func TestCatalogSearch_TMDBOnly_NotInLibrary(t *testing.T) {
	// A TMDB title absent from the library ("Speed") → in_library=false.
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "k")
	c := catalog.New(realLibrary(t), tm)

	got, err := c.Search(context.Background(), "speed", catalog.ScopeAll, 20)
	if err != nil {
		t.Fatal(err)
	}
	var speed *catalog.Candidate
	for i := range got {
		if got[i].Name == "Speed" {
			speed = &got[i]
		}
	}
	if speed == nil {
		t.Fatal("expected Speed from TMDB")
	}
	if speed.InLibrary {
		t.Error("Speed is not in the library fixture; should be in_library=false")
	}
	if speed.TMDBID != 100 {
		t.Errorf("Speed TMDBID = %d, want 100", speed.TMDBID)
	}
}

func TestCatalogSearch_InLibraryOrderedFirst(t *testing.T) {
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "k")
	c := catalog.New(realLibrary(t), tm)

	// "matrix" matches an in-library title; add a TMDB-only via broad-ish search
	// isn't possible with distinct terms, so just assert in-library sorts first
	// among the matrix results (all in-library here) — and that ordering is stable.
	got, _ := c.Search(context.Background(), "matrix", catalog.ScopeAll, 20)
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
			TMDBID: 10_000 + i,
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
	c := catalog.Candidate{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix"}
	k, err := c.Key()
	if err != nil || k != "movie:tmdb:603" {
		t.Errorf("Key() = %q,%v want movie:tmdb:603", k, err)
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
	rated := catalog.Candidate{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix", OfficialRating: "R"}
	unrated := catalog.Candidate{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix"}
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
	// The mock owns The Matrix (603), rated R → discovery of Action marks it in-library
	// AND carries the rating, so audience enforcement has something to judge.
	c := catalog.New(lib, tm).WithPresence(fakePresence{
		owned:   map[int]string{603: "lib-603"},
		ratings: map[int]string{603: "R"},
	})

	got, err := c.Discover(context.Background(), provision.Movie, []string{"Action"}, 1990, 1999, 20)
	if err != nil {
		t.Fatal(err)
	}
	var matrix *catalog.Candidate
	for i := range got {
		if got[i].TMDBID == 603 {
			matrix = &got[i]
		}
	}
	if matrix == nil {
		t.Fatal("The Matrix (action) should be discovered")
	}
	if !matrix.InLibrary || matrix.LibraryItemID != "lib-603" {
		t.Errorf("owned discovered title should be backfilled in-library, got InLibrary=%v id=%q", matrix.InLibrary, matrix.LibraryItemID)
	}
	// The rating MUST ride along. Dropping it here is FINDING 6: a discovered-then-
	// owned title reaches the scheduler unrated, and an audience ceiling silently
	// excludes it — a channel that goes live playing nothing (§9 dead air).
	if matrix.OfficialRating != "R" {
		t.Errorf("backfill dropped the rating: got %q, want R — an owned title reaches the scheduler unrated", matrix.OfficialRating)
	}
	// A discovered title NOT owned stays an acquisition candidate.
	for _, c := range got {
		if c.TMDBID == 100 && c.InLibrary { // Speed, not in the owned set
			t.Error("un-owned discovered title should stay not-in-library")
		}
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
			MediaType: provision.Movie, TMDBID: 20_000 + i,
			Name:   fmt.Sprintf("%c Holiday Pick %02d", 'A'+rune(i/24)*25, i),
			Genres: []string{"Family"}, Source: catalog.ScopeTMDB,
		}
		if i < 24 {
			owned[candidates[i].TMDBID] = catalog.Presence{LibraryItemID: fmt.Sprintf("owned-%02d", i)}
		}
	}
	corpus := &catalogfixture.Corpus{Candidates: candidates}
	c := catalog.New(nil, corpus).WithPresence(&catalogfixture.Presence{Hits: owned})

	got, err := c.Discover(context.Background(), provision.Movie, []string{"Family"}, 0, 0, 24)
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
		id := 30_000 + i
		tmdbServer.AddMovie(id, fmt.Sprintf("Holiday Pick %02d", i), 1990+i%10, []int{10_751}, "A family holiday gathering.")
		if i < 20 {
			owned[id] = catalog.Presence{LibraryItemID: fmt.Sprintf("owned-%02d", i)}
		}
	}
	tmdbClient := tmdb.NewWithBase(tmdbServer.URL, "test-key")
	c := catalog.New(nil, tmdbClient).WithPresence(&catalogfixture.Presence{Hits: owned})

	got, err := c.Discover(context.Background(), provision.Movie, []string{"Family"}, 0, 0, 24)
	if err != nil {
		t.Fatal(err)
	}
	var outside int
	for _, candidate := range got {
		if candidate.TMDBID >= 30_000 && !candidate.InLibrary {
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
		tmdbServer.AddMovie(50_000+i, fmt.Sprintf("Action Movie %02d", i), 2000+i%20,
			[]int{28}, "An action movie.")
	}
	// TMDB's TV genre namespace uses 10759 (Action & Adventure), not the movie
	// namespace's 28 (Action). The adapter must translate the human genre per endpoint.
	tmdbServer.AddSeries(51_000, "Action Dispatch", 2018, []int{10759}, "An action series.")
	c := catalog.New(nil, tmdb.NewWithBase(tmdbServer.URL, "test-key"))

	got, err := c.Discover(context.Background(), "", []string{"Action"}, 0, 0, 24)
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
	tmdbServer.AddKeywordMovie(41_000, "Midnight Eve", 2020, []int{35},
		"A New Year's party goes sideways.", "Christmas Eve")
	tmdbServer.AddKeywordMovie(41_001, "Snowbound Reunion", 2021, []int{35, 10_751},
		"Estranged siblings reunite during Christmas week.", "Christmas")
	tmdbClient := tmdb.NewWithBase(tmdbServer.URL, "test-key")
	c := catalog.New(nil, tmdbClient).WithPresence(&catalogfixture.Presence{})

	got, err := c.DiscoverKeywords(context.Background(), provision.Movie,
		[]string{"Christmas"}, []string{"Comedy", "Family"}, 2015, 2025, 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TMDBID != 41_001 {
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
		{Name: "One", MediaType: provision.Movie, TMDBID: 101},
		{Name: "Two", MediaType: provision.Movie, TMDBID: 202},
	}}
	primary := &catalogfixture.Presence{Hits: map[int]catalog.Presence{
		101: {LibraryItemID: "item-101"},
		202: {LibraryItemID: "item-202"},
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

	got, err := c.Discover(context.Background(), provision.Movie, nil, 0, 0, 20)
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
		{MediaType: provision.Movie, TMDBID: 4, Name: "Zulu Outside", Source: catalog.ScopeTMDB},
		{MediaType: provision.Movie, TMDBID: 3, Name: "Zulu Owned", Source: catalog.ScopeTMDB},
		{MediaType: provision.Movie, TMDBID: 2, Name: "Alpha Outside", Source: catalog.ScopeTMDB},
		{MediaType: provision.Movie, TMDBID: 1, Name: "Alpha Owned", Source: catalog.ScopeTMDB},
	}}
	c := catalog.New(nil, corpus).WithPresence(&catalogfixture.Presence{Hits: map[int]catalog.Presence{
		3: {LibraryItemID: "owned-3"}, 1: {LibraryItemID: "owned-1"},
	}})

	got, err := c.Discover(context.Background(), provision.Movie, nil, 0, 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{3, 1, 4, 2}
	if len(got) != len(want) {
		t.Fatalf("discovery count = %d, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].TMDBID != id {
			t.Fatalf("discovery order = %+v, want upstream-ranked owned %v then outside", got, want)
		}
	}
}

func TestCatalogDiscoverBreaksEqualSourceRanksByCanonicalIdentity(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{
		{MediaType: provision.Movie, TMDBID: 20, Name: "Earlier Alphabetically", RelevanceRank: 1},
		{MediaType: provision.Movie, TMDBID: 10, Name: "Later Alphabetically", RelevanceRank: 1},
	}}
	got, err := catalog.New(nil, corpus).Discover(context.Background(), provision.Movie, nil, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].TMDBID != 10 || got[1].TMDBID != 20 {
		t.Fatalf("equal-rank order = %+v, want canonical identity tie-break", got)
	}
}
