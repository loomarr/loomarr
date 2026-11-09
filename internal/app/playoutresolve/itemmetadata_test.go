package playoutresolve

import (
	"context"
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout"
)

type fakeItemFacts struct {
	facts map[string]store.LibraryItemFacts
	err   error
	asked []string
}

func (f *fakeItemFacts) LibraryItemFacts(_ context.Context, ids []string) (map[string]store.LibraryItemFacts, error) {
	f.asked = ids
	out := map[string]store.LibraryItemFacts{}
	for _, id := range ids {
		if got, ok := f.facts[id]; ok {
			out[id] = got
		}
	}
	return out, f.err
}

// A library item id is answered from the Library's tables; any other id is simply absent, as the
// media server's own contract has it.
func TestItemMetadataAnswersLibraryIDsFromTheTables(t *testing.T) {
	facts := &fakeItemFacts{facts: map[string]store.LibraryItemFacts{
		"lib-1": {Overview: "A film.", Genres: []string{"Drama"}, Year: 1999, Rating: "PG", RuntimeMs: 5_400_000},
	}}
	m := ItemMetadata{Facts: facts}
	got, err := m.ItemMetadataByID(context.Background(), []string{"lib-1", "90000001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("answered %v, want only the library item", got)
	}
	meta := got["lib-1"]
	if meta.Overview != "A film." || meta.Year != 1999 || meta.OfficialRating != "PG" || meta.RuntimeMs != 5_400_000 ||
		len(meta.Genres) != 1 || meta.Genres[0] != "Drama" {
		t.Errorf("lib-1 = %+v", meta)
	}
	if d, err := m.ItemDurationMs(context.Background(), "lib-1"); err != nil || d != 5_400_000 {
		t.Errorf("duration = %d, %v", d, err)
	}
	if d, err := m.ItemDurationMs(context.Background(), "90000001"); err != nil || d != 0 {
		t.Errorf("unknown id duration = %d, %v; want 0 and no error", d, err)
	}

	// The guide reads the same facts with no media server connected (r.lib is nil).
	bs := []playout.Broadcast{{LibraryItemID: "lib-1", Title: "Film"}, {LibraryItemID: "90000001", Title: "Gone"}}
	(&Resolver{itemFacts: facts}).attachMetadata(context.Background(), bs)
	if bs[0].Description != "A film." || bs[0].Year != 1999 || bs[0].Rating != "PG" || len(bs[0].Genres) != 1 {
		t.Errorf("guide entry = %+v, want the Library's facts", bs[0])
	}
	if bs[1].Description != "" || bs[1].Title != "Gone" {
		t.Errorf("unknown id = %+v, want its title only", bs[1])
	}

	facts.err = errors.New("database is locked")
	if got, err := m.ItemMetadataByID(context.Background(), []string{"lib-1"}); err == nil || len(got) != 1 {
		t.Errorf("a failing read = %v, %v; want the partial answer with its error", got, err)
	}
}
