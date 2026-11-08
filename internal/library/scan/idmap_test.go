package scan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// Each media-server id pairs with the library item its file was written as; a show pairs through
// its episodes; anything that cannot pair says why.
func TestIDMapPairsByPathAndShowsThroughEpisodes(t *testing.T) {
	pathMap := library.ParsePathMap("/srv/media=>/mnt/media")
	at := time.Unix(1_800_000_000, 0)
	files := []store.LibraryFile{
		{Path: "/mnt/media/movies/a.mkv", ItemID: "lib-a"},
		{Path: "/mnt/media/movies/a.4k.mkv", ItemID: "lib-a"},
		{Path: "/mnt/media/movies/b.mkv", ItemID: "lib-b"},
		{Path: "/mnt/media/movies/b2.mkv", ItemID: "lib-b2"},
		{Path: "/mnt/media/tv/s/e1.mkv", ItemID: "lib-e1", SeriesID: "lib-show"},
		{Path: "/mnt/media/tv/s/e2.mkv", ItemID: "lib-e2", SeriesID: "lib-show"},
		{Path: "/mnt/media/tv/x/e1.mkv", ItemID: "lib-x1", SeriesID: "lib-x"},
		{Path: "/mnt/media/tv/y/e1.mkv", ItemID: "lib-y1", SeriesID: "lib-y"},
		{Path: "/mnt/media/movies/sample.mkv", Problem: "a sample"},
		{Path: "/mnt/media/movies/new.mkv"},
		{Path: "/mnt/media/movies/gone.mkv", ItemID: "lib-gone", MissingAt: at},
	}
	server := []library.ServerFile{
		{ItemID: "90000001", Kind: "movie", Path: "/srv/media/movies/a.mkv"},
		{ItemID: "90000001", Kind: "movie", Path: "/srv/media/movies/a.4k.mkv"},
		{ItemID: "90000002", Kind: "movie", Path: "/srv/media/movies/b.mkv"},
		{ItemID: "90000002", Kind: "movie", Path: "/srv/media/movies/b2.mkv"},
		{ItemID: "90000003", Kind: "episode", Path: "/srv/media/tv/s/e1.mkv", SeriesItemID: "90000010"},
		{ItemID: "90000004", Kind: "episode", Path: "/srv/media/tv/s/e2.mkv", SeriesItemID: "90000010"},
		{ItemID: "90000005", Kind: "episode", Path: "/srv/media/tv/x/e1.mkv", SeriesItemID: "90000011"},
		{ItemID: "90000006", Kind: "episode", Path: "/srv/media/tv/y/e1.mkv", SeriesItemID: "90000011"},
		{ItemID: "90000007", Kind: "movie", Path: "/elsewhere/c.mkv"},
		{ItemID: "90000008", Kind: "movie", Path: "/srv/media/movies/sample.mkv"},
		{ItemID: "90000009", Kind: "movie", Path: "/srv/media/movies/new.mkv"},
		{ItemID: "90000012", Kind: "movie", Path: "/srv/media/movies/gone.mkv"},
		{ItemID: "90000013", Kind: "movie", Path: "/srv/media/movies/a.mkv"},
	}
	got := map[string]store.LibraryIDMapping{}
	for _, m := range IDMap(server, files, pathMap) {
		got[m.MediaID] = m
	}
	want := map[string]store.LibraryIDMapping{
		"90000001": {MediaID: "90000001", Kind: "movie", ItemID: "lib-a"},
		"90000002": {MediaID: "90000002", Kind: "movie", Reason: "its versions are 2 different titles"},
		"90000003": {MediaID: "90000003", Kind: "episode", ItemID: "lib-e1"},
		"90000004": {MediaID: "90000004", Kind: "episode", ItemID: "lib-e2"},
		"90000005": {MediaID: "90000005", Kind: "episode", ItemID: "lib-x1"},
		"90000006": {MediaID: "90000006", Kind: "episode", ItemID: "lib-y1"},
		"90000007": {MediaID: "90000007", Kind: "movie", Reason: "the path map doesn't cover its file"},
		"90000008": {MediaID: "90000008", Kind: "movie", Reason: "the scanner skips its file: a sample"},
		"90000009": {MediaID: "90000009", Kind: "movie", Reason: "its file isn't a title yet"},
		"90000010": {MediaID: "90000010", Kind: "series", ItemID: "lib-show"},
		"90000011": {MediaID: "90000011", Kind: "series", Reason: "its episodes are in 2 different shows"},
		"90000012": {MediaID: "90000012", Kind: "movie", Reason: "the scanner hasn't found its file"},
		"90000013": {MediaID: "90000013", Kind: "movie", ItemID: "lib-a"},
	}
	if len(got) != len(want) {
		t.Errorf("%d ids mapped, want %d: %+v", len(got), len(want), got)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s = %+v, want %+v", id, got[id], w)
		}
	}
}

