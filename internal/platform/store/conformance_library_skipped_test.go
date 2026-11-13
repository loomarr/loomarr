package store

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// The files a scan skips are listed by why, each reason with its count and its first few paths in
// path order; a file no longer there, or one the scan keeps, is none of them.
func testLibrarySkippedFiles(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.UnixMilli(1_800_000_000_000).UTC()
	if got, err := s.LibrarySkippedFiles(ctx, 2); err != nil || len(got) != 0 {
		t.Fatalf("empty = %+v, %v", got, err)
	}
	file := func(path, problem string, missing bool) LibraryFile {
		f := LibraryFile{Path: path, Root: "/media/a", Size: 1, ModifiedNs: 1, Problem: problem, SeenAt: at}
		if missing {
			f.MissingAt = at
		}
		return f
	}
	if err := s.SaveLibraryFiles(ctx, []LibraryFile{
		file("/media/a/kept.mkv", "", false),
		file("/media/a/c-link.mkv", "broken_link", false),
		file("/media/a/a-link.mkv", "broken_link", false),
		file("/media/a/b-link.mkv", "broken_link", false),
		file("/media/a/gone-link.mkv", "broken_link", true),
		file("/media/a/out.mkv", "link_outside", false),
		file("/media/a/disc.iso", "disc_image", false),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.LibrarySkippedFiles(ctx, 2)
	want := []LibrarySkipped{
		{Problem: "broken_link", Total: 3, Paths: []string{"/media/a/a-link.mkv", "/media/a/b-link.mkv"}},
		{Problem: "disc_image", Total: 1, Paths: []string{"/media/a/disc.iso"}},
		{Problem: "link_outside", Total: 1, Paths: []string{"/media/a/out.mkv"}},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("skipped = %+v, %v; want %+v", got, err, want)
	}
}
