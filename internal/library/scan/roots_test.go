package scan

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// fakeDisk answers readability from a fixed set of folders.
type fakeDisk struct {
	readable map[string]bool
	mounts   []string
}

func (d fakeDisk) Readable(_ context.Context, path string) bool { return d.readable[path] }
func (d fakeDisk) Mounts() []string                             { return d.mounts }

func folders(list ...string) func(context.Context) ([]string, error) {
	return func(context.Context) ([]string, error) { return list, nil }
}

func openStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), testkit.MigratedSQLiteDSN(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	st.ProjectLibraryWith(ProjectLibraryItem)
	return st
}

func newRoots(t *testing.T, disk fakeDisk, pathMap string, sources ...Source) *Roots {
	t.Helper()
	return &Roots{
		Store:   openStore(t),
		Sources: func() []Source { return sources },
		PathMap: func() library.PathMap { return library.ParsePathMap(pathMap) },
		Disk:    disk,
		Now:     func() time.Time { return time.UnixMilli(1_800_000_000_000) },
	}
}

func rootFor(all librarystore.LibraryRoots, authority, folder string) string {
	for _, p := range all.Paths {
		if p.Authority == authority && p.AuthorityPath == folder {
			return p.RootPath
		}
	}
	return "<absent>"
}

// Each authority's folder maps to a local folder: as named, through the path map, or by the one
// mount holding its trailing names. Ambiguous and missing folders stay unmapped.
func TestDiscoverDerivesLocalFolders(t *testing.T) {
	disk := fakeDisk{
		readable: map[string]bool{
			"/media/tv": true, "/mnt/nas/movies": true, "/srv/anime": true, "media/tv": true,
			"/mnt/a/kids": true, "/mnt/b/kids": true, "/mnt/music": true,
		},
		mounts: []string{"/mnt/nas", "/mnt/a", "/mnt/b", "/srv/anime"},
	}
	r := newRoots(t, disk, "/data=>/mnt/nas",
		Source{Authority: "emby", Folders: folders("/media/tv", "/data/movies/", "/data/kids", "/data/gone", "media/tv")},
		Source{Authority: "sonarr", Folders: folders("/tv-anime/anime")},
		Source{Authority: "radarr", Folders: func(context.Context) ([]string, error) { return nil, ErrSkipped }},
	)
	all, results, err := r.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ authority, folder, want string }{
		{"emby", "/media/tv", "/media/tv"},
		{"emby", "/data/movies", "/mnt/nas/movies"},
		{"emby", "/data/kids", ""}, // the path map misses, and two mounts hold a "kids"
		{"emby", "/data/gone", ""},
		{"emby", "media/tv", ""}, // relative: readable from here, but never a root

		{"sonarr", "/tv-anime/anime", "/srv/anime"},
	} {
		if got := rootFor(all, c.authority, c.folder); got != c.want {
			t.Errorf("%s %s → %q, want %q", c.authority, c.folder, got, c.want)
		}
	}
	if len(all.Roots) != 3 {
		t.Fatalf("roots %+v", all.Roots)
	}
	for _, root := range all.Roots {
		if root.InUse || !root.Readable {
			t.Errorf("a discovered root starts unconfirmed and checked: %+v", root)
		}
	}
	states := map[string]string{}
	for _, res := range results {
		states[res.Authority] = res.State
	}
	if states["emby"] != "ok" || states["sonarr"] != "ok" || states["radarr"] != "skipped" {
		t.Fatalf("results %+v", results)
	}
}

// A source that fails keeps last time's folders; a root that goes offline is recorded unreadable.
func TestDiscoverKeepsFailedSourceAndMarksOffline(t *testing.T) {
	disk := fakeDisk{readable: map[string]bool{"/media/tv": true}}
	fail := false
	sonarr := Source{Authority: "sonarr", Folders: func(context.Context) ([]string, error) {
		if fail {
			return nil, errors.New("connection refused")
		}
		return []string{"/media/tv"}, nil
	}}
	r := newRoots(t, disk, "", sonarr)
	ctx := context.Background()
	if _, _, err := r.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.SetInUse(ctx, "/media/tv", true); err != nil {
		t.Fatal(err)
	}

	fail = true
	disk.readable["/media/tv"] = false
	all, results, err := r.Discover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rootFor(all, "sonarr", "/media/tv") != "/media/tv" {
		t.Fatalf("a failed source's folder was forgotten: %+v", all.Paths)
	}
	if len(all.Roots) != 1 || !all.Roots[0].InUse || all.Roots[0].Readable {
		t.Fatalf("offline root %+v", all.Roots)
	}
	if results[0].State != "failed" || !strings.Contains(results[0].Error, "refused") {
		t.Fatalf("results %+v", results)
	}
}

// A hand-typed folder must be absolute and readable; a hand-set mapping survives discovery.
func TestHandEdits(t *testing.T) {
	disk := fakeDisk{readable: map[string]bool{"/mnt/extra": true, "/mnt/shows": true, "/media/tv": true}}
	r := newRoots(t, disk, "", Source{Authority: "sonarr", Folders: folders("/media/tv")})
	ctx := context.Background()

	if err := r.Add(ctx, "relative/path"); !errors.Is(err, ErrNotAbsolute) {
		t.Fatalf("relative: %v", err)
	}
	if err := r.Add(ctx, "/mnt/missing"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("unreadable: %v", err)
	}
	if err := r.Add(ctx, " /mnt/extra/ "); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.SetMapping(ctx, "sonarr", "/media/tv", "/mnt/shows"); err != nil {
		t.Fatal(err)
	}
	all, _, err := r.Discover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rootFor(all, "sonarr", "/media/tv") != "/mnt/shows" {
		t.Fatalf("discovery rewrote a hand mapping: %+v", all.Paths)
	}
	var extra, shows bool
	for _, root := range all.Roots {
		extra = extra || (root.Path == "/mnt/extra" && root.InUse && root.ByHand)
		shows = shows || (root.Path == "/mnt/shows" && root.Readable)
	}
	if !extra || !shows {
		t.Fatalf("roots %+v", all.Roots)
	}
	if err := r.SetMapping(ctx, "sonarr", "/nowhere", "/x"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown mapping: %v", err)
	}
}

func TestParseMountInfo(t *testing.T) {
	const table = `22 1 0:21 / / rw - ext4 /dev/sda1 rw
23 22 0:5 / /proc rw - proc proc rw
40 22 0:45 / /mnt/nas rw - cifs //nas/media rw
41 22 0:46 / /mnt/my\040films rw - cifs //nas/films rw
42 22 0:47 / /run/user/1000 rw - tmpfs tmpfs rw
43 22 0:45 / /mnt/nas rw - cifs //nas/media rw
`
	got := parseMountInfo(strings.NewReader(table))
	if strings.Join(got, "|") != "/mnt/nas|/mnt/my films" {
		t.Fatalf("mounts %q", got)
	}
}

func TestOSDiskReadable(t *testing.T) {
	dir := t.TempDir()
	var d OSDisk
	if !d.Readable(context.Background(), dir) {
		t.Fatal("an empty directory is readable")
	}
	if d.Readable(context.Background(), filepath.Join(dir, "missing")) {
		t.Fatal("a missing directory is not readable")
	}
}