// discoveredRoots answers the roots and path map discovery would have recorded.
type discoveredRoots struct {
	IDMapStore
	roots store.LibraryRoots
}

func (d discoveredRoots) LibraryRoots(context.Context) (store.LibraryRoots, error) { return d.roots, nil }

// A pass replaces the stored map; a pass that cannot list the media server leaves it alone.
func TestIDMapperReplacesTheMapAndKeepsItWhenTheServerFails(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	ctx := context.Background()
	at := time.Unix(1_800_000_000, 0)
	roots := discoveredRoots{IDMapStore: st, roots: store.LibraryRoots{
		Roots: []store.LibraryRoot{{Path: "/mnt/media", InUse: true}},
		Paths: []store.LibraryRootPath{{Authority: string(library.Emby), AuthorityPath: "/srv/media", RootPath: "/mnt/media"}},
	}}
	if err := st.SaveLibraryFiles(ctx, []store.LibraryFile{{Path: "/mnt/media/a.mkv", Root: "/mnt/media", Size: 1,
		ModifiedNs: 1, Identity: "d", Kind: "movie", Header: store.LibraryHeaderRead, SeenAt: at, ItemID: "lib-a"}}); err != nil {
		t.Fatal(err)
	}
	var fail error
	m := &IDMapper{Store: roots, Now: func() time.Time { return at }, ServerFiles: func(context.Context) ([]library.ServerFile, error) {
		return []library.ServerFile{{ItemID: "90000001", Kind: "movie", Path: "/srv/media/a.mkv"}}, fail
	}}
	if err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	want := store.LibraryIDMapping{MediaID: "90000001", Kind: "movie", ItemID: "lib-a"}
	if got, err := st.LibraryIDMap(ctx); err != nil || len(got) != 1 || got[0] != want {
		t.Fatalf("map = %+v, %v; want %+v", got, err, want)
	}
	fail = errors.New("media server unreachable")
	if err := m.Tick(ctx); err == nil {
		t.Fatal("a pass that cannot list the media server must fail")
	}
	if got, _ := st.LibraryIDMap(ctx); len(got) != 1 || got[0] != want {
		t.Fatalf("after a failed pass, map = %+v; want the last one kept", got)
	}
	empty := &IDMapper{Store: roots, ServerFiles: func(context.Context) ([]library.ServerFile, error) { return nil, nil }}
	none := &IDMapper{Store: roots, ServerFiles: func(context.Context) ([]library.ServerFile, error) {
		return nil, ErrNoServerCatalog
	}}
	for name, pass := range map[string]*IDMapper{"an empty catalog": empty, "no media server": none} {
		if err := pass.Tick(ctx); err != nil {
			t.Errorf("%s: %v; want a quiet pass", name, err)
		}
		if got, _ := st.LibraryIDMap(ctx); len(got) != 1 || got[0] != want {
			t.Errorf("after %s, map = %+v; want the last one kept", name, got)
		}
	}
}
