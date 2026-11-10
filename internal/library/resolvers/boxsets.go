package resolvers

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/library/provisionbackstop"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// OwnCollections is the household's collections (#2151 step 12), as the collection scope reads
// them.
type OwnCollections interface {
	LibraryCollectionMemberIDs(ctx context.Context) ([]librarystore.LibraryCollectionMember, error)
	ListLibraryCollections(ctx context.Context) ([]librarystore.LibraryCollection, error)
}

// BoxSets answers "which collections is this title in?" for the reconcile-time stamp backing
// scope.collections (programming-design §2.2), from the household's collections alone (#2151
// step 16).
//
// It builds a REVERSE INDEX in one read and caches it, rather than querying per key. Membership is
// indexed under EVERY key form via provisionbackstop.ScanItemKeys, because a member carrying both
// provider ids must be findable under whichever key the lineup entry was born with — the same
// parity problem the availability scan solves, and the same solution.
//
// The cache is refreshed on a TTL: a collection edited in the Library takes effect within it,
// which is consistent with the stamp only being read at reconcile anyway.
type BoxSets struct {
	own OwnCollections
	ttl time.Duration

	mu      sync.Mutex
	index   map[provision.Key][]string
	fetched time.Time
}

// NewBoxSets returns the box-set resolver over the household's collections, refreshing its index
// every ttl.
func NewBoxSets(own OwnCollections, ttl time.Duration) *BoxSets {
	return &BoxSets{own: own, ttl: ttl}
}

func (a *BoxSets) BoxSets(ctx context.Context, key provision.Key) ([]string, bool, error) {
	idx, err := a.ensureIndex(ctx)
	if err != nil {
		return nil, false, err // unresolved this pass; the entry stays unstamped and airs (§2.2 fail-open)
	}
	// A key absent from the index is a REAL answer — "in no collection" — not a failure. It
	// must return ok=true so the stamp settles, or every non-member re-resolves forever.
	return idx[key], true, nil
}

func (a *BoxSets) ensureIndex(ctx context.Context) (map[provision.Key][]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if a.index != nil && now.Sub(a.fetched) < a.ttl {
		return a.index, nil
	}
	members, err := a.own.LibraryCollectionMemberIDs(ctx)
	if err != nil {
		return nil, err
	}
	idx := make(map[provision.Key][]string)
	for _, m := range members {
		for _, k := range provisionbackstop.ScanItemKeys(ownMember(m)) {
			idx[k] = append(idx[k], m.CollectionID)
		}
	}
	a.index, a.fetched = idx, now
	return idx, nil
}

// ownMember reads a household collection's title as a search result, so its keys come from the
// one derivation every title uses.
func ownMember(m librarystore.LibraryCollectionMember) mediaserver.SearchResult {
	r := mediaserver.SearchResult{MediaType: mediaserver.Movie}
	if m.Kind == "series" {
		r.MediaType = mediaserver.Series
	}
	// Provisioning keys speak TMDB and TVDB (the arrs' ids), so those are read by name.
	r.TMDBID, _ = strconv.Atoi(librarystore.IDIn(m.IDs, string(mediaserver.TMDB)))
	r.TVDBID, _ = strconv.Atoi(librarystore.IDIn(m.IDs, string(mediaserver.TVDB)))
	return r
}
