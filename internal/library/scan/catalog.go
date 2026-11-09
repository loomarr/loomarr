package scan

import (
	"context"
	"encoding/json"

	"github.com/loomarr/loomarr/internal/inventory"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store"
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

// linkRecord is the answer for a movie or series nothing else grounded, as a record: the TMDB
// title the maintainer said it is, or the one a search agreed on. None, an ignore, or an answer
// that cannot be read says nothing.
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
	basis := mp.BasisLink
	if answer.By == store.AnsweredBySearch {
		basis = mp.BasisSearch
	}
	return mp.Record{
		Source: mp.Source{Basis: basis}, Kind: kind,
		IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: answer.TMDBID}},
	}, true
}

// CatalogKeys is what the catalog provider fetches, in the order it declares it
// (Capabilities.Resolves). The Library names no catalog of its own: a title's catalog row is the
// first of its ids the provider can fetch, so swapping the provider swaps the rows, not the code.
// None fetches nothing.
type CatalogKeys []mp.Resolvable

// For is the catalog row a title's details come from: the first resolvable for kind whose
// namespace one of ids is in (an episode's by its own id, never its season and number; decision
// 53). A title with none, or a kind the provider does not fetch, has the zero key. The key's
// provider is the namespace the Enricher fetches it by.
func (c CatalogKeys) For(kind mp.Kind, ids []mp.ExternalID) store.LibraryCatalogKey {
	for _, r := range c {
		if r.Kind != kind {
			continue
		}
		for _, id := range ids {
			if id.Namespace == r.Namespace && id.Value != "" {
				return store.LibraryCatalogKey{Provider: string(r.Namespace), Kind: string(kind), ExternalID: id.Value}
			}
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
