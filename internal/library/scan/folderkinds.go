package scan

import (
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Folder kinds: what a folder's name and place say it is before the walk goes in (a by-product
// it passes over, a title's own, its extras), which the walk asks and the folder browser shares.

// skippedDirs hold media-server and NAS by-products, snapshots and recycle bins, never media. A
// ".trash-<uid>" folder is skipped as hidden; a NAS's "@…" folders (QNAP's "@Recycle" and
// "@Recently-Snapshot", Synology's "@eaDir") and a downloader's unfinished "_UNPACK_…" and
// "_FAILED_…" folders by their prefixes (skippedPrefixes). A snapshot holds a second copy of the
// whole library.
var skippedDirs = map[string]bool{
	// Emby's and Kodi's artwork folders: trickplay tiles, extra backdrops and thumbnails, cast photos.
	"trickplay": true, "extrafanart": true, "extrathumbs": true, ".actors": true,
	".recycle.bin": true, "#recycle": true, "$recycle.bin": true, "lost+found": true, "incomplete": true,
	"#snapshot": true, "system volume information": true, "plex versions": true,
	// A Mac's leavings on a share, Windows' older recycle bins, NZBGet's unpacking.
	"network trash folder": true, "temporary items": true, "recycler": true, "recycle bin": true,
	"intermediate": true,
}

// subsDirs are the folders a release keeps its subtitles in.
var subsDirs = map[string]bool{"subs": true, "subtitles": true, "subtitle": true, "sub": true}

// skippedPrefixes begin the names of folders skippedDirs skips by their prefix.
var skippedPrefixes = []string{".", "@", "~", "_unpack_", "_failed_"}

// discDirs are disc structures, which are not imported but listed as problems.
var discDirs = map[string]bool{"video_ts": true, "bdmv": true}

// skippedDir reports whether a walk passes over a folder named name in parent (relative to the
// root, "." its top): a by-product, a recycle bin, a download still coming, a hidden folder, or an
// extras folder in a title's own folder (titled, as holdsTitle says), which a season folder's
// "Specials" is (Emby's). An extras folder in a catch-all ("Movies/Shorts") or a folder of folders
// ("Movies/Kids/Shorts") is a collection of titles, not a title's extras. A disc structure is not
// walked either, but is listed as a problem.
func skippedDir(parent, name string, titled bool) bool {
	lower := strings.ToLower(name)
	// A film's own "Specials" ("Movies/Ashwood (2015)/Specials/") are its extras, where a show's
	// are its season 0; and a subtitles' folder holds no title, whatever a release put in it.
	filmSpecials := lower == "specials" && parent != "." && amongFilms(parent) && !shelfAt(parent)
	// A films' shelf's "Trailers/" or "Samples/" holds its films' trailers and samples, named as
	// the films are; a film called so names its folder with its year, "Trailers (2019)/".
	// "Trailers/" at a root's top, or on a shelf of shelves ("Media/Library/"), holds trailers too.
	// Its "Extras/" is walked: a film may be kept there (shelfBonusExtras).
	shelfTrailers := shelfExtras[lower] && parent != "." && amongFilms(parent) ||
		shelfExtras[lower] && (parent == "." || shelfAt(parent) && !underShows(name) && !showsFolderNames[folderKey(path.Base(parent))])
	return neverMedia(name) || seasonExtrasFolder(name) || filmSpecials || shelfTrailers ||
		(parent != "." && extrasFolder(name, parent) && titled && !shelfAt(parent)) ||
		(lower == "specials" && parent != "." && isSeasonFolder(parent) && !strings.EqualFold(path.Base(parent), "specials"))
}

// neverMedia says whether a folder's name alone says it holds no media, wherever it is: a NAS's,
// an OS's or a downloader's own folder ("@eaDir", "#recycle", ".Trash-1000", "lost+found") or a
// subtitles' folder. The folder browser hides what this says, so it and the scan agree.
func neverMedia(name string) bool {
	lower := strings.ToLower(name)
	// A NAS's own folders have no spaces ("@eaDir", ".Trash-1000"); "@Home Again (2019)" is a title's.
	prefixed := !strings.Contains(lower, " ") && slices.ContainsFunc(skippedPrefixes, func(p string) bool { return strings.HasPrefix(lower, p) })
	return skippedDirs[lower] || prefixed || subsDirs[lower]
}

// holdsTitle says whether a folder, rel to the root with its entries, is a title's own: a season
// folder, one named as a title is with its year ("Ashgrove (2015)"), or one holding a video, a
// disc or a season folder. An extras folder in it is that title's extras.
func holdsTitle(rel string, entries []fs.DirEntry) bool {
	if rel == "." {
		return false
	}
	if title, year, _ := searchQuery(path.Base(rel), 0); isSeasonFolder(rel) || title != "" && year != 0 {
		return true
	}
	for _, e := range entries {
		lower := strings.ToLower(e.Name())
		switch {
		case e.IsDir() && (discDirs[lower] || isSeasonFolder(path.Join(rel, e.Name()))):
			return true
		case !e.IsDir() && (videoExtensions[filepath.Ext(lower)] && !extraIn(e.Name(), path.Base(rel)) || filepath.Ext(lower) == ".iso"):
			return true
		}
	}
	return false
}
