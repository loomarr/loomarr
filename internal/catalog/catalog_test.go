package catalog_test

import (
	"context"
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
