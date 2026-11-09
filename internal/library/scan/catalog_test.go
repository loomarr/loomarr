package scan

import (
	"testing"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// A title's catalog row is whatever the catalog provider declares it fetches, in its order: the
// same ids key a TMDB-shaped catalog by TMDB and an IMDb-shaped one by IMDb, and a kind or ids it
// fetches nothing by have no row.
func TestCatalogKeysFollowTheProvider(t *testing.T) {
	ids := []mp.ExternalID{
		{Namespace: mp.NamespaceIMDB, Value: "tt90000101"},
		{Namespace: mp.NamespaceTMDB, Value: "90000101"},
		{Namespace: mp.NamespaceTVDB, Value: "90000102"},
	}
	imdb := CatalogKeys{{Kind: mp.KindMovie, Namespace: mp.NamespaceIMDB}}
	for _, tc := range []struct {
		name string
		keys CatalogKeys
		kind mp.Kind
		ids  []mp.ExternalID
		want store.LibraryCatalogKey
	}{
		{"tmdb movie", fakeCatalogKeys, mp.KindMovie, ids, store.LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000101"}},
		{"tmdb episode by its tvdb id", fakeCatalogKeys, mp.KindEpisode, ids, store.LibraryCatalogKey{Provider: "tvdb", Kind: "episode", ExternalID: "90000102"}},
		{"imdb movie", imdb, mp.KindMovie, ids, store.LibraryCatalogKey{Provider: "imdb", Kind: "movie", ExternalID: "tt90000101"}},
		{"kind not fetched", imdb, mp.KindSeries, ids, store.LibraryCatalogKey{}},
		{"empty id skipped", imdb, mp.KindMovie, []mp.ExternalID{{Namespace: mp.NamespaceIMDB}}, store.LibraryCatalogKey{}},
		{"no catalog", nil, mp.KindMovie, ids, store.LibraryCatalogKey{}},
	} {
		if got := tc.keys.For(tc.kind, tc.ids); got != tc.want {
			t.Errorf("%s: For = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
