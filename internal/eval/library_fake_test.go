//go:build eval

package eval

import (
	"context"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// fakeLibrary is an in-memory LibraryReader: the titles, facts and episodes a Loomarr database
// would record for the household's Library.
type fakeLibrary struct {
	titles   []librarystore.LibraryTitle
	facts    map[string]librarystore.LibraryItemFacts
	episodes map[string][]librarystore.LibraryEpisode
}

func newFakeLibrary() *fakeLibrary {
	return &fakeLibrary{facts: map[string]librarystore.LibraryItemFacts{}, episodes: map[string][]librarystore.LibraryEpisode{}}
}

func (f *fakeLibrary) SearchLibraryTitles(_ context.Context, term string, limit int) ([]librarystore.LibraryTitle, error) {
	var out []librarystore.LibraryTitle
	for _, t := range f.titles {
		if strings.Contains(strings.ToLower(t.Name), strings.ToLower(term)) && len(out) < limit {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeLibrary) LibraryTitles(context.Context, time.Time) ([]librarystore.LibraryTitle, error) {
	return f.titles, nil
}

func (f *fakeLibrary) LibraryTitleByProviderID(_ context.Context, kind, namespace, value string) (librarystore.LibraryTitle, bool, error) {
	for _, t := range f.titles {
		if t.Kind == kind && librarystore.IDIn(t.IDs, namespace) == value {
			return t, true, nil
		}
	}
	return librarystore.LibraryTitle{}, false, nil
}

func (f *fakeLibrary) LibraryItemFacts(_ context.Context, ids []string) (map[string]librarystore.LibraryItemFacts, error) {
	out := map[string]librarystore.LibraryItemFacts{}
	for _, id := range ids {
		if facts, ok := f.facts[id]; ok {
			out[id] = facts
		}
	}
	return out, nil
}

func (f *fakeLibrary) LibraryEpisodes(_ context.Context, seriesID string) ([]librarystore.LibraryEpisode, bool, error) {
	eps, ok := f.episodes[seriesID]
	return eps, ok, nil
}
