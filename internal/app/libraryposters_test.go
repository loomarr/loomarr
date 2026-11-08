package app

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func writePoster(t *testing.T, path string, shade uint8) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 20, 30))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	img.Set(0, 0, color.RGBA{A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A scanned title's poster lands in the image service with the title as its owner: a file beside
// the media is imported once, a manager's URL is adopted, a new poster replaces the old one, none
// lets it go, and a symlink is never followed.
func TestLibraryPosters(t *testing.T) {
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	svc := newTestImageService(t, t.TempDir(), "", imageStore{st})
	posters := libraryPostersOf(svc, st)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := st.SaveLibraryItems(ctx, []store.LibraryItem{{ID: "90000001", Kind: "movie", Name: "Alpha"}}, at); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(ctx, []store.LibraryFile{{Path: "/media/a.mkv", Root: "/media", Size: 1,
		ModifiedNs: 1, Kind: "movie", Header: store.LibraryHeaderRead, SeenAt: at, ItemID: "90000001"}}); err != nil {
		t.Fatal(err)
	}
	poster := func() string {
		t.Helper()
		it, err := st.LibraryItem(ctx, "90000001")
		if err != nil {
			t.Fatal(err)
		}
		return it.Poster
	}

	dir := t.TempDir()
	first := filepath.Join(dir, "poster.png")
	writePoster(t, first, 40)
	if err := posters.Poster(ctx, "90000001", first); err != nil {
		t.Fatal(err)
	}
	hash := poster()
	rec, err := svc.Get(ctx, hash)
	if err != nil || rec.Role != images.RolePoster || rec.Origin != images.OriginExtracted || rec.Width != 20 {
		t.Fatalf("imported = %+v, %v", rec, err)
	}
	// The same bytes again keep the same image.
	if err := posters.Poster(ctx, "90000001", first); err != nil || poster() != hash {
		t.Fatalf("again = %q, %v; want %q", poster(), err, hash)
	}

	second := filepath.Join(dir, "Alpha-poster.png")
	writePoster(t, second, 200)
	if err := posters.Poster(ctx, "90000001", second); err != nil {
		t.Fatal(err)
	}
	if got := poster(); got == hash || got == "" {
		t.Fatalf("replaced poster = %q, want a new image", got)
	}
	if owned, err := st.ImagesForOwner(ctx, store.LibraryItemImageOwner, "90000001"); err != nil || len(owned) != 1 {
		t.Fatalf("owned = %d images, %v; want only the new poster", len(owned), err)
	}

	link := filepath.Join(dir, "link-poster.png")
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	before := poster()
	if err := posters.Poster(ctx, "90000001", link); err == nil || poster() != before {
		t.Fatalf("symlink = %v, poster %q; want refused and unchanged", err, poster())
	}

	const remote = "https://image.example.org/t/p/original/alpha.jpg"
	if err := posters.Poster(ctx, "90000001", remote); err != nil {
		t.Fatal(err)
	}
	if rec, err := svc.Get(ctx, poster()); err != nil || rec.SourceURL != remote {
		t.Fatalf("adopted = %+v, %v", rec, err)
	}

	if err := posters.Poster(ctx, "90000001", ""); err != nil || poster() != "" {
		t.Fatalf("cleared = %q, %v", poster(), err)
	}
	if err := posters.Poster(ctx, "90000001", "relative/poster.png"); err == nil {
		t.Fatal("a relative path was accepted")
	}
}
