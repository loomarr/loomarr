package scan

import (
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// An arr's paths are mapped to the root they're on wherever the arr runs: on Linux, behind a
// trailing slash, or on Windows, where it writes backslashes, a drive letter or a share, and any
// case for them. The longest mapping wins, and a path no mapping covers maps to nothing.
func TestAnArrsPathsMapToTheRootWhereverTheArrRuns(t *testing.T) {
	rows := []librarystore.LibraryRootPath{
		{Authority: "sonarr", AuthorityPath: "/tv/", RootPath: "/mnt/shows"},
		{Authority: "sonarr", AuthorityPath: "/tv/kids", RootPath: "/mnt/kids"},
		{Authority: "radarr", AuthorityPath: `D:\Movies`, RootPath: "/mnt/films"},
		{Authority: "radarr", AuthorityPath: `\\nas\media\4k\`, RootPath: "/mnt/uhd"},
	}
	sonarr, radarr := pathMapper("sonarr", rows), pathMapper("radarr", rows)
	for _, tc := range []struct {
		mapped     func(string) string
		arr, local string
	}{
		{sonarr, "/tv/Kettle Bay/Season 01/Kettle Bay - S01E01.mkv", "/mnt/shows/Kettle Bay/Season 01/Kettle Bay - S01E01.mkv"},
		{sonarr, "/tv/kids/Ferrow Lane/S01E01.mkv", "/mnt/kids/Ferrow Lane/S01E01.mkv"},
		{sonarr, "/tv", "/mnt/shows"},
		{sonarr, "/tvshows/Kettle Bay/x.mkv", ""},
		{radarr, `D:\Movies\Ashgrove (2015)\Ashgrove (2015).mkv`, "/mnt/films/Ashgrove (2015)/Ashgrove (2015).mkv"},
		{radarr, `d:\movies\Ashgrove (2015)\Ashgrove (2015).mkv`, "/mnt/films/Ashgrove (2015)/Ashgrove (2015).mkv"},
		{radarr, `D:/Movies/Ashgrove (2015)/Ashgrove (2015).mkv`, "/mnt/films/Ashgrove (2015)/Ashgrove (2015).mkv"},
		{radarr, `\\NAS\Media\4K\Tinder Hollow (2012)\Tinder Hollow (2012).mkv`, "/mnt/uhd/Tinder Hollow (2012)/Tinder Hollow (2012).mkv"},
		{radarr, `E:\Movies\Ashgrove (2015)\Ashgrove (2015).mkv`, ""},
	} {
		if got := tc.mapped(tc.arr); got != tc.local {
			t.Errorf("%s = %q, want %q", tc.arr, got, tc.local)
		}
	}
}
