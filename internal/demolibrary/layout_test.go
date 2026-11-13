package demolibrary

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// The demo's media folders are named as a household's are, with each title's provider id in its
// folder and file names: the shapes Loomarr's scan grounds with no arr and no search (#2151).
func TestLayoutNamesTitlesTheWayTheScanGroundsThem(t *testing.T) {
	l := Layout{Dir: "/demo"}
	film := Title{N: 3, Kind: Movie, Name: "Return: of the Kite", Year: 1986}
	show := Title{N: 1, Kind: Series, Name: "Captain Pickle", Year: 1987}
	filmID, showID := strconv.Itoa(ProviderIDBase+3), strconv.Itoa(ProviderIDBase+1)

	if got, want := l.FilmFile(film),
		"/demo/library/movies/Return- of the Kite (1986) {tmdb-"+filmID+"}/Return- of the Kite (1986) {tmdb-"+filmID+"}.mp4"; got != want {
		t.Fatalf("film file = %q, want %q", got, want)
	}
	if got, want := l.EpisodeFile(show, Episode{Season: 1, Number: 4}),
		"/demo/library/shows/Captain Pickle (1987) {tmdb-"+showID+"} {tvdb-"+showID+"}/Season 01/Captain Pickle - S01E04.mp4"; got != want {
		t.Fatalf("episode file = %q, want %q", got, want)
	}
}

// Every film and episode gets its own hard link to its encoded file, and a link left pointing at
// a file the generator has since replaced is made again.
func TestLinkLibraryHardLinksEachItemToItsEncodedFile(t *testing.T) {
	l := Layout{Dir: t.TempDir()}
	film := Title{N: 1, Kind: Movie, Name: "Kite", Year: 1986, Format: "film-1080p-h264"}
	show := Title{N: 2, Kind: Series, Name: "Pickle", Year: 1987, Format: "episode-1080p-h264", Episodes: 2}
	lib := &Library{Titles: []Title{film, show}, episodes: map[string][]Episode{show.ID(): EpisodesOf(show)}}
	for _, f := range []string{film.Format, show.Format} {
		if err := os.MkdirAll(filepath.Dir(l.Video(f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(l.Video(f), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.linkLibrary(lib); err != nil {
		t.Fatal(err)
	}
	sameAs := func(path, target string) bool {
		t.Helper()
		a, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if a.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s is a symlink; the scan never follows one", path)
		}
		b, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		return os.SameFile(a, b)
	}
	if !sameAs(l.FilmFile(film), l.Video(film.Format)) {
		t.Fatal("the film's file isn't its format file")
	}
	// An invented film has English subtitles beside it, named so the scan reads their language.
	subs, err := os.ReadFile(filepath.Join(filepath.Dir(l.FilmFile(film)),
		"Kite (1986) {tmdb-"+strconv.Itoa(ProviderIDBase+1)+"}.en.srt"))
	if err != nil {
		t.Fatalf("the film has no subtitles beside it: %v", err)
	}
	if want := "1\n00:00:10,000 --> 00:00:14,000\nDemo subtitles · line 1\n\n2\n00:00:20,000"; len(subs) < len(want) || string(subs[:len(want)]) != want {
		t.Fatalf("subtitles begin %q, want %q", subs[:min(len(subs), len(want))], want)
	}
	for _, e := range EpisodesOf(show) {
		if !sameAs(l.EpisodeFile(show, e), l.Video(show.Format)) {
			t.Fatalf("episode %d's file isn't its format file", e.Number)
		}
	}

	// The generator replaces a file by renaming a new one over it, which leaves the link on the old.
	replaced := l.Video(film.Format) + ".new"
	if err := os.WriteFile(replaced, []byte("again"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replaced, l.Video(film.Format)); err != nil {
		t.Fatal(err)
	}
	if err := l.linkLibrary(lib); err != nil {
		t.Fatal(err)
	}
	if !sameAs(l.FilmFile(film), l.Video(film.Format)) {
		t.Fatal("after the format file was replaced, the film's link still names the old one")
	}
}
