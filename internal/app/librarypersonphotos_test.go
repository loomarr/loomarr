package app

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A cast photo is adopted at w185 in the bulk class (#4476), so it is fetched after a title's
// artwork however late that arrives, and a second pass adopts nothing again.
func TestLibraryPersonPhotos(t *testing.T) {
	ctx, st, svc := artworkFixture(t)
	if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{{ID: "90000001", Kind: "movie", Name: "Alpha",
		Credits: []librarystore.LibraryCredit{{Name: "Ada Quill", Role: "actor",
			Photo: "https://image.tmdb.org/t/p/original/ada.jpg"}}}}, time.UnixMilli(1_790_000_000_000)); err != nil {
		t.Fatal(err)
	}
	art := libraryArtwork{images: svc, refs: st}
	photos := &scan.PersonPhotos{Store: st, Artwork: art}
	for range 2 {
		if err := photos.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	const poster = "https://image.example.org/alpha.jpg"
	if err := art.Art(ctx, "90000001", "poster", poster); err != nil {
		t.Fatal(err)
	}
	queue, err := st.ListImagesAwaitingFetch(ctx, time.Now().Add(time.Hour), 10)
	if err != nil || len(queue) != 2 {
		t.Fatalf("queue = %+v, %v; want the poster and one photo", queue, err)
	}
	if photo := queue[1]; queue[0].SourceURL != poster || photo.SourceURL != "https://image.tmdb.org/t/p/w185/ada.jpg" ||
		photo.Role != string(images.RolePoster) || photo.FetchPriority != int(images.FetchBulk) {
		t.Fatalf("queue = %+v; want the w185 photo behind the poster, in the bulk class", queue)
	}
}
