package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// personPhotos is a title with credits, the adopting pass over it, and what its page holds.
type personPhotos struct {
	t     *testing.T
	ctx   context.Context
	st    records.Store
	photo *scan.PersonPhotos
}

func newPersonPhotos(t *testing.T) personPhotos {
	t.Helper()
	ctx, st, svc := artworkFixture(t)
	return personPhotos{t: t, ctx: ctx, st: st,
		photo: &scan.PersonPhotos{Store: st, Artwork: libraryArtwork{images: svc, refs: st}}}
}

// credit writes the title's cast as name and photo URL pairs, then runs a pass.
func (p personPhotos) credit(pairs ...string) {
	p.t.Helper()
	item := librarystore.LibraryItem{ID: "90000001", Kind: "movie", Name: "Alpha"}
	for i := 0; i < len(pairs); i += 2 {
		item.Credits = append(item.Credits, librarystore.LibraryCredit{Name: pairs[i], Role: "actor", Photo: pairs[i+1]})
	}
	if err := p.st.SaveLibraryItems(p.ctx, []librarystore.LibraryItem{item}, time.UnixMilli(1_790_000_000_000)); err != nil {
		p.t.Fatal(err)
	}
	p.tick()
}

func (p personPhotos) tick() {
	p.t.Helper()
	if err := p.photo.Tick(p.ctx); err != nil {
		p.t.Fatal(err)
	}
}

func (p personPhotos) queue() []clipstore.Image {
	p.t.Helper()
	queue, err := p.st.ListImagesAwaitingFetch(p.ctx, time.Now().Add(time.Hour), 10)
	if err != nil {
		p.t.Fatal(err)
	}
	return queue
}

// land gives url's queued image its bytes as the fetch job does: the row is re-keyed to the
// content hash, which takes url as its source, and the refs follow.
func (p personPhotos) land(url, content string) {
	p.t.Helper()
	for _, img := range p.queue() {
		if img.SourceURL != url {
			continue
		}
		pending := img.Hash
		img.Hash, img.OriginFetchedAt = content, time.Now()
		if err := errors.Join(p.st.PutImage(p.ctx, img), p.st.RepointImageRefs(p.ctx, pending, content),
			p.st.DeleteImage(p.ctx, pending)); err != nil {
			p.t.Fatal(err)
		}
		return
	}
	p.t.Fatalf("%s is not queued", url)
}

func (p personPhotos) shown() []string {
	p.t.Helper()
	detail, err := p.st.LibraryItemDetail(p.ctx, "90000001")
	if err != nil {
		p.t.Fatal(err)
	}
	var out []string
	for _, c := range detail.Credits {
		out = append(out, c.PhotoHash)
	}
	return out
}

// Two people whose URLs serve the same bytes share one image row, whose source is the later URL.
// Both keep the photo, and neither is adopted again.
func TestLibraryPersonPhotosShareIdenticalBytes(t *testing.T) {
	p := newPersonPhotos(t)
	const a, b = "https://image.example.org/a.jpg", "https://image.example.org/b.jpg"
	same := strings.Repeat("a", 64)
	p.credit("Ada Quill", a, "Ben Marrow", b)
	p.land(a, same)
	p.land(b, same)
	for range 2 {
		p.tick()
		if q := p.queue(); len(q) != 0 {
			t.Fatalf("queued again: %+v", q)
		}
		if got := p.shown(); !slices.Equal(got, []string{same, same}) {
			t.Fatalf("shown = %v", got)
		}
	}
}

// A new photo takes the old one's place once its bytes are here, and a dead URL leaves the old one.
func TestLibraryPersonPhotoSwapsWhenTheNewOneArrives(t *testing.T) {
	p := newPersonPhotos(t)
	const first, second, dead = "https://image.example.org/1.jpg", "https://image.example.org/2.jpg", "https://image.example.org/dead.jpg"
	one, two := strings.Repeat("f", 64), strings.Repeat("1", 64)
	p.credit("Ada Quill", first)
	p.land(first, one)
	p.credit("Ada Quill", second)
	if got := p.shown(); !slices.Equal(got, []string{one}) {
		t.Fatalf("before the new bytes, shown = %v, want the old photo", got)
	}
	p.land(second, two)
	if got := p.shown(); !slices.Equal(got, []string{two}) {
		t.Fatalf("shown = %v, want the new photo", got)
	}
	p.tick()
	owned, err := p.st.ImagesForOwner(p.ctx, librarystore.LibraryPersonImageOwner, "name:Ada Quill")
	if err != nil || len(owned) != 1 || owned[0].Hash != two {
		t.Fatalf("owned = %+v, %v; want the new photo alone", owned, err)
	}
	p.credit("Ada Quill", dead)
	p.tick()
	if got := p.shown(); !slices.Equal(got, []string{two}) {
		t.Fatalf("with a dead URL, shown = %v, want the old photo", got)
	}
}

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
