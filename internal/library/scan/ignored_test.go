package scan

import (
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// An ignored movie is its file and an ignored show is its folder, above any season folder, each
// under a folder label that starts at the media folder's name.
func TestIgnoredTitleNamesWhatIsOnDisk(t *testing.T) {
	series := librarystore.LibraryOrigin{Key: seriesOrigin("/srv/TV", "Unsorted/Night Ferry").ExternalItemID}
	for _, tc := range []struct {
		title        librarystore.LibraryIgnoredTitle
		path         string
		name, folder string
	}{
		{librarystore.LibraryIgnoredTitle{Kind: "movie", Name: "Clip"}, "/srv/TV/Unsorted/clip.mkv", "clip.mkv", "TV/Unsorted"},
		{librarystore.LibraryIgnoredTitle{Kind: "movie", Name: "Clip"}, "/srv/TV/clip.mkv", "clip.mkv", "TV"},
		{librarystore.LibraryIgnoredTitle{Kind: "series", Name: "Night Ferry", Origin: series},
			"/srv/TV/Unsorted/Night Ferry/Season 1/e1.mkv", "Night Ferry", "TV/Unsorted"},
		{librarystore.LibraryIgnoredTitle{Kind: "series", Name: "Paper Kites"}, "/srv/TV/Loose/e1.mkv", "Paper Kites", "TV/Loose"},
	} {
		got := ignoredTitle(tc.title, []librarystore.LibraryFile{{Root: "/srv/TV", Path: tc.path}})
		if got.Name != tc.name || got.Folder != tc.folder {
			t.Errorf("%s: name %q folder %q, want %q %q", tc.path, got.Name, got.Folder, tc.name, tc.folder)
		}
	}
}
