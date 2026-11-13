package app

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

type libraryTitlesFake []librarystore.LibraryTitle

func (f libraryTitlesFake) LibraryTitles(_ context.Context, since time.Time) ([]librarystore.LibraryTitle, error) {
	return f, nil
}

func (f libraryTitlesFake) LibraryTitleByProviderID(_ context.Context, kind, namespace, value string) (librarystore.LibraryTitle, bool, error) {
	for _, t := range f {
		if t.Kind == kind && librarystore.IDIn(t.IDs, namespace) == value {
			return t, true, nil
		}
	}
	return librarystore.LibraryTitle{}, false, nil
}

var testLibraryTitles = libraryTitlesFake{
	{ID: "lib-film", Kind: "movie", Name: "Film", IDs: []librarystore.LibraryID{{Namespace: "tmdb", Value: "90000001"}},
		Rating: "PG", Genres: []string{"Drama"}, SizeBytes: 4_000},
	{ID: "lib-show", Kind: "series", Name: "Show", IDs: []librarystore.LibraryID{
		{Namespace: "tmdb", Value: "90000003"}, {Namespace: "tvdb", Value: "90000002"}}, Rating: "TV-Y"},
}
