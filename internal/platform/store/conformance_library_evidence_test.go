package store

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"
)

// A Library item's evidence (#3006): the asked keywords its tags hold and the asked people its
// credits name, case-insensitively and in the Library's spelling, once per person; an item naming
// none is absent, and ids are read across chunks.
func testLibraryEvidence(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_800_000_000_000)
	person := func(name, role, id string) LibraryCredit {
		return LibraryCredit{Name: name, Role: role, ID: LibraryID{Namespace: "tmdb", Value: id}}
	}
	items := []LibraryItem{
		{ID: "elf", Kind: "movie", Name: "Elf", Tags: []string{"christmas", "north pole", "Snow"},
			Credits: []LibraryCredit{person("Ada Example", "actor", "90005001"), person("Ada Example", "director", "90005001"),
				person("Bo Other", "actor", "90005002")}},
		{ID: "heist", Kind: "movie", Name: "Heist", Tags: []string{"heist"},
			Credits: []LibraryCredit{person("Bo Other", "actor", "90005002")}},
		{ID: "bare", Kind: "movie", Name: "Bare"},
	}
	if err := st.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	ids := []string{"elf", "heist", "bare"}
	for i := range 600 {
		ids = append(ids, fmt.Sprintf("absent-%d", i))
	}
	got, err := st.LibraryEvidence(ctx, ids, []string{" Christmas", "snow", "space"}, []string{"ada example"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("%d answered, want only elf: %+v", len(got), got)
	}
	if e := got["elf"]; !slices.Equal(e.Keywords, []string{"christmas", "Snow"}) || !slices.Equal(e.People, []string{"Ada Example"}) {
		t.Errorf("elf = %+v, want keywords christmas, Snow and people Ada Example once", e)
	}
	none, err := st.LibraryEvidence(ctx, ids, nil, nil)
	if err != nil || len(none) != 0 {
		t.Errorf("no terms = %+v, %v; want nothing read", none, err)
	}
}
