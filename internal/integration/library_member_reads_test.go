package integration_test

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// The Library titles a household member's other routes read follow the Library's member rule
// (#4476): search, channel ideas, the collection picker's sizes and a movie collection's
// in-Library marks leave out a title waiting in Needs you and one the admin ignored. An admin,
// and the suggester's grounding, read every title as before.
const (
	quillCollection   = 90007100
	ignoredTMDB       = 90000603 // the TMDB stand-in's "Marigold Protocol"
	ignoredItem       = "lib-ignored"
	unidentifiedItem  = "lib-unidentified"
	firstIdentified   = "lib-q1"
	firstIdentifiedID = 90007001
)

func seedMemberRuleLibrary(t *testing.T, h *harness) {
	t.Helper()
	ctx := context.Background()
	at := time.Unix(1_800_000_000, 0).UTC()
	origin := func(key string) librarystore.LibraryOrigin {
		return librarystore.LibraryOrigin{Authority: "library:/movies", Key: key}
	}
	grounded := librarystore.LibraryMatch{Basis: "embedded_id"}
	animation := []string{"Animation"}
	var items []librarystore.LibraryItem
	for n := 1; n <= 6; n++ {
		items = append(items, librarystore.LibraryItem{
			ID: "lib-q" + strconv.Itoa(n), Kind: "movie", Name: "Quill Harbor " + strconv.Itoa(n), Year: 1990 + n,
			Genres: animation, Origin: origin("/movies/q" + strconv.Itoa(n) + ".mkv"), Match: grounded,
			IDs: []librarystore.LibraryID{{Namespace: "tmdb", Value: strconv.Itoa(firstIdentifiedID - 1 + n)}},
		})
	}
	items = append(items,
		librarystore.LibraryItem{ID: ignoredItem, Kind: "movie", Name: "Marigold Protocol", Year: 1999, Genres: animation,
			Origin: origin("/movies/ignored.mkv"), Match: grounded,
			IDs: []librarystore.LibraryID{{Namespace: "tmdb", Value: strconv.Itoa(ignoredTMDB)}}},
		librarystore.LibraryItem{ID: unidentifiedItem, Kind: "movie", Name: "marigold extras folder", Genres: animation,
			Origin: origin("/movies/marigold extras folder/x.mkv")},
	)
	if err := h.store.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	files := make([]librarystore.LibraryFile, 0, len(items))
	for _, it := range items {
		files = append(files, librarystore.LibraryFile{Path: it.Origin.Key, Root: "/movies", Size: 1, ModifiedNs: 1,
			Kind: "movie", Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: it.ID})
	}
	if err := h.store.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SaveLibraryAnswer(ctx, librarystore.LibraryAnswer{
		Origin: origin("/movies/ignored.mkv"), Kind: "movie", Ignored: true, AnsweredAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.CreateLibraryCollection(ctx, librarystore.LibraryCollection{
		Name: "Quill Shelf", Members: []string{firstIdentified, unidentifiedItem, ignoredItem},
	}, at); err != nil {
		t.Fatal(err)
	}
	h.tmdb.AddMovie(firstIdentifiedID, "Quill Harbor 1", 1991, []int{16}, "A lighthouse keeper's first winter.")
	h.tmdb.SetMovieCollection(quillCollection, "Quill Collection", firstIdentifiedID, ignoredTMDB)
}

type searchWire struct {
	Candidates []struct {
		TMDBID        int    `json:"tmdbId"`
		Name          string `json:"name"`
		InLibrary     bool   `json:"inLibrary"`
		LibraryItemID string `json:"libraryItemId"`
	} `json:"candidates"`
}

func TestMemberRoutesReadOnlyIdentifiedLibraryTitles(t *testing.T) {
	h := newHarness(t)
	admin := h.asAdmin()
	member := h.asMember(admin, "Fixture Member", fixtureMemberID)
	seedMemberRuleLibrary(t, h)

	libraryHits := func(cookie *http.Cookie) []string {
		t.Helper()
		var got searchWire
		h.getJSON("/v1/search?q=marigold&scope=library", cookie, &got)
		ids := []string{}
		for _, c := range got.Candidates {
			ids = append(ids, c.LibraryItemID)
		}
		slices.Sort(ids)
		return ids
	}
	if got, want := libraryHits(admin), []string{ignoredItem, unidentifiedItem}; !slices.Equal(got, want) {
		t.Errorf("admin library search = %v, want %v", got, want)
	}
	if got := libraryHits(member); len(got) != 0 {
		t.Errorf("member library search = %v, want none: one waits in Needs you, one is ignored", got)
	}

	// Federated search: the TMDB result for the ignored title is marked in the Library only for
	// the admin.
	marked := func(cookie *http.Cookie) (found, inLibrary bool, itemID string, unidentified bool) {
		t.Helper()
		var got searchWire
		h.getJSON("/v1/search?q=marigold&scope=all", cookie, &got)
		for _, c := range got.Candidates {
			unidentified = unidentified || c.LibraryItemID == unidentifiedItem
			if c.TMDBID == ignoredTMDB {
				found, inLibrary, itemID = true, c.InLibrary, c.LibraryItemID
			}
		}
		return found, inLibrary, itemID, unidentified
	}
	if found, in, id, unidentified := marked(admin); !found || !in || id != ignoredItem || !unidentified {
		t.Errorf("admin federated search: found %v, inLibrary %v, item %q, unidentified %v; want the ignored title marked and the unidentified one listed",
			found, in, id, unidentified)
	}
	if found, in, id, unidentified := marked(member); !found || in || id != "" || unidentified {
		t.Errorf("member federated search: found %v, inLibrary %v, item %q, unidentified %v; want TMDB's title, not in the Library, and no unidentified title",
			found, in, id, unidentified)
	}

	// Structured discovery: the presence backfill marks the ignored title only for the admin.
	discovered := func(cookie *http.Cookie) (found, inLibrary bool, itemID string) {
		t.Helper()
		var got searchWire
		h.getJSON("/v1/search?media_type=movie&genres=Science%20Fiction&scope=all", cookie, &got)
		for _, c := range got.Candidates {
			if c.TMDBID == ignoredTMDB {
				return true, c.InLibrary, c.LibraryItemID
			}
		}
		return false, false, ""
	}
	if found, in, id := discovered(admin); !found || !in || id != ignoredItem {
		t.Errorf("admin discovery: found %v, inLibrary %v, item %q; want the ignored title marked", found, in, id)
	}
	if found, in, id := discovered(member); !found || in || id != "" {
		t.Errorf("member discovery: found %v, inLibrary %v, item %q; want TMDB's title, not in the Library", found, in, id)
	}

	// Channel ideas: the Animation idea counts the ignored title only for the admin.
	ideaKeys := func(cookie *http.Cookie) []string {
		t.Helper()
		var got struct {
			Ideas []struct {
				Facet string   `json:"facet"`
				Value string   `json:"value"`
				Keys  []string `json:"keys"`
			} `json:"ideas"`
		}
		h.getJSON("/v1/discovery/ideas", cookie, &got)
		for _, idea := range got.Ideas {
			if idea.Facet == "genre" && idea.Value == "Animation" {
				return idea.Keys
			}
		}
		t.Fatalf("no Animation idea in %+v", got.Ideas)
		return nil
	}
	ignoredKey := "movie:tmdb:" + strconv.Itoa(ignoredTMDB)
	if keys := ideaKeys(admin); len(keys) != 7 || !slices.Contains(keys, ignoredKey) {
		t.Errorf("admin Animation idea = %v, want the six identified titles and the ignored one", keys)
	}
	if keys := ideaKeys(member); len(keys) != 6 || slices.Contains(keys, ignoredKey) {
		t.Errorf("member Animation idea = %v, want the six identified titles only", keys)
	}

	// The collection picker's size counts only what a member reads.
	childCount := func(cookie *http.Cookie) int {
		t.Helper()
		var got struct {
			Collections []struct {
				Name       string `json:"name"`
				ChildCount int    `json:"childCount"`
			} `json:"collections"`
		}
		h.getJSON("/v1/library/collections", cookie, &got)
		if len(got.Collections) != 1 {
			t.Fatalf("collections = %+v, want one", got.Collections)
		}
		return got.Collections[0].ChildCount
	}
	if got := childCount(admin); got != 3 {
		t.Errorf("admin picker size = %d, want 3", got)
	}
	if got := childCount(member); got != 1 {
		t.Errorf("member picker size = %d, want 1: one title waits in Needs you, one is ignored", got)
	}

	// A movie collection's roster marks the ignored title in the Library only for the admin.
	rosterMark := func(cookie *http.Cookie) (identified, ignored bool) {
		t.Helper()
		var got struct {
			Collections []struct {
				Members []struct {
					TMDBID    int  `json:"tmdbId"`
					InLibrary bool `json:"inLibrary"`
				} `json:"members"`
			} `json:"collections"`
		}
		h.getJSON("/v1/movie-collections?key=movie:tmdb:"+strconv.Itoa(firstIdentifiedID), cookie, &got)
		if len(got.Collections) != 1 {
			t.Fatalf("movie collections = %+v, want one", got.Collections)
		}
		for _, m := range got.Collections[0].Members {
			switch m.TMDBID {
			case firstIdentifiedID:
				identified = m.InLibrary
			case ignoredTMDB:
				ignored = m.InLibrary
			}
		}
		return identified, ignored
	}
	if identified, ignored := rosterMark(admin); !identified || !ignored {
		t.Errorf("admin roster in Library: identified %v, ignored %v; want both", identified, ignored)
	}
	if identified, ignored := rosterMark(member); !identified || ignored {
		t.Errorf("member roster in Library: identified %v, ignored %v; want the identified one only", identified, ignored)
	}
}
