package scan

import (
	"testing"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
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
		want librarystore.LibraryCatalogKey
	}{
		{"tmdb movie", fakeCatalogKeys, mp.KindMovie, ids, librarystore.LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000101"}},
		{"tmdb episode by its tvdb id", fakeCatalogKeys, mp.KindEpisode, ids, librarystore.LibraryCatalogKey{Provider: "tvdb", Kind: "episode", ExternalID: "90000102"}},
		{"imdb movie", imdb, mp.KindMovie, ids, librarystore.LibraryCatalogKey{Provider: "imdb", Kind: "movie", ExternalID: "tt90000101"}},
		{"kind not fetched", imdb, mp.KindSeries, ids, librarystore.LibraryCatalogKey{}},
		{"empty id skipped", imdb, mp.KindMovie, []mp.ExternalID{{Namespace: mp.NamespaceIMDB}}, librarystore.LibraryCatalogKey{}},
		{"no catalog", nil, mp.KindMovie, ids, librarystore.LibraryCatalogKey{}},
	} {
		if got := tc.keys.For(tc.kind, tc.ids); got != tc.want {
			t.Errorf("%s: For = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
