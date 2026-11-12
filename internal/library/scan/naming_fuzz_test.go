package scan

import (
	"path"
	"strings"
	"testing"
	"unicode/utf8"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

// FuzzNamingFromAPath reads what a file's path says, over any path, and holds the scanner's naming
// to what must be true whatever a library looks like. The naming table's paths seed it, so a plain
// `go test` runs each of them through these rules too.
//
//   - Nothing panics, and the same path reads the same way twice.
//   - A file is a movie or an episode, nothing else.
//   - An episode's last episode, if it has one, comes after its first.
//   - An episode's show is never a season folder: "Season 1" names no show.
//   - A show in a folder is one of the file's own folders, never one beside or below it, unless the
//     file is loose and names its own: in a catch-all folder, or a release folder at the top; or
//     its season pack's release folder stands for the show's folder beside it.
//   - A show's folder name never comes out empty, and a search year is a real one.
func FuzzNamingFromAPath(f *testing.F) {
	for _, row := range namingMatrix {
		for rel := range row.files {
			f.Add(rel)
		}
	}
	for _, rel := range []string{
		"", "/", "a", ".mkv", "S01E01.mkv", "Season 1/S01E01.mkv", "Show/Season 1/Season 1/x.mkv",
		"Show/S01E01-S01E01.mkv", "Show/S99E9999-E0001.mkv", "Show/1x01-1x00.mkv", "{tvdb-1}/S01E02.mkv",
		"Show (2019) {tmdb-0}/Season 0/x.mkv", "Show/Specials/Specials/x.mkv", "x/Season 1/../S01E01.mkv",
		"TV/00/A 0X0", "[](0-0)0/x.mkv", "S0/0/A S0E0", "SeAsonOne/A/0 S0E0", "0/S0/ S/S0/1", "S0/S0/S0 S0e0",
		"TV/Series Seven/Series S0E0/0", "TV/Series Seven/Series Seven S0E0/0", "0/S0/TV/S S0E0", "S0/A/0/A S0E0",
		"TV/ S0/0", "(tV)",
	} {
		f.Add(rel)
	}

	f.Fuzz(func(t *testing.T, rel string) {
		if !utf8.ValidString(rel) || strings.ContainsRune(rel, 0) {
			t.Skip("a path on disk is valid text")
		}
		rel = strings.Trim(path.Clean("/"+rel), "/")
		if rel == "" || rel == "." {
			t.Skip("a file has a name")
		}

		rec, seriesDir := embeddedRecord(File{Rel: rel})
		again, againDir := embeddedRecord(File{Rel: rel})
		if rec.Kind != again.Kind || seriesDir != againDir {
			t.Fatalf("%q read %v in %q, then %v in %q", rel, rec.Kind, seriesDir, again.Kind, againDir)
		}
		switch rec.Kind {
		case mp.KindMovie:
			if seriesDir != "" {
				t.Errorf("%q is a movie with a show folder %q", rel, seriesDir)
			}
		case mp.KindEpisode:
			checkEpisode(t, rel, rec, seriesDir)
		default:
			t.Errorf("%q is a %v", rel, rec.Kind)
		}

		if n, ok := numberingIn(path.Base(rel)); ok && n.EpisodeEnd != 0 && n.EpisodeEnd <= n.Episode {
			t.Errorf("%q holds episodes %d through %d", rel, n.Episode, n.EpisodeEnd)
		}
		for _, name := range strings.Split(rel, "/") {
			if strings.TrimSpace(embeddedID.ReplaceAllString(name, "")) != "" && showFolderName(name) == "" {
				t.Errorf("show folder %q is named nothing", name)
			}
			if _, year, _ := searchQuery(name, 0); year != 0 && (year < 1900 || year > 2099) {
				t.Errorf("%q is searched for with year %d", name, year)
			}
		}
		_ = fallbackName(rel, rec.Kind, 0)
		_ = fallbackName(rel, rec.Kind, 1)
	})
}

// checkEpisode holds an episode's numbering and show to the fuzz's rules.
func checkEpisode(t *testing.T, rel string, rec mp.Record, seriesDir string) {
	t.Helper()
	if n := rec.Numbering; n != nil && n.EpisodeEnd != 0 && n.EpisodeEnd <= n.Episode {
		t.Errorf("%q holds episodes %d through %d", rel, n.Episode, n.EpisodeEnd)
	}
	dir := path.Dir(rel)
	if dir == "." || seriesDir == "" {
		return // a loose episode's show is a name, or the root
	}
	inFolder := dir == seriesDir || strings.HasPrefix(dir, seriesDir+"/")
	// A season pack's release folder stands for its show's folder beside it: "Downloads/Kettle.Bay.
	// S02.1080p.WEB/" for "Downloads/Kettle Bay".
	for d := dir; !inFolder && d != "."; d = path.Dir(d) {
		show := showFolderName(path.Base(d))
		inFolder = show != path.Base(d) && path.Join(path.Dir(d), show) == seriesDir
	}
	if !inFolder {
		// A loose episode is named by its file, not a folder: one in a season folder (or its disc's
		// folder) with no show's folder above, in a catch-all folder ("Downloads/Complete"), in a
		// season's number folder in either ("TV/1/"), or in its own release folder at the top.
		var loose func(dir string) bool
		loose = func(dir string) bool {
			season := seasonDirOf(dir)
			showless := season != "" && (aboveSeasons(season) == "." || catchAllFolder(path.Base(aboveSeasons(season))))
			if showless || catchAllFolder(path.Base(dir)) || episodeMark.MatchString(path.Base(dir)) {
				return true
			}
			base, above := path.Base(dir), path.Dir(dir)
			return digits(base) && len(base) <= 2 && (above == "." || isSeasonFolder(above) || loose(above))
		}
		if strings.Contains(seriesDir, "/") || !loose(dir) {
			t.Errorf("%q files under %q, which is not one of its folders", rel, seriesDir)
		}
		return
	}
	// A show named as a season is ("TV/Series Seven/Series Seven S01E01.mkv") is filed under its own
	// folder when nothing but a catch-all is above it.
	// The episode is named by its file, or by the episode's own folder it is in.
	name := path.Base(rel)
	for d := path.Dir(rel); d != "." && episodeMark.MatchString(path.Base(d)); d = path.Dir(d) {
		name = path.Base(d)
	}
	namedSo := catchAllFolder(path.Base(path.Dir(seriesDir))) && sameWords(looseSeries(name), path.Base(seriesDir))
	if isSeasonFolder(seriesDir) && path.Dir(seriesDir) != "." && !namedSo {
		t.Errorf("%q files under the season folder %q", rel, seriesDir)
	}
}
