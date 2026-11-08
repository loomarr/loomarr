package scan

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// A title is listed from its present scanner origin over a newer missing one, never from another
// authority's, and not at all by an origin written before it carried how it is listed.
func TestProjectLibraryItemPicksThePresentScannerOrigin(t *testing.T) {
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	listed := func(page string) json.RawMessage {
		raw, _ := json.Marshal(originExtension{Listed: &listedExtension{
			Catalog: store.LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000801"},
			Match:   store.LibraryMatch{Basis: "manager", App: "radarr", Page: page},
		}})
		return raw
	}
	origin := func(authority, key, name string, observed time.Time, missing bool, ext json.RawMessage) inventory.ItemOrigin {
		o := inventory.ItemOrigin{Key: inventory.OriginKey{Authority: inventory.AuthorityID(authority), ExternalItemID: key},
			Observation: inventory.Observation[inventory.ItemFacts]{ObservedAt: observed, Extension: ext,
				Facts: inventory.ItemFacts{Name: name, SortTitle: "Sort " + name, ProductionYear: 2001}}}
		if missing {
			o.MissingAt = observed
		}
		return o
	}
	present := origin(authorityPrefix+"a", "file:1", "Kept Film", at, false, listed("https://radarr.invalid/1"))
	gone := origin(authorityPrefix+"a", "file:2", "Gone Film", at.Add(time.Hour), true, listed("https://radarr.invalid/2"))
	other := origin("emby", "90000802", "Server Film", at.Add(2*time.Hour), false, nil)

	got, ok := ProjectLibraryItem(inventory.Item{ID: "m1", Kind: kindMovie,
		Origins: []inventory.ItemOrigin{gone, other, present}})
	if !ok {
		t.Fatal("a movie with a present scanner origin is not listed")
	}
	want := store.LibraryItem{ID: "m1", Kind: "movie", Name: "Kept Film", SortName: "sort kept film", Year: 2001,
		Catalog: store.LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000801"},
		Origin:  store.LibraryOrigin{Authority: authorityPrefix + "a", Key: "file:1"},
		Match:   store.LibraryMatch{Basis: "manager", App: "radarr", Page: "https://radarr.invalid/1"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listed = %+v\nwant     %+v", got, want)
	}

	if _, ok := ProjectLibraryItem(inventory.Item{ID: "m2", Kind: kindMovie, Origins: []inventory.ItemOrigin{other}}); ok {
		t.Error("a movie with no scanner origin is listed")
	}
	old := origin(authorityPrefix+"a", "file:3", "Old Film", at, false, json.RawMessage(`{"from":{}}`))
	if _, ok := ProjectLibraryItem(inventory.Item{ID: "m3", Kind: kindMovie, Origins: []inventory.ItemOrigin{old}}); ok {
		t.Error("an origin written before write version 10 is listed; its row would lose its match")
	}
	// An episode is listed with its numbering and its series' origin; its genres and studios are
	// its show's, so it has none of its own (decision 42).
	epExt, _ := json.Marshal(originExtension{Listed: &listedExtension{
		Series: store.LibraryOrigin{Authority: authorityPrefix + "a", Key: "series:dir:1"}}})
	ep := origin(authorityPrefix+"a", "file:4", "Pilot", at, false, epExt)
	ep.Observation.Facts.SeasonNumber, ep.Observation.Facts.EpisodeNumber = 1, 1
	ep.Observation.Facts.Genres, ep.Observation.Facts.Studios = []string{"Drama"}, []string{"Network"}
	got, ok = ProjectLibraryItem(inventory.Item{ID: "e1", Kind: kindEpisode, Origins: []inventory.ItemOrigin{ep}})
	if !ok || got.Season != 1 || got.Episode != 1 || got.Series.Key != "series:dir:1" || got.Genres != nil || got.Studios != nil {
		t.Errorf("episode = %+v, %v; want it listed under its series with no genres or studios", got, ok)
	}
}
