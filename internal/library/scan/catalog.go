package scan

import (
	"context"
	"encoding/json"

	"github.com/loomarr/loomarr/internal/inventory"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/store"
)

// CatalogStore is the cache of catalog records the walk reads (#2151, decision 20). Only the
// Enricher fetches: a walk never waits on a catalog.
type CatalogStore interface {
	WantLibraryCatalog(ctx context.Context, keys []store.LibraryCatalogKey) error
	LibraryCatalogRecord(ctx context.Context, key store.LibraryCatalogKey) (string, bool, error)
}

// AnswerStore is the maintainer's Needs you answers the walk reads (#2151 step 11).
type AnswerStore interface {
	LibraryAnswerFor(ctx context.Context, origin store.LibraryOrigin) (store.LibraryAnswer, bool, error)
}

// linkRecord is the maintainer's link for a movie or series nothing else grounded, as a record:
// the TMDB title they said it is. None, an ignore, or an answer that cannot be read says nothing.
func (im *Importer) linkRecord(ctx context.Context, kind mp.Kind, origin inventory.OriginKey) (mp.Record, bool) {
	if im.Answers == nil || (kind != mp.KindMovie && kind != mp.KindSeries) {
		return mp.Record{}, false
	}
	answer, ok, err := im.Answers.LibraryAnswerFor(ctx, libraryOrigin(origin))
	if err != nil {
		im.log().Warn("library scan: answer not read", "err", err)
		return mp.Record{}, false
	}
	if !ok || answer.TMDBID == "" || answer.Kind != string(kind) {
		return mp.Record{}, false
	}
	return mp.Record{
		Source: mp.Source{Basis: mp.BasisLink}, Kind: kind,
		IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: answer.TMDBID}},
	}, true
}

// catalogKey is the catalog row a movie's or series' details come from: its TMDB id, the one
// catalog a title is fetched from. A title with none has the zero key.
func catalogKey(kind mp.Kind, ids []mp.ExternalID) store.LibraryCatalogKey {
	if kind != mp.KindMovie && kind != mp.KindSeries {
		return store.LibraryCatalogKey{}
	}
	for _, id := range ids {
		if id.Namespace == mp.NamespaceTMDB && id.Value != "" {
			return store.LibraryCatalogKey{Provider: string(mp.IDTMDB), Kind: string(kind), ExternalID: id.Value}
		}
	}
	return store.LibraryCatalogKey{}
}

// catalogRecord is the cached record for key. A key not fetched yet is noted as wanted, so the
// Enricher fetches it and the title is written again with it. A catalog that does not know the
// title, or a cache that cannot be read, says nothing.
func (im *Importer) catalogRecord(ctx context.Context, key store.LibraryCatalogKey) (mp.Record, bool) {
	if im.Catalog == nil || key.String() == "" {
		return mp.Record{}, false
	}
	raw, fetched, err := im.Catalog.LibraryCatalogRecord(ctx, key)
	if err != nil {
		im.log().Warn("library scan: catalog record not read", "catalog", key.String(), "err", err)
		return mp.Record{}, false
	}
	if !fetched {
		if err := im.Catalog.WantLibraryCatalog(ctx, []store.LibraryCatalogKey{key}); err != nil {
			im.log().Warn("library scan: catalog record not asked for", "catalog", key.String(), "err", err)
		}
		return mp.Record{}, false
	}
	if raw == "" {
		return mp.Record{}, false
	}
	var rec mp.Record
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		im.log().Warn("library scan: catalog record unreadable", "catalog", key.String(), "err", err)
		return mp.Record{}, false
	}
	return rec, true
}
