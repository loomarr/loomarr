package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The file table keeps whole rows per root, counts present files, and records a root's last walk.
func testLibraryFiles(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.UnixMilli(1_800_000_000_000).UTC()
	if err := s.AddLibraryRoot(ctx, LibraryRootCheck{Path: "/media/a", Readable: true}, at); err != nil {
		t.Fatal(err)
	}
	files := []LibraryFile{
		{Path: "/media/a/one.mkv", Root: "/media/a", Size: 1 << 40, ModifiedNs: 1_700_000_000_123_456_789, Identity: "i1", SeenAt: at},
		{Path: "/media/a/two.mkv", Root: "/media/a", Size: 2, ModifiedNs: 2, Kind: "episode", Header: LibraryHeaderRead, Held: true, SeenAt: at},
		{Path: "/media/a/disc.iso", Root: "/media/a", Size: 3, ModifiedNs: 3, Header: LibraryHeaderFailed, Problem: "disc_image", SeenAt: at},
		{Path: "/media/a/gone.mkv", Root: "/media/a", Size: 4, ModifiedNs: 4, SeenAt: at, MissingAt: at},
		{Path: "/media/b/three.mkv", Root: "/media/b", Size: 5, ModifiedNs: 5, SeenAt: at},
	}
	if err := s.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	got, err := s.LibraryFiles(ctx, "/media/a")
	if err != nil || len(got) != 4 {
		t.Fatalf("files = %+v, %v", got, err)
	}
	one := got[2]
	if one.Path != "/media/a/one.mkv" || one.Size != 1<<40 || one.ModifiedNs != 1_700_000_000_123_456_789 ||
		one.Header != LibraryHeaderPending || one.Identity != "i1" || !one.SeenAt.Equal(at) || !one.MissingAt.IsZero() {
		t.Errorf("one = %+v", one)
	}
	if !got[3].Held || got[3].Kind != "episode" || got[0].Problem != "disc_image" || !got[1].MissingAt.Equal(at) {
		t.Errorf("rows = %+v", got)
	}

	counts, err := s.LibraryFileCounts(ctx)
	if err != nil || counts.Files != 4 || counts.HeadersPending != 2 || counts.Problems != 1 ||
		len(counts.Held) != 1 || counts.Held[0] != (LibraryHeldCount{Root: "/media/a", Kind: "episode", Count: 1}) {
		t.Errorf("counts = %+v, %v", counts, err)
	}

	one.Header, one.Identity = LibraryHeaderRead, "i2"
	if err := s.SaveLibraryFiles(ctx, []LibraryFile{one}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LibraryFiles(ctx, "/media/a"); got[2].Header != LibraryHeaderRead || got[2].Identity != "i2" {
		t.Errorf("rewritten = %+v", got[2])
	}

	roots, err := s.LibraryFileRoots(ctx)
	if err != nil || len(roots) != 2 || roots[0] != "/media/a" || roots[1] != "/media/b" {
		t.Errorf("file roots = %v, %v", roots, err)
	}
	if err := s.DeleteLibraryFiles(ctx, "/media/b"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LibraryFiles(ctx, "/media/b"); len(got) != 0 {
		t.Errorf("deleted root still has %d files", len(got))
	}

	if err := s.SetLibraryRootScanned(ctx, "/media/a", at); err != nil {
		t.Fatal(err)
	}
	if r, ok := rootByPath(t, mustLibraryRoots(t, s), "/media/a"); !ok || !r.ScannedAt.Equal(at) {
		t.Errorf("scanned root = %+v", r)
	}
	if err := s.SetLibraryRootScanned(ctx, "/media/none", at); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown root = %v", err)
	}
}
