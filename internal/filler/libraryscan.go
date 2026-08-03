package filler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Library sources (§10 V38c) — scanning a media-server library for clips.
//
// ⚠ **This reverses V35, which had the library row scanned by nothing at all.** §9.1 had taken
// the media server out of the filler path, so V35 recorded that the row carried no switch because
// there was no work to stop. The maintainer's 2026-08-02 decision restored the kind as real work:
// an operator who already keeps commercials in an Emby/Jellyfin library should be able to point
// Loomarr at it rather than being told to copy files by hand.
//
// ⚠ **What §9.1 forbade is still forbidden, and this file is where that is enforced.** A library
// is ONE source among several, never the catalog's only route:
//
//   - A library that cannot be reached is logged and SKIPPED, exactly like an unreachable archive
//     collection. It never fails the sync, because "no media server ⇒ no commercials" is the
//     dependency §9.1 removed and it must not come back through this door.
//   - Identity is still the content hash. A media-server item id never becomes a clip id, so a
//     library that is re-indexed, moved, or deleted cannot orphan a catalogued clip.
//   - Clips are COPIED into the watch folder and then filed like anything else. Loomarr does not
//     play out of the operator's library in place and never modifies it.

// LibraryClip is one clip as read from a media-server library.
//
// Declared here rather than reusing library.FillerClip so `filler` does not import `library` —
// the same direction rule StoreClip follows. The adapter in the composition root bridges them.
type LibraryClip struct {
	// Name is the item's display name, used for the initial kind/era heuristics and preserved as
	// the clip's original name once it is filed.
	Name string
	// Path is the file's location AS THE MEDIA SERVER SEES IT.
	//
	// ⚠ Usable only when Loomarr and the media server share storage. See LibraryScanner.Scan for
	// what happens when they do not — the answer is an honest report, never a guess.
	Path string
}

// ScanSource is one source the sync reads from local/LAN storage rather than downloading (§10
// V38c) — a watched folder or a media-server library.
//
// ⚠ Declared here, mirroring FetchSource, so `filler` does not import `store`. The adapter at the
// composition root maps `store.FillerSource` rows whose `Scannable()` is true onto these.
type ScanSource struct {
	// ID is the source row's id, used to attribute a failure to the row an operator can see.
	ID string
	// Kind is "folder" or "library". The two differ only in HOW the clips are reached; both end
	// with files in the watch folder, filed by the one intake.
	Kind string
	// URI is the folder path, or the media-server library name.
	URI string
}

// ScanSourceStore lists the sources a sync should read.
//
// ⚠ Separate from FetchStore rather than a method added to it, because the two jobs genuinely
// differ: fetching makes outbound requests on a slow schedule, scanning reads local storage on a
// fast one, and §18.1 gives them separate rows on the Tasks page precisely so an archive.org
// outage does not read as "the filler catalog sync failed".
type ScanSourceStore interface {
	ListScanSources(ctx context.Context) ([]ScanSource, error)
}

// LibraryLister reads the clips in one named media-server library.
//
// Narrow on purpose: a library source needs exactly this, and a wide interface here would make
// the media server look like a bigger dependency of the filler path than it is.
type LibraryLister interface {
	// ListLibraryClips returns the clips in the library with the given NAME. An unknown name is
	// an empty result and no error — the operator typed a name their server does not have, which
	// is a "found nothing" rather than a failure.
	ListLibraryClips(ctx context.Context, libraryName string) ([]LibraryClip, error)
}

// ErrLibraryUnreachableStorage reports that a library's files are not on storage Loomarr can read.
//
// ⚠ **A named error rather than a silent zero result**, because the two are indistinguishable to
// an operator staring at a source that found nothing, and the remedies are completely different:
// an empty library needs clips added to it, while unreachable storage needs a volume mounted.
// A scan that returns "0 clips" for both teaches the operator that the feature is broken.
var ErrLibraryUnreachableStorage = errors.New(
	"filler: the media server's files are not on storage Loomarr can read")

// LibraryScanResult reports what one library scan did.
type LibraryScanResult struct {
	// Found counts clips the media server listed.
	Found int
	// Copied counts clips written into the watch folder for intake to file.
	Copied int
	// Unreadable counts clips whose media-server path Loomarr could not open. Reported rather
	// than folded into Skipped: a nonzero count here means a STORAGE problem, which is a
	// different conversation from a clip that failed for its own reasons.
	Unreadable int
}

// LibraryScanner copies clips out of a media-server library into the watch folder.
//
// ⚠ It writes to the WATCH folder, not the clip folder, and that is what keeps "one pipeline, no
// divergent paths" true. Everything a library yields is hashed, deduplicated, renamed and given a
// sidecar by the same intake that handles a hand-dropped file — so a clip already in the catalog
// from another source is recognised as the duplicate it is rather than filed twice.
type LibraryScanner struct {
	lister LibraryLister
	log    func(string, ...any)
}

// NewLibraryScanner builds a scanner over a media-server client.
func NewLibraryScanner(lister LibraryLister, log func(string, ...any)) *LibraryScanner {
	return &LibraryScanner{lister: lister, log: log}
}

// Scan copies everything in `libraryName` into `watchDir`, ready for intake.
//
// ⚠ **A clip already copied is not copied again.** The destination name is derived from the
// source path, so a re-scan of an unchanged library writes nothing — without this, every scheduled
// scan would re-copy the entire library into the watch folder, and intake would dutifully hash and
// discard all of it as duplicates. Correct, but it would read the whole library from disk on a
// timer forever.
func (s *LibraryScanner) Scan(ctx context.Context, libraryName, watchDir string) (LibraryScanResult, error) {
	var res LibraryScanResult
	if s == nil || s.lister == nil || libraryName == "" || watchDir == "" {
		return res, nil
	}

	clips, err := s.lister.ListLibraryClips(ctx, libraryName)
	if err != nil {
		// ⚠ Returned, but the CALLER logs and continues to the next source. The sync must not die
		// because one media server is down — see this file's header.
		return res, fmt.Errorf("list library %q: %w", libraryName, err)
	}
	res.Found = len(clips)
	if err := os.MkdirAll(watchDir, 0o755); err != nil {
		return res, fmt.Errorf("create watch folder: %w", err)
	}

	for _, c := range clips {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		if c.Path == "" {
			res.Unreadable++
			continue
		}
		// ⚠ The destination keeps the ORIGINAL FILENAME, because intake records it as
		// `originalName` and §8 grounds an era only on a year that literally appears in a text
		// signal. Copying `Frosted Flakes 1993.mp4` in as `lib-0007.mp4` would strip the year
		// before intake ever saw it, and the clip would become ungrounded on arrival.
		dst := filepath.Join(watchDir, filepath.Base(c.Path))
		if _, err := os.Stat(dst); err == nil {
			continue // already waiting for intake
		}
		if err := copyFile(c.Path, dst); err != nil {
			res.Unreadable++
			continue
		}
		res.Copied++
	}

	// ⚠ Nothing readable at all, with clips listed, is a STORAGE problem rather than an empty
	// library — the media server told us about files we cannot open, which happens when Loomarr
	// and the media server do not share a mount. Named so the UI can say "mount the volume"
	// instead of "this library has no clips".
	if res.Found > 0 && res.Copied == 0 && res.Unreadable == res.Found {
		return res, ErrLibraryUnreachableStorage
	}
	return res, nil
}
