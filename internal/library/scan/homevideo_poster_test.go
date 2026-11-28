package scan

import (
	"context"
	"slices"
	"testing"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

type posterWrites []string

func (w *posterWrites) Art(_ context.Context, itemID, kind, ref string) error {
	if kind == mp.KindPoster {
		*w = append(*w, itemID+"="+ref)
	}
	return nil
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
