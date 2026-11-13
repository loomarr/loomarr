package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func rootByPath(t *testing.T, all LibraryRoots, path string) (LibraryRoot, bool) {
	t.Helper()
	for _, r := range all.Roots {
		if r.Path == path {
			return r, true
		}
	}
	return LibraryRoot{}, false
}

func mappingFor(all LibraryRoots, authority, authorityPath string) (LibraryRootPath, bool) {
	for _, p := range all.Paths {
		if p.Authority == authority && p.AuthorityPath == authorityPath {
			return p, true
		}
	}
	return LibraryRootPath{}, false
}

func mustLibraryRoots(t *testing.T, s Store) LibraryRoots {
	t.Helper()
	all, err := s.LibraryRoots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return all
}

// Discovery adds roots unconfirmed, keeps the maintainer's confirmation, forgets folders an
// answering authority stopped reporting, and never forgets one whose authority failed.
func testLibraryRootDiscovery(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	t0 := time.UnixMilli(1_800_000_000_000).UTC()

	first := LibraryRootDiscovery{
		Found: []LibraryRootPath{
			{Authority: "emby", AuthorityPath: "/data/tv", RootPath: "/media/tv"},
			{Authority: "sonarr", AuthorityPath: "/tv", RootPath: "/media/tv"},
			{Authority: "radarr", AuthorityPath: "/movies", RootPath: "/media/movies"},
			{Authority: "emby", AuthorityPath: "/data/home-videos"},
		},
		Complete: []string{"emby", "sonarr", "radarr"},
		Checks:   []LibraryRootCheck{{Path: "/media/tv", Readable: true}, {Path: "/media/movies", Readable: false}},
	}
	if err := s.RecordLibraryRootDiscovery(ctx, first, t0); err != nil {
		t.Fatal(err)
	}
	all := mustLibraryRoots(t, s)
	if len(all.Roots) != 2 || len(all.Paths) != 4 {
		t.Fatalf("roots %+v paths %+v", all.Roots, all.Paths)
	}
	tv, _ := rootByPath(t, all, "/media/tv")
	if tv.InUse || !tv.Readable || !tv.CheckedAt.Equal(t0) || tv.ByHand {
		t.Fatalf("tv root %+v", tv)
	}
	movies, _ := rootByPath(t, all, "/media/movies")
	if movies.Readable || !movies.CheckedAt.Equal(t0) {
		t.Fatalf("movies root %+v", movies)
	}
	if home, ok := mappingFor(all, "emby", "/data/home-videos"); !ok || home.RootPath != "" {
		t.Fatalf("an unmapped folder stays in the map with no root: %+v", home)
	}

	if err := s.SetLibraryRootInUse(ctx, "/media/tv", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLibraryRootInUse(ctx, "/nowhere", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown root: %v", err)
	}

	// Radarr fails this time; Emby and Sonarr stop reporting their TV folder.
	t1 := t0.Add(time.Hour)
	second := LibraryRootDiscovery{
		Found:    []LibraryRootPath{{Authority: "emby", AuthorityPath: "/data/home-videos"}},
		Complete: []string{"emby", "sonarr"},
	}
	if err := s.RecordLibraryRootDiscovery(ctx, second, t1); err != nil {
		t.Fatal(err)
	}
	all = mustLibraryRoots(t, s)
	if _, ok := mappingFor(all, "sonarr", "/tv"); ok {
		t.Fatal("a folder an answering authority stopped reporting is kept")
	}
	if _, ok := mappingFor(all, "radarr", "/movies"); !ok {
		t.Fatal("a failed authority's folder was forgotten")
	}
	if tv, ok := rootByPath(t, all, "/media/tv"); !ok || !tv.InUse {
		t.Fatalf("a confirmed root no authority names is kept: %+v", all.Roots)
	}
	if _, ok := rootByPath(t, all, "/media/movies"); !ok {
		t.Fatal("a root a kept mapping names was dropped")
	}

	// Radarr answers again without its folder: the unconfirmed root goes with it.
	if err := s.RecordLibraryRootDiscovery(ctx, LibraryRootDiscovery{Complete: []string{"radarr"}}, t1.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, ok := rootByPath(t, mustLibraryRoots(t, s), "/media/movies"); ok {
		t.Fatal("an unconfirmed root nothing names was kept")
	}
}

// Hand-typed roots are confirmed; hand-set mappings survive discovery; releasing a hand-typed root
// nothing names removes it.
// A pass's readability check flips a root only against an earlier check, so one notification goes
// out per change rather than per pass.
func testLibraryRootCheck(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	t0 := time.UnixMilli(1_800_000_000_000).UTC()
	found := LibraryRootDiscovery{
		Found: []LibraryRootPath{{Authority: "radarr", AuthorityPath: "/movies", RootPath: "/media/movies"}}, Complete: []string{"radarr"},
	}
	if err := s.RecordLibraryRootDiscovery(ctx, found, t0); err != nil {
		t.Fatal(err)
	}
	for i, step := range []struct {
		readable, empty bool
		flipped         LibraryRootFlip
	}{
		{readable: true}, // the first check sets a baseline
		{readable: true}, // no change
		{readable: false, flipped: LibraryRootFlip{Readable: true}}, // went offline
		{readable: false}, // still offline
		{readable: true, flipped: LibraryRootFlip{Readable: true}},               // back
		{readable: true, empty: true, flipped: LibraryRootFlip{Empty: true}},     // the share came unmounted
		{readable: true, empty: true},                                            // still empty
		{readable: true, flipped: LibraryRootFlip{Empty: true}},                  // mounted again
		{readable: false, empty: true, flipped: LibraryRootFlip{Readable: true}}, // never empty while unreadable
	} {
		at := t0.Add(time.Duration(i+1) * time.Hour)
		check := LibraryRootCheck{Path: "/media/movies", Readable: step.readable, Empty: step.empty}
		flipped, err := s.CheckLibraryRoot(ctx, check, at)
		if err != nil || flipped != step.flipped {
			t.Fatalf("check %d (%+v) = %+v, %v; want %+v", i, check, flipped, err, step.flipped)
		}
		root, _ := rootByPath(t, mustLibraryRoots(t, s), "/media/movies")
		if root.Readable != step.readable || root.Empty != (step.readable && step.empty) || !root.CheckedAt.Equal(at) {
			t.Fatalf("check %d left %+v", i, root)
		}
	}
	if _, err := s.CheckLibraryRoot(ctx, LibraryRootCheck{Path: "/nowhere", Readable: true}, t0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("check of an unknown root = %v, want ErrNotFound", err)
	}
}

func testLibraryRootByHand(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	t0 := time.UnixMilli(1_800_000_000_000).UTC()

	if err := s.AddLibraryRoot(ctx, LibraryRootCheck{Path: "/mnt/extra", Readable: true}, t0); err != nil {
		t.Fatal(err)
	}
	extra, ok := rootByPath(t, mustLibraryRoots(t, s), "/mnt/extra")
	if !ok || !extra.InUse || !extra.ByHand || !extra.Readable {
		t.Fatalf("hand-typed root %+v", extra)
	}

	found := LibraryRootDiscovery{
		Found:    []LibraryRootPath{{Authority: "sonarr", AuthorityPath: "/tv", RootPath: "/media/tv"}},
		Complete: []string{"sonarr"},
	}
	if err := s.RecordLibraryRootDiscovery(ctx, found, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLibraryRootPath(ctx, "sonarr", "/tv", "/mnt/shows", t0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLibraryRootPath(ctx, "sonarr", "/elsewhere", "/x", t0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown mapping: %v", err)
	}
	if err := s.RecordLibraryRootDiscovery(ctx, found, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	all := mustLibraryRoots(t, s)
	if m, _ := mappingFor(all, "sonarr", "/tv"); m.RootPath != "/mnt/shows" || !m.ByHand {
		t.Fatalf("discovery rewrote a hand-set mapping: %+v", m)
	}
	if _, ok := rootByPath(t, all, "/mnt/shows"); !ok {
		t.Fatal("a hand-mapped root was not added")
	}
	if _, ok := rootByPath(t, all, "/media/tv"); ok {
		t.Fatal("the root the hand mapping replaced was kept")
	}

	// Handing the row back lets discovery derive it again.
	if err := s.SetLibraryRootPath(ctx, "sonarr", "/tv", "", t0); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordLibraryRootDiscovery(ctx, found, t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if m, _ := mappingFor(mustLibraryRoots(t, s), "sonarr", "/tv"); m.RootPath != "/media/tv" || m.ByHand {
		t.Fatalf("handed-back mapping %+v", m)
	}

	if err := s.SetLibraryRootInUse(ctx, "/mnt/extra", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := rootByPath(t, mustLibraryRoots(t, s), "/mnt/extra"); ok {
		t.Fatal("releasing a hand-typed root kept it")
	}
}
