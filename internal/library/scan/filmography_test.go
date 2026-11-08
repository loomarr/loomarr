package scan

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store"
)

type fakeFilmCatalog struct {
	credits map[string][]mp.FilmCredit
	err     error
	asked   []string
}

func (f *fakeFilmCatalog) Capabilities() mp.Capabilities {
	return mp.Capabilities{Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB}
}

func (f *fakeFilmCatalog) Filmography(_ context.Context, person string) ([]mp.FilmCredit, error) {
	f.asked = append(f.asked, person)
	if f.err != nil {
		return nil, f.err
	}
	return f.credits[person], nil
}

// A director's filmography is fetched once and kept until the refresh; a catalog that fails holds
// the person back, one not configured holds no one; and people no title credits any more go.
func TestFilmographerKeepsTheQualifyingPeoplesFilmographies(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	at := time.UnixMilli(1_800_000_000_000)
	film := store.LibraryItem{ID: "m1", Kind: "movie", Name: "Film", Credits: []store.LibraryCredit{
		{Name: "The Director", Role: "director", TMDB: "90004003"},
		{Name: "Pen Name", Role: "writer"},
	}}
	if err := st.SaveLibraryItems(ctx, []store.LibraryItem{film}, at); err != nil {
		t.Fatal(err)
	}
	catalog := &fakeFilmCatalog{err: errors.New("tmdb: 503")}
	f := &Filmographer{Store: st, Catalog: catalog, Now: func() time.Time { return at }}
	for range 2 {
		if err := f.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(catalog.asked, []string{"90004003"}) {
		t.Fatalf("asked %v over two ticks of a failing catalog, want the director once, then held back", catalog.asked)
	}

	catalog.asked, catalog.err = nil, mp.ErrNotConfigured
	f.Now = func() time.Time { return at.Add(filmographyRetry) }
	for range 2 {
		if err := f.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(catalog.asked) != 2 {
		t.Fatalf("asked %v; an unconfigured catalog holds no one back, so the director is asked each tick", catalog.asked)
	}

	catalog.asked, catalog.err = nil, nil
	catalog.credits = map[string][]mp.FilmCredit{"90004003": {
		{Kind: mp.KindMovie, TMDB: "90000602", Title: "Copper Ledger", Year: 2023, Role: mp.PersonDirector},
	}}
	for range 2 {
		if err := f.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(catalog.asked, []string{"90004003"}) {
		t.Fatalf("asked %v over two ticks, want the director once, then kept until the refresh", catalog.asked)
	}
	due, err := st.DueLibraryFilmographies(ctx, store.LibraryFilmographyDue{
		Now: f.now(), StaleBefore: f.now().Add(time.Minute), Limit: 10})
	if err != nil || len(due) != 1 || due[0].ID != "tmdb:90004003" {
		t.Fatalf("due a month on = %+v, %v; want the director, whose filmography was saved", due, err)
	}

	film.Credits = nil
	if err := st.SaveLibraryItems(ctx, []store.LibraryItem{film}, at); err != nil {
		t.Fatal(err)
	}
	if err := f.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := st.PruneLibraryPeople(ctx); err != nil || n != 0 {
		t.Fatalf("people left after a tick = %d, %v; want the tick to have removed both", n, err)
	}
}
