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

	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout"
)

var homeVideoOrigin = librarystore.LibraryOrigin{Authority: "library:/media", Key: "/media/a.mkv"}

// homePosterFixture is a library with one title whose only file is path, ms long, not yet kept.
func homePosterFixture(t *testing.T, path string, ms int64) (*homePosters, func(), func() librarystore.LibraryItemSummary) {
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
	p := &homePosters{store: st, images: svc, log: slog.New(slog.DiscardHandler), now: func() time.Time { return at }}
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
	p.still = func(_ context.Context, src playout.StillSource, offset time.Duration) ([]byte, error) {
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

	pass(0, 0) // not kept
	keep()
	p.busy = func() bool { return true }
	pass(0, 0) // someone is watching
	p.busy = nil
	pass(0, 1)
	pass(homePosterRetry-time.Second, 1)
	pass(homePosterRetry, 2)
	pass(2*homePosterRetry, 2) // the second wait is twice as long
	failing = false
	pass(3*homePosterRetry, 3)
	if offsets[0] != 20*time.Second {
		t.Fatalf("frame at %s, want a tenth of 200s", offsets[0])
	}
	rec, err := p.images.Get(ctx, item().Poster)
	if err != nil || rec.Role != images.RolePoster || rec.Origin != images.OriginExtracted {
		t.Fatalf("poster = %+v, %v", rec, err)
	}
	pass(time.Hour, 3) // it has its poster
}
