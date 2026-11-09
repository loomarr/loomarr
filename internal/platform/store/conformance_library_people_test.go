package store

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"
)

// Who gets a filmography (decision 50), when it is fetched again, and people no title credits.
func testLibraryFilmography(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	sqlSt, ok := st.(*sqlStore)
	if !ok {
		t.Skip("reads rows directly")
	}
	at := time.UnixMilli(1_800_000_000_000)
	// billedEleventh puts a credit after ten unlinked extras, so it is billed eleventh.
	billedEleventh := func(c ...LibraryCredit) []LibraryCredit {
		var out []LibraryCredit
		for i := range 10 {
			out = append(out, LibraryCredit{Name: fmt.Sprintf("Extra %d", i), Role: "actor"})
		}
		return append(out, c...)
	}
	tmdb := func(v string) LibraryID { return LibraryID{Namespace: "tmdb", Value: v} }
	low := LibraryCredit{Name: "Low Billed", Role: "actor", ID: tmdb("90004002")}
	m1 := billedEleventh(low,
		LibraryCredit{Name: "Lonely Extra", Role: "actor", ID: tmdb("90004004")},
		LibraryCredit{Name: "The Director", Role: "director", ID: tmdb("90004003")},
		LibraryCredit{Name: "Other Director", Role: "director", ID: LibraryID{Namespace: "imdb", Value: "nm90004005"}})
	m1[0] = LibraryCredit{Name: "The Lead", Role: "actor", ID: tmdb("90004001")}
	items := []LibraryItem{
		{ID: "m1", Kind: "movie", Name: "One", Credits: m1},
		{ID: "m2", Kind: "movie", Name: "Two", Credits: billedEleventh(low)},
		{ID: "m3", Kind: "movie", Name: "Three", Credits: billedEleventh(low)},
	}
	if err := st.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	due := func(d LibraryFilmographyDue) []string {
		t.Helper()
		refs, err := st.DueLibraryFilmographies(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range refs {
			out = append(out, r.ID+"="+r.External.Value)
		}
		return out
	}
	monthly := LibraryFilmographyDue{Namespace: "tmdb", Now: at, StaleBefore: at.Add(-30 * 24 * time.Hour), Limit: 10}
	// The lead and the director by billing, the eleventh-billed actor by three titles; not the
	// eleventh-billed actor of one title, nor anyone without an id in the catalog's namespace.
	want := []string{"tmdb:90004001=90004001", "tmdb:90004002=90004002", "tmdb:90004003=90004003"}
	if got := due(monthly); !slices.Equal(got, want) {
		t.Fatalf("due = %v, want %v", got, want)
	}
	if got := due(LibraryFilmographyDue{Namespace: "tmdb", Now: at, StaleBefore: at, Limit: 1}); len(got) != 1 {
		t.Fatalf("due with a limit of one = %v", got)
	}
	if got := due(LibraryFilmographyDue{Namespace: "imdb", Now: at, StaleBefore: at, Limit: 10}); !slices.Equal(got,
		[]string{"imdb:nm90004005=nm90004005"}) {
		t.Fatalf("due in another catalog = %v, want the one director it knows", got)
	}

	entries := []LibraryFilmEntry{
		{Kind: "movie", ID: tmdb("90000601"), Title: "The Lantern Archive", Year: 2019, Role: "actor", Character: "Night Clerk"},
		{Kind: "series", ID: tmdb("90000701"), Title: "Tidewater Station", Year: 2021, Role: "actor"},
	}
	if err := st.SaveLibraryFilmography(ctx, "tmdb:90004001", entries, at); err != nil {
		t.Fatal(err)
	}
	if err := st.FailLibraryFilmography(ctx, "tmdb:90004002", at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Fetched this month, the lead waits; held back, the low-billed actor waits too.
	if got := due(monthly); !slices.Equal(got, []string{"tmdb:90004003=90004003"}) {
		t.Fatalf("due = %v, want only the director", got)
	}
	// A month on, the lead's is due again, after the director, who was never fetched.
	later := LibraryFilmographyDue{Namespace: "tmdb", Now: at, StaleBefore: at.Add(time.Minute), Limit: 10}
	if got := due(later); !slices.Equal(got, []string{"tmdb:90004003=90004003", "tmdb:90004001=90004001"}) {
		t.Fatalf("due a month on = %v", got)
	}
	count := func(query string) int {
		t.Helper()
		var n int
		if err := sqlSt.db.QueryRowContext(ctx, query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT COUNT(*) FROM library_filmography WHERE person_id = 'tmdb:90004001' AND
		((position = 0 AND kind = 'movie' AND namespace = 'tmdb' AND external_id = '90000601' AND year = 2019
			AND character_name = 'Night Clerk') OR
		 (position = 1 AND kind = 'series' AND namespace = 'tmdb' AND external_id = '90000701'))`); n != 2 {
		t.Fatalf("filmography rows = %d, want 2", n)
	}
	if err := st.SaveLibraryFilmography(ctx, "tmdb:90004001", entries[:1], at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT COUNT(*) FROM library_filmography`); n != 1 {
		t.Fatalf("filmography rows after a refresh = %d, want it replaced", n)
	}

	// Titles rewritten without credits leave their people uncredited: pruned, filmographies too.
	for i := range items {
		items[i].Credits = nil
	}
	if err := st.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	if n, err := st.PruneLibraryPeople(ctx); err != nil || n != 15 {
		t.Fatalf("pruned %d, %v; want ten extras and five linked people", n, err)
	}
	if n := count(`SELECT COUNT(*) FROM library_people`) + count(`SELECT COUNT(*) FROM library_filmography`); n != 0 {
		t.Fatalf("rows left after pruning = %d", n)
	}
}
