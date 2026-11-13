package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// fakeDisk answers from a fixed set of folders: readable ones, and of those the empty ones.
type fakeDisk struct {
	readable map[string]bool
	empty    map[string]bool
	mounts   []string
}

func (d fakeDisk) Look(_ context.Context, path string) Folder {
	return Folder{Readable: d.readable[path], Empty: d.empty[path]}
}
func (d fakeDisk) Mounts() []string { return d.mounts }

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
		PathMap: func() mediaserver.PathMap { return mediaserver.ParsePathMap(pathMap) },
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

// Each authority's folder maps to a local folder: through the path map, as named, or by the one
// mount holding its trailing names. Ambiguous and missing folders stay unmapped.
func TestDiscoverDerivesLocalFolders(t *testing.T) {
	disk := fakeDisk{
		readable: map[string]bool{
			"/media/tv": true, "/mnt/nas/movies": true, "/srv/anime": true, "media/tv": true,
			"/mnt/a/kids": true, "/mnt/b/kids": true, "/mnt/music": true,
			// The media server's own name for shows also exists here, empty: Loomarr's data folder
			// is mounted at /data too. The path map says where the shows really are.
			"/data/shows": true, "/mnt/nas/shows": true,
		},
		mounts: []string{"/mnt/nas", "/mnt/a", "/mnt/b", "/srv/anime"},
	}
	r := newRoots(t, disk, "/data=>/mnt/nas",
		Source{Authority: "emby", Folders: folders("/media/tv", "/data/movies/", "/data/kids", "/data/gone", "media/tv", "/data/shows")},
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
		{"emby", "media/tv", ""},                  // relative: readable from here, but never a root
		{"emby", "/data/shows", "/mnt/nas/shows"}, // the path map wins over a same-named folder

		{"sonarr", "/tv-anime/anime", "/srv/anime"},
	} {
		if got := rootFor(all, c.authority, c.folder); got != c.want {
			t.Errorf("%s %s → %q, want %q", c.authority, c.folder, got, c.want)
		}
	}
	if len(all.Roots) != 4 {
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

// Using a folder, typing one in, or mapping one by hand asks for a pass at once; releasing a folder
// or clearing a mapping does not.
func TestUsingAFolderAsksForAPass(t *testing.T) {
	disk := fakeDisk{readable: map[string]bool{"/media/tv": true, "/mnt/extra": true, "/mnt/shows": true}}
	h := newHarness(t)
	roots := newRoots(t, disk, "", Source{Authority: "sonarr", Folders: folders("/media/tv")})
	lib := &Library{Roots: roots, Scanner: NewScanner(h.im, time.Hour)}
	ctx := context.Background()
	if _, _, err := roots.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	asked := func() bool {
		select {
		case <-lib.Scanner.trigger:
			return true
		default:
			return false
		}
	}

	if err := lib.SetInUse(ctx, "/media/tv", true); err != nil {
		t.Fatal(err)
	}
	if !asked() {
		t.Fatal("Use this folder did not ask for a pass")
	}
	if err := lib.SetInUse(ctx, "/media/tv", false); err != nil {
		t.Fatal(err)
	}
	if asked() {
		t.Fatal("releasing a folder asked for a pass")
	}
	if err := lib.Add(ctx, "/mnt/extra"); err != nil {
		t.Fatal(err)
	}
	if !asked() {
		t.Fatal("a folder typed in did not ask for a pass")
	}
	if err := lib.SetInUse(ctx, "/nowhere", true); err == nil || asked() {
		t.Fatalf("an unknown folder: err %v, asked %v", err, asked())
	}
	if err := lib.SetMapping(ctx, "sonarr", "/media/tv", "/mnt/shows"); err != nil {
		t.Fatal(err)
	}
	if !asked() {
		t.Fatal("a mapping set by hand did not ask for a pass")
	}
	if err := lib.SetMapping(ctx, "sonarr", "/media/tv", ""); err != nil {
		t.Fatal(err)
	}
	if asked() {
		t.Fatal("clearing a mapping asked for a pass")
	}
	if err := lib.SetMapping(ctx, "sonarr", "/nowhere", "/mnt/shows"); err == nil || asked() {
		t.Fatalf("an unknown mapping: err %v, asked %v", err, asked())
	}
}

// An empty folder is usually a share that isn't mounted: it is added, but the library isn't
// called healthy while it stays empty, and is again once something is in it.
func TestAnEmptyFolderIsAddedButNotHealthy(t *testing.T) {
	disk := fakeDisk{
		readable: map[string]bool{"/mnt/movies": true, "/mnt/tv": true},
		empty:    map[string]bool{"/mnt/tv": true},
	}
	r := newRoots(t, disk, "")
	ctx := context.Background()
	for _, path := range []string{"/mnt/movies", "/mnt/tv"} {
		if err := r.Add(ctx, path); err != nil {
			t.Fatalf("add %s: %v", path, err)
		}
	}
	all, err := r.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := StatusOf(all); got.Health != HealthFolderEmpty || got.InUse != 2 || got.Empty != 1 {
		t.Fatalf("with tv empty: %+v, want folder_empty with 2 in use, 1 empty", got)
	}

	delete(disk.empty, "/mnt/tv") // the share is mounted again
	if all, _, err = r.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	if got := StatusOf(all); got.Health != HealthOK || got.Empty != 0 {
		t.Fatalf("with tv mounted: %+v, want ok and nothing empty", got)
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

func TestOSDiskLook(t *testing.T) {
	dir := t.TempDir()
	var d OSDisk
	if got := d.Look(context.Background(), dir); got != (Folder{Readable: true, Empty: true}) {
		t.Fatalf("an empty directory: %+v, want readable and empty", got)
	}
	if err := os.Mkdir(filepath.Join(dir, "Films"), 0o750); err != nil {
		t.Fatal(err)
	}
	if got := d.Look(context.Background(), dir); got != (Folder{Readable: true}) {
		t.Fatalf("a directory with a folder in it: %+v, want readable and not empty", got)
	}
	if got := d.Look(context.Background(), filepath.Join(dir, "missing")); got.Readable || got.Empty {
		t.Fatalf("a missing directory: %+v, want neither readable nor empty", got)
	}
}
