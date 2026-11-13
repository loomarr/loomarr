package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/image/webp"

	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/media/images/storeadapter"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/imagetestkit"
)

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePoster(t *testing.T, path string, shade uint8) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 20, 30))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	img.Set(0, 0, color.RGBA{A: 255})
	writePNG(t, path, img)
}

// writeClearLogo writes a clear logo as the Kodi convention has it: a wide PNG, transparent but for
// an opaque red word block in its middle.
func writeClearLogo(t *testing.T, path string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 800, 310))
	for y := 105; y < 205; y++ {
		for x := 200; x < 600; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	writePNG(t, path, img)
}

// artworkFixture is a migrated store with one listed title, and its image service.
func artworkFixture(t *testing.T) (context.Context, records.Store, *images.Service) {
	t.Helper()
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	svc := imagetestkit.NewService(t, t.TempDir(), "", storeadapter.New(st))
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{{ID: "90000001", Kind: "movie", Name: "Alpha"}}, at); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{{Path: "/media/a.mkv", Root: "/media", Size: 1,
		ModifiedNs: 1, Kind: "movie", Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: "90000001"}}); err != nil {
		t.Fatal(err)
	}
	return ctx, st, svc
}

func libraryItem(t *testing.T, st records.Store) librarystore.LibraryItemSummary {
	t.Helper()
	it, err := st.LibraryItem(context.Background(), "90000001")
	if err != nil {
		t.Fatal(err)
	}
	return it
}

// A scanned title's poster lands in the image service with the title as its owner: a file beside
// the media is imported once, a manager's URL is adopted, a new poster replaces the old one, none
// lets it go, and a symlink is never followed.
func TestLibraryPosters(t *testing.T) {
	ctx, st, svc := artworkFixture(t)
	art := libraryArtworkOf(svc, st)
	poster := func() string { return libraryItem(t, st).Poster }
	setPoster := func(ref string) error { return art.Art(ctx, "90000001", metadataprovider.KindPoster, ref) }

	dir := t.TempDir()
	first := filepath.Join(dir, "poster.png")
	writePoster(t, first, 40)
	if err := setPoster(first); err != nil {
		t.Fatal(err)
	}
	hash := poster()
	rec, err := svc.Get(ctx, hash)
	if err != nil || rec.Role != images.RolePoster || rec.Origin != images.OriginExtracted || rec.Width != 20 {
		t.Fatalf("imported = %+v, %v", rec, err)
	}
	// The same bytes again keep the same image.
	if err := setPoster(first); err != nil || poster() != hash {
		t.Fatalf("again = %q, %v; want %q", poster(), err, hash)
	}

	second := filepath.Join(dir, "Alpha-poster.png")
	writePoster(t, second, 200)
	if err := setPoster(second); err != nil {
		t.Fatal(err)
	}
	if got := poster(); got == hash || got == "" {
		t.Fatalf("replaced poster = %q, want a new image", got)
	}
	if owned, err := st.ImagesForOwner(ctx, librarystore.LibraryItemImageOwner, "90000001"); err != nil || len(owned) != 1 {
		t.Fatalf("owned = %d images, %v; want only the new poster", len(owned), err)
	}

	link := filepath.Join(dir, "link-poster.png")
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	before := poster()
	if err := setPoster(link); err == nil || poster() != before {
		t.Fatalf("symlink = %v, poster %q; want refused and unchanged", err, poster())
	}

	const remote = "https://image.example.org/t/p/original/alpha.jpg"
	if err := setPoster(remote); err != nil {
		t.Fatal(err)
	}
	if rec, err := svc.Get(ctx, poster()); err != nil || rec.SourceURL != remote {
		t.Fatalf("adopted = %+v, %v", rec, err)
	}

	if err := setPoster(""); err != nil || poster() != "" {
		t.Fatalf("cleared = %q, %v", poster(), err)
	}
	if err := setPoster("relative/poster.png"); err == nil {
		t.Fatal("a relative path was accepted")
	}
}

// A clear logo keeps its transparency (#2567): its WebP rendition still has see-through pixels,
// it has no JPEG rendition at all (one would draw it on a black box), and its dominant colour is
// its ink, not the transparent canvas around it. Setting it leaves the title's poster alone, and
// fanart lands as the backdrop.
func TestLibraryClearLogoKeepsItsAlpha(t *testing.T) {
	ctx, st, svc := artworkFixture(t)
	art := libraryArtworkOf(svc, st)
	dir := t.TempDir()
	posterPath, logoPath, fanartPath := filepath.Join(dir, "poster.png"), filepath.Join(dir, "clearlogo.png"), filepath.Join(dir, "fanart.png")
	writePoster(t, posterPath, 40)
	writeClearLogo(t, logoPath)
	writePoster(t, fanartPath, 90)
	for kind, ref := range map[string]string{
		metadataprovider.KindPoster: posterPath, metadataprovider.KindClearlogo: logoPath,
		metadataprovider.KindFanart: fanartPath,
	} {
		if err := art.Art(ctx, "90000001", kind, ref); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}

	it := libraryItem(t, st)
	if it.Poster == "" || it.Logo == "" || it.Backdrop == "" {
		t.Fatalf("title holds poster %q, logo %q, backdrop %q; want all three", it.Poster, it.Logo, it.Backdrop)
	}
	if rec, err := svc.Get(ctx, it.Backdrop); err != nil || rec.Role != images.RoleBackdrop {
		t.Errorf("backdrop = %+v, %v; want the backdrop role", rec, err)
	}
	logo, err := svc.Get(ctx, it.Logo)
	if err != nil || logo.Role != images.RoleLogo {
		t.Fatalf("logo = %+v, %v; want the logo role", logo, err)
	}
	if logo.DominantHex != "#ff0000" {
		t.Errorf("dominantHex = %s, want the ink's #ff0000: the transparent canvas must not dim it", logo.DominantHex)
	}

	rend, err := svc.Rendition(ctx, it.Logo, images.FormatWebP, 300)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(rend.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	decoded, err := webp.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := decoded.Bounds()
	if _, _, _, a := decoded.At(b.Min.X, b.Min.Y).RGBA(); a == 0xffff {
		t.Error("the WebP rendition's transparent corner is opaque: the logo lost its alpha")
	}
	if _, _, _, a := decoded.At(b.Dx()/2, b.Dy()/2).RGBA(); a != 0xffff {
		t.Error("the WebP rendition's ink is not opaque")
	}

	if _, err := svc.Rendition(ctx, it.Logo, images.FormatJPEG, 300); !errors.Is(err, images.ErrNotFound) {
		t.Errorf("JPEG rendition of a logo = %v, want ErrNotFound", err)
	}

	// Letting the logo go keeps the poster and backdrop.
	if err := art.Art(ctx, "90000001", metadataprovider.KindClearlogo, ""); err != nil {
		t.Fatal(err)
	}
	if it := libraryItem(t, st); it.Logo != "" || it.Poster == "" || it.Backdrop == "" {
		t.Errorf("after letting the logo go: poster %q, logo %q, backdrop %q", it.Poster, it.Logo, it.Backdrop)
	}
}
