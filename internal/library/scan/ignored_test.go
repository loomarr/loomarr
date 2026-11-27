package scan

import (
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// An ignored movie is its file and an ignored show is its folder, above any season folder, each
// under a folder label that starts at its media folder's name, or its path when two share a name.
// Only the files under the title's own media folder name it.
func TestIgnoredTitleNamesWhatIsOnDisk(t *testing.T) {
	origin := func(root, dir string) librarystore.LibraryOrigin {
		o := seriesOrigin(root, dir)
		return librarystore.LibraryOrigin{Authority: string(o.Authority), Key: o.ExternalItemID}
	}
	at := func(root, rel string) librarystore.LibraryFile {
		return librarystore.LibraryFile{Root: root, Path: root + "/" + rel}
	}
	movie := librarystore.LibraryIgnoredTitle{Kind: "movie", Name: "Clip", Origin: origin("/srv/tv", "")}
	show := librarystore.LibraryIgnoredTitle{Kind: "series", Name: "Night Ferry", Origin: origin("/srv/tv", "Unsorted/Night Ferry")}
	loose := librarystore.LibraryIgnoredTitle{Kind: "series", Name: "Paper Kites", Origin: origin("/srv/tv", "paper kites")}
	one := []string{"/srv/tv"}
	for what, tc := range map[string]struct {
		title        librarystore.LibraryIgnoredTitle
		files        []librarystore.LibraryFile
		roots        []string
		name, folder string
	}{
		"movie": {movie, []librarystore.LibraryFile{at("/srv/tv", "Unsorted/clip.mkv")}, one, "clip.mkv", "TV/Unsorted"},
		"movie at the top": {movie, []librarystore.LibraryFile{at("/srv/tv", "clip.mkv")}, one, "clip.mkv", "TV"},
		"show": {show, []librarystore.LibraryFile{at("/srv/tv", "Unsorted/Night Ferry/Season 1/e1.mkv")}, one,
			"Night Ferry", "TV/Unsorted"},
		"show with no folder": {loose, []librarystore.LibraryFile{at("/srv/tv", "Loose/e1.mkv")}, one, "Paper Kites", "TV/Loose"},
		"show joined from another folder that sorts first": {show, []librarystore.LibraryFile{
			at("/srv/tv", "Downloads/ferry/e2.mkv"), at("/srv/tv", "Unsorted/Night Ferry/e1.mkv")}, one,
			"Night Ferry", "TV/Unsorted"},
		"a copy under another media folder sorts first": {movie, []librarystore.LibraryFile{
			at("/mnt/other", "copy.mkv"), at("/srv/tv", "Unsorted/clip.mkv")}, []string{"/mnt/other", "/srv/tv"},
			"clip.mkv", "TV/Unsorted"},
		"two media folders with one name": {movie, []librarystore.LibraryFile{at("/srv/tv", "Unsorted/clip.mkv")},
			[]string{"/mnt/TV", "/srv/tv"}, "clip.mkv", "/srv/tv/Unsorted"},
	} {
		got := ignoredTitle(tc.title, tc.files, tc.roots)
		if got.Name != tc.name || got.Folder != tc.folder {
			t.Errorf("%s: name %q folder %q, want %q %q", what, got.Name, got.Folder, tc.name, tc.folder)
		}
	}
}
