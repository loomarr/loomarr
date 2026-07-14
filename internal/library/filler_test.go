package library_test

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/testkit"
)

// ListFillerClips reads the media-server filler library and derives duration from
// the server's RunTimeTicks (§10 — the core never probes media), inferring kind +
// era from the filename convention.
func TestListFillerClips_DurationFromServer(t *testing.T) {
	ms := testkit.NewMediaServer(t)
	c := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")

	clips, err := c.ListFillerClips(context.Background(), "filler-lib-id")
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 4 {
		t.Fatalf("want 4 clips from the filler fixture, got %d", len(clips))
	}

	byID := map[string]library.FillerClip{}
	for _, cl := range clips {
		byID[cl.LibraryItemID] = cl
	}

	// Duration comes from RunTimeTicks (300000000 ticks / 10000 = 30000ms).
	if byID["clip-100"].DurationMs != 30000 {
		t.Errorf("clip-100 duration = %d, want 30000 (from server ticks)", byID["clip-100"].DurationMs)
	}
	// Kind inferred from the filename/folder convention.
	if byID["clip-100"].Kind != filler.Commercial {
		t.Errorf("clip-100 kind = %s, want commercial", byID["clip-100"].Kind)
	}
	if byID["clip-200"].Kind != filler.Bumper {
		t.Errorf("clip-200 (bumper) kind = %s, want bumper", byID["clip-200"].Kind)
	}
	if byID["clip-300"].Kind != filler.StationID {
		t.Errorf("clip-300 (station id) kind = %s, want station_id", byID["clip-300"].Kind)
	}
	// Era parsed from the filename ("... 1992").
	if byID["clip-100"].Era != 1992 {
		t.Errorf("clip-100 era = %d, want 1992 (from filename)", byID["clip-100"].Era)
	}
	if byID["clip-101"].Era != 1994 {
		t.Errorf("clip-101 era = %d, want 1994", byID["clip-101"].Era)
	}
}
