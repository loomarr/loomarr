package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

type posterWrites []string

func (w *posterWrites) Art(_ context.Context, itemID, kind, ref string) error {
	if kind == mp.KindPoster {
		*w = append(*w, itemID+"="+ref)
	}
	return nil
}

// The revision a frame is saved against is the scanner's, so a replaced file has another one.
func TestKeptHomeVideoFrameNamesTheFilesRevision(t *testing.T) {
	const film = "Beach day (2009)/Beach day (2009).mkv"
	h, _, _ := keptTitle(t, film)
	revision := func() string {
		t.Helper()
		frames, err := h.st.HomeVideosWithoutPoster(context.Background())
		if err != nil || len(frames) != 1 || frames[0].Revision == "" {
			t.Fatalf("frames = %+v, %v; want one with a revision", frames, err)
		}
		return frames[0].Revision
	}
	before := revision()
	h.file(film, 20)
	h.run()
	if after := revision(); after == before {
		t.Fatalf("revision %q did not change with the file", after)
	}
}

// storePosters is the app's poster write over the real store: a title written with no poster of its
// own clears what it holds.
type storePosters struct{ h *harness }

func (p storePosters) Art(ctx context.Context, itemID, kind, ref string) error {
	if kind == mp.KindPoster && ref == "" {
		return p.h.st.ClearLibraryPoster(ctx, itemID)
	}
	return nil
}

// A kept show's frame goes when a pass changes its main file, and stays when it does not.
func TestKeptShowPosterFollowsItsMainFile(t *testing.T) {
	const ep1, ep2 = "Garden Diary/Season 01/Garden Diary S01E01.mkv", "Garden Diary/Season 01/Garden Diary S01E02.mkv"
	ctx := context.Background()
	// Enough episodes that removing one is not held for the maintainer to confirm.
	episodes := []string{ep1, ep2}
	for n := 3; n <= 11; n++ {
		episodes = append(episodes, fmt.Sprintf("Garden Diary/Season 01/Garden Diary S01E%02d.mkv", n))
	}
	for what, tc := range map[string]struct {
		change func(h *harness)
		main   string
	}{
		"nothing changes":    {func(*harness) {}, ep1},
		"a later episode":    {func(h *harness) { h.file("Garden Diary/Season 01/Garden Diary S01E12.mkv", 12) }, ep1},
		"an earlier episode": {func(h *harness) { h.file("Garden Diary/Season 00/Garden Diary S00E01.mkv", 12) }, "Garden Diary/Season 00/Garden Diary S00E01.mkv"},
		"a renamed main episode": {func(h *harness) {
			from := filepath.Join(h.root, filepath.FromSlash(ep1))
			if err := os.Rename(from, strings.TrimSuffix(from, ".mkv")+" 1080p.mkv"); err != nil {
				h.t.Fatal(err)
			}
		}, "Garden Diary/Season 01/Garden Diary S01E01 1080p.mkv"},
		"the main episode removed": {func(h *harness) {
			if err := os.Remove(filepath.Join(h.root, filepath.FromSlash(ep1))); err != nil {
				h.t.Fatal(err)
			}
		}, ep2},
	} {
		t.Run(what, func(t *testing.T) {
			h, _, id := keptTitle(t, episodes...)
			h.im.Artwork = storePosters{h}
			wanted := func() []librarystore.LibraryHomeVideoFrame {
				t.Helper()
				frames, err := h.st.HomeVideosWithoutPoster(ctx)
				if err != nil {
					t.Fatal(err)
				}
				return frames
			}
			before := wanted()
			if len(before) != 1 || before[0].ItemID != id {
				t.Fatalf("wanted = %+v, want %s", before, id)
			}
			at := h.im.now()
			img := clipstore.Image{Hash: strings.Repeat("f", 64), Origin: "extracted", Visibility: "member",
				Role: "poster", MIME: "image/jpeg", Width: 16, Height: 9, Bytes: 1, FrameCount: 1,
				CreatedAt: at, UpdatedAt: at, LastUsedAt: at}
			if err := h.st.PutImage(ctx, img); err != nil {
				t.Fatal(err)
			}
			if put, err := h.st.PutHomeVideoPoster(ctx, before[0], img.Hash); err != nil || !put {
				t.Fatalf("put = %v, %v", put, err)
			}

			tc.change(h)
			h.run()

			got := wanted()
			if tc.main == ep1 {
				if len(got) != 0 {
					t.Fatalf("the poster of an unchanged main file was cleared: %+v", got)
				}
				return
			}
			main := filepath.Join(h.root, filepath.FromSlash(tc.main))
			if len(got) != 1 || got[0].Path != main {
				t.Fatalf("wanted = %+v, want a new frame of %s", got, main)
			}
		})
	}
}

// A home video's frame is its title's poster ref, which the scanner clears by writing the title:
// taking the keep back, and a changed file, both write it with no poster of its own.
func TestKeptHomeVideoPosterIsClearedByARewrite(t *testing.T) {
	const film = "Beach day (2009)/Beach day (2009).mkv"
	ctx := context.Background()
	for what, change := range map[string]func(h *harness, lib *Library, id string){
		"undo": func(_ *harness, lib *Library, id string) {
			if err := lib.Unanswer(ctx, id); err != nil {
				t.Fatal(err)
			}
		},
		"changed file": func(h *harness, _ *Library, _ string) { h.file(film, 20) },
	} {
		t.Run(what, func(t *testing.T) {
			h, lib, id := keptTitle(t, film)
			writes := &posterWrites{}
			h.im.Artwork = writes
			change(h, lib, id)
			h.run()
			if !slices.Contains(*writes, id+"=") {
				t.Fatalf("poster writes = %v, want %s cleared", *writes, id)
			}
		})
	}
}
