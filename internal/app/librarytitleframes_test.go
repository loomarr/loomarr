package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout/hls"
)

var homeVideoOrigin = librarystore.LibraryOrigin{Authority: "library:/media", Key: "/media/a.mkv"}

// homePosterFixture is a library with one title whose only file is path, ms long, not yet kept.
func homePosterFixture(t *testing.T, path string, ms int64) (*titleFrames, func(), func() librarystore.LibraryItemSummary) {
	t.Helper()
	ctx, st, svc := artworkFixture(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{{ID: "90000001", Kind: "movie", Name: "Alpha",
		Origin:  homeVideoOrigin,
		Sources: []librarystore.LibrarySource{{Path: path, DurationMs: ms, Plays: true}}}}, at); err != nil {
		t.Fatal(err)
	}
	keep := func() {
		t.Helper()
		if err := st.SaveLibraryAnswer(ctx, librarystore.LibraryAnswer{Origin: homeVideoOrigin, Kind: "movie",
			Kept: true, AnsweredAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	p := &titleFrames{store: st, images: svc, log: slog.New(slog.DiscardHandler), now: func() time.Time { return at }}
	return p, keep, func() librarystore.LibraryItemSummary { return libraryItem(t, st) }
}

// A kept title's poster is the frame a tenth of the way into its file; no other title asks for one,
// and a failure waits before it is tried again, longer each time.
func TestHomeVideoPoster(t *testing.T) {
	ctx := context.Background()
	p, keep, item := homePosterFixture(t, "/media/a.mkv", 200_000)
	at := p.now()
	var frame bytes.Buffer
	if err := png.Encode(&frame, image.NewGray(image.Rect(0, 0, 32, 18))); err != nil {
		t.Fatal(err)
	}
	var offsets []time.Duration
	failing := true
	p.still = func(_ context.Context, src hls.StillSource, offset time.Duration) ([]byte, error) {
		if src.Input != "/media/a.mkv" {
			t.Fatalf("read %q", src.Input)
		}
		offsets = append(offsets, offset)
		if failing {
			return nil, errors.New("unreadable")
		}
		return frame.Bytes(), nil
	}
	pass := func(after time.Duration, attempts int) {
		t.Helper()
		p.now = func() time.Time { return at.Add(after) }
		if err := p.pass(ctx); err != nil {
			t.Fatal(err)
		}
		if len(offsets) != attempts {
			t.Fatalf("after %s: %d attempts, want %d", after, len(offsets), attempts)
		}
	}

	// Not kept, it waits in Needs you: its card's frame is tried, and its failure is not the
	// poster's.
	pass(0, 1)
	if item().Poster != "" {
		t.Fatal("a title not kept has a poster")
	}
	keep()
	p.busy = func() bool { return true }
	pass(0, 1) // someone is watching
	p.busy = nil
	pass(0, 2)
	pass(titleFrameRetry-time.Second, 2)
	pass(titleFrameRetry, 3)
	pass(2*titleFrameRetry, 3) // the second wait is twice as long
	failing = false
	pass(3*titleFrameRetry, 4)
	if offsets[1] != 20*time.Second {
		t.Fatalf("frame at %s, want a tenth of 200s", offsets[1])
	}
	rec, err := p.images.Get(ctx, item().Poster)
	if err != nil || rec.Role != images.RolePoster || rec.Origin != images.OriginExtracted {
		t.Fatalf("poster = %+v, %v", rec, err)
	}
	pass(time.Hour, 4) // it has its poster
}

// needFrame is the fixture title's Needs you frame, "" for none or when it no longer waits.
func needFrame(t *testing.T, p *titleFrames) string {
	t.Helper()
	needs, err := p.store.(records.Combined).LibraryNeeds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range needs {
		if n.ID == "90000001" {
			return n.Frame
		}
	}
	return ""
}

// A title waiting in Needs you gets a 16:9 frame of its main file for its card, never a poster. A
// replaced file loses its frame to the sweep and gets a new one, and a kept title gets a poster
// instead and loses the card's frame.
func TestLibraryNeedFrame(t *testing.T) {
	ctx := context.Background()
	p, keep, item := homePosterFixture(t, "/media/a.mkv", 200_000)
	saveHomeVideo(t, p, "stat:1:1")
	var offsets []time.Duration
	p.still = func(_ context.Context, src hls.StillSource, offset time.Duration) ([]byte, error) {
		offsets = append(offsets, offset)
		return homeFrame(t, 32+len(offsets)), nil
	}
	pass := func(attempts int) {
		t.Helper()
		if err := p.pass(ctx); err != nil {
			t.Fatal(err)
		}
		if len(offsets) != attempts {
			t.Fatalf("%d attempts, want %d", len(offsets), attempts)
		}
	}
	pass(1)
	first := needFrame(t, p)
	rec, err := p.images.Get(ctx, first)
	if err != nil || rec.Role != images.RoleThumb || rec.Origin != images.OriginExtracted || offsets[0] != 20*time.Second {
		t.Fatalf("frame = %+v at %s, %v; want an extracted still a tenth of the way in", rec, offsets[0], err)
	}
	if item().Poster != "" {
		t.Fatal("a Needs you frame became the poster")
	}
	pass(1) // it has its frame

	// What the scanner does after it saves a replaced file's rows.
	saveHomeVideo(t, p, "stat:2:2")
	if err := p.store.(records.Combined).ClearStaleLibraryFrames(ctx); err != nil {
		t.Fatal(err)
	}
	if got := needFrame(t, p); got != "" {
		t.Fatalf("a replaced file kept its frame %q", got)
	}
	pass(2)
	if got := needFrame(t, p); got == "" || got == first {
		t.Fatalf("frame after the file changed = %q, want a new one", got)
	}

	// Kept, it stops waiting: no card frame is asked for, the sweep drops the old one, and the
	// poster is taken.
	keep()
	if err := p.store.(records.Combined).ClearStaleLibraryFrames(ctx); err != nil {
		t.Fatal(err)
	}
	pass(3)
	owned, err := p.store.(records.Combined).ImagesForOwner(ctx, librarystore.LibraryItemImageOwner, "90000001")
	if err != nil || len(owned) != 1 || owned[0].Hash != item().Poster {
		t.Fatalf("owned = %+v, %v; want only the poster", owned, err)
	}
}
