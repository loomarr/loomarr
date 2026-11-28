package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout"
)

func homeFrame(t *testing.T, width int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, width, 18))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// saveHomeVideo writes the fixture's title with its file at revision.
func saveHomeVideo(t *testing.T, p *homePosters, revision string) {
	t.Helper()
	if err := p.store.(records.Store).SaveLibraryItems(context.Background(), []librarystore.LibraryItem{{ID: "90000001", Kind: "movie", Name: "Alpha",
		Origin: homeVideoOrigin, Sources: []librarystore.LibrarySource{
			{Path: "/media/a.mkv", DurationMs: 200_000, Plays: true, Revision: revision}}}}, p.now()); err != nil {
		t.Fatal(err)
	}
}

// Two frames extracted at once for one title leave it one poster.
func TestHomeVideoPosterRacingWrites(t *testing.T) {
	ctx := context.Background()
	p, keep, _ := homePosterFixture(t, "/media/a.mkv", 200_000)
	keep()
	var both sync.WaitGroup
	both.Add(2)
	var done sync.WaitGroup
	for _, width := range []int{32, 48} {
		frame := homeFrame(t, width)
		racer := *p
		racer.still = func(context.Context, playout.StillSource, time.Duration) ([]byte, error) {
			both.Done()
			both.Wait()
			return frame, nil
		}
		done.Add(1)
		go func() {
			defer done.Done()
			if err := racer.pass(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	done.Wait()
	owned, err := p.store.(records.Store).ImagesForOwner(ctx, librarystore.LibraryItemImageOwner, "90000001")
	if err != nil || len(owned) != 1 {
		t.Fatalf("owned = %d images, %v; want one poster", len(owned), err)
	}
}

// A replaced file of the same length does not wait out the failures of the one it replaced.
func TestHomeVideoPosterBackoffBelongsToTheFile(t *testing.T) {
	ctx := context.Background()
	p, keep, _ := homePosterFixture(t, "/media/a.mkv", 200_000)
	saveHomeVideo(t, p, "stat:1:1")
	keep()
	attempts := 0
	p.still = func(context.Context, playout.StillSource, time.Duration) ([]byte, error) {
		attempts++
		return nil, errors.New("unreadable")
	}
	pass := func(want int) {
		t.Helper()
		if err := p.pass(ctx); err != nil {
			t.Fatal(err)
		}
		if attempts != want {
			t.Fatalf("%d attempts, want %d", attempts, want)
		}
	}
	pass(1)
	pass(1)
	saveHomeVideo(t, p, "stat:2:2")
	pass(2)
}

// Writing a kept title keeps its frame while it plays the same file, and clears it when it does not.
func TestHomeVideoPosterSurvivesARewrite(t *testing.T) {
	ctx := context.Background()
	p, keep, item := homePosterFixture(t, "/media/a.mkv", 200_000)
	saveHomeVideo(t, p, "stat:1:1")
	keep()
	frame := homeFrame(t, 32)
	p.still = func(context.Context, playout.StillSource, time.Duration) ([]byte, error) { return frame, nil }
	if err := p.pass(ctx); err != nil {
		t.Fatal(err)
	}
	hash := item().Poster
	if hash == "" {
		t.Fatal("no poster")
	}
	// What the scanner does for a title with no artwork of its own.
	rewrite := func(revision string) {
		t.Helper()
		saveHomeVideo(t, p, revision)
		art := libraryArtworkOf(p.images, p.store.(records.Store))
		if err := art.Art(ctx, "90000001", metadataprovider.KindPoster, ""); err != nil {
			t.Fatal(err)
		}
	}
	rewrite("stat:1:1")
	if got := item().Poster; got != hash {
		t.Fatalf("after rewriting the same file, poster = %q, want %q", got, hash)
	}
	rewrite("stat:2:2")
	if got := item().Poster; got != "" {
		t.Fatalf("after the file changed, poster = %q, want none", got)
	}
}
