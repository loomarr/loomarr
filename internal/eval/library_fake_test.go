//go:build eval

package eval

import (
	"context"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
)

// fakeLibrary is an in-memory LibraryReader: the titles, facts and episodes a Loomarr database
// would record for the household's Library.
type fakeLibrary struct {
	titles   []store.LibraryTitle
	facts    map[string]store.LibraryItemFacts
	episodes map[string][]store.LibraryEpisode
}

func newFakeLibrary() *fakeLibrary {
	return &fakeLibrary{facts: map[string]store.LibraryItemFacts{}, episodes: map[string][]store.LibraryEpisode{}}
}

func (f *fakeLibrary) SearchLibraryTitles(_ context.Context, term string, limit int) ([]store.LibraryTitle, error) {
	var out []store.LibraryTitle
	for _, t := range f.titles {
		if strings.Contains(strings.ToLower(t.Name), strings.ToLower(term)) && len(out) < limit {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeLibrary) LibraryTitles(context.Context, time.Time) ([]store.LibraryTitle, error) {
	return f.titles, nil
}

func (f *fakeLibrary) LibraryTitleByProviderID(_ context.Context, kind, namespace, value string) (store.LibraryTitle, bool, error) {
	for _, t := range f.titles {
		if t.Kind == kind && store.IDIn(t.IDs, namespace) == value {
			return t, true, nil
		}
	}
	return store.LibraryTitle{}, false, nil
}

func (f *fakeLibrary) LibraryItemFacts(_ context.Context, ids []string) (map[string]store.LibraryItemFacts, error) {
	out := map[string]store.LibraryItemFacts{}
	for _, id := range ids {
		if facts, ok := f.facts[id]; ok {
			out[id] = facts
		}
	}
	return out, nil
}

func (f *fakeLibrary) LibraryEpisodes(_ context.Context, seriesID string) ([]store.LibraryEpisode, bool, error) {
	eps, ok := f.episodes[seriesID]
	return eps, ok, nil
}
