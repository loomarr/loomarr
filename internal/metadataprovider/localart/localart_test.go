package localart_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/localart"
	"github.com/loomarr/loomarr/internal/metadataprovider/providertest"
)

func touch(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func fileRef(kind mp.Kind, path string) mp.Ref {
	return mp.Ref{Kind: kind, ID: mp.ExternalID{Namespace: mp.NamespaceFile, Value: path}}
}

func fetch(t *testing.T, kind mp.Kind, path string) map[string]string {
	t.Helper()
	rec, err := localart.New().Fetch(context.Background(), fileRef(kind, path))
	if err != nil {
		t.Fatalf("Fetch(%s %s): %v", kind, path, err)
	}
	out := map[string]string{}
	for _, a := range rec.Artwork {
		if _, dup := out[a.Kind]; dup {
			t.Errorf("two %s images", a.Kind)
		}
		out[a.Kind] = a.Ref
	}
	return out
}

func fixture(t *testing.T) string {
	root := t.TempDir()
	touch(t, root,
		"movies/Amber Field (2004)/Amber Field (2004).mkv",
		"movies/Amber Field (2004)/Amber Field (2004)-poster.jpg",
		"movies/Amber Field (2004)/poster.png",
		"movies/Amber Field (2004)/Fanart.JPG",
		"movies/Amber Field (2004)/clearlogo.png",
		"movies/Shared/Birch Lane.mkv",
		"movies/Shared/Birch Lane-fanart.jpg",
		"movies/Shared/Cedar Point.mkv",
		"movies/Shared/poster.jpg",
		"tv/Dune Office/poster.jpg",
		"tv/Dune Office/folder.jpg",
		"tv/Dune Office/backdrop.webp",
		"tv/Dune Office/Season 01/Dune Office S01E01.mkv",
		"tv/Dune Office/Season 01/Dune Office S01E01-thumb.jpg",
		"tv/Dune Office/Season 01/Dune Office S01E02.mkv",
		"tv/Dune Office/Season 01/poster.jpg",
	)
	return root
}

func TestContract(t *testing.T) {
	root := fixture(t)
	providertest.Run(t, localart.New(), providertest.Case{
		Held: []mp.Ref{
			fileRef(mp.KindMovie, filepath.Join(root, "movies", "Amber Field (2004)", "Amber Field (2004).mkv")),
			fileRef(mp.KindSeries, filepath.Join(root, "tv", "Dune Office")),
			fileRef(mp.KindEpisode, filepath.Join(root, "tv", "Dune Office", "Season 01", "Dune Office S01E01.mkv")),
		},
		Missing: []mp.Ref{
			// A season folder's poster is not an episode's still.
			fileRef(mp.KindEpisode, filepath.Join(root, "tv", "Dune Office", "Season 01", "Dune Office S01E02.mkv")),
			fileRef(mp.KindSeries, filepath.Join(root, "tv", "Absent Show")),
		},
	})
}

// A movie's own sidecar wins over the folder's; the folder's count only for its sole video, and
// names match whatever their case.
func TestMovieArtwork(t *testing.T) {
	root := fixture(t)
	dir := filepath.Join(root, "movies", "Amber Field (2004)")
	got := fetch(t, mp.KindMovie, filepath.Join(dir, "Amber Field (2004).mkv"))
	want := map[string]string{
		mp.KindPoster:    filepath.Join(dir, "Amber Field (2004)-poster.jpg"),
		mp.KindFanart:    filepath.Join(dir, "Fanart.JPG"),
		mp.KindClearlogo: filepath.Join(dir, "clearlogo.png"),
	}
	if len(got) != len(want) {
		t.Fatalf("artwork = %v, want %v", got, want)
	}
	for kind, ref := range want {
		if got[kind] != ref {
			t.Errorf("%s = %q, want %q", kind, got[kind], ref)
		}
	}

	shared := filepath.Join(root, "movies", "Shared")
	got = fetch(t, mp.KindMovie, filepath.Join(shared, "Birch Lane.mkv"))
	if len(got) != 1 || got[mp.KindFanart] != filepath.Join(shared, "Birch Lane-fanart.jpg") {
		t.Errorf("a shared folder's poster was given to one of its movies: %v", got)
	}
	if _, err := localart.New().Fetch(context.Background(), fileRef(mp.KindMovie, filepath.Join(shared, "Cedar Point.mkv"))); !errors.Is(err, mp.ErrNotFound) {
		t.Errorf("Cedar Point = %v, want ErrNotFound", err)
	}
}

// A series' art is its folder's, the first name of each kind; an episode's is its own thumb.
func TestSeriesAndEpisodeArtwork(t *testing.T) {
	root := fixture(t)
	show := filepath.Join(root, "tv", "Dune Office")
	got := fetch(t, mp.KindSeries, show)
	if got[mp.KindPoster] != filepath.Join(show, "poster.jpg") || got[mp.KindFanart] !=filepath.Join(show, "backdrop.webp") || len(got) != 2 {
		t.Errorf("series artwork = %v", got)
	}
	got = fetch(t, mp.KindEpisode, filepath.Join(show, "Season 01", "Dune Office S01E01.mkv"))
	if len(got) != 1 || got[mp.KindThumb] !=filepath.Join(show, "Season 01", "Dune Office S01E01-thumb.jpg") {
		t.Errorf("episode artwork = %v", got)
	}
}

// A series folder's season posters are each a season's, the specials' as season 0, one per season
// by preferred extension; "season-all-poster" is not a season's, and a season banner is not a
// poster.
func TestSeasonPosters(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "Ember Coast/poster.jpg", "Ember Coast/season01-poster.png", "Ember Coast/Season01-Poster.jpg",
		"Ember Coast/season12-poster.jpg", "Ember Coast/season-specials-poster.jpg", "Ember Coast/season-all-poster.jpg",
		"Ember Coast/season02-banner.jpg")
	show := filepath.Join(root, "Ember Coast")
	rec, err := localart.New().Fetch(context.Background(), fileRef(mp.KindSeries, show))
	if err != nil {
		t.Fatal(err)
	}
	var seasons []mp.ArtworkRef
	for _, a := range rec.Artwork {
		if a.Kind == mp.KindSeasonPoster {
			seasons = append(seasons, a)
		}
	}
	want := []mp.ArtworkRef{
		{Kind: mp.KindSeasonPoster, Season: 0, Ref: filepath.Join(show, "season-specials-poster.jpg")},
		{Kind: mp.KindSeasonPoster, Season: 1, Ref: filepath.Join(show, "Season01-Poster.jpg")},
		{Kind: mp.KindSeasonPoster, Season: 12, Ref: filepath.Join(show, "season12-poster.jpg")},
	}
	if !slices.Equal(seasons, want) {
		t.Errorf("season posters = %+v, want %+v", seasons, want)
	}
}

// A symlink is never referenced: the import must not be pointed outside the share.
func TestSymlinkIsNotArtwork(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	touch(t, root, "Elm/Elm.mkv")
	touch(t, outside, "secret.jpg")
	if err := os.Symlink(filepath.Join(outside, "secret.jpg"), filepath.Join(root, "Elm", "poster.jpg")); err != nil {
		t.Fatal(err)
	}
	if _, err := localart.New().Fetch(context.Background(), fileRef(mp.KindMovie, filepath.Join(root, "Elm", "Elm.mkv"))); !errors.Is(err, mp.ErrNotFound) {
		t.Errorf("Fetch = %v, want ErrNotFound", err)
	}
}
