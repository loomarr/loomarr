package filler

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Library sources (§10 V38c, #2151 decision 36) — copying clips out of a folder of the household's
// media library.
//
// A library source names one of the Library's scan roots, a local folder, and reads it where it
// is. It used to name a media-server library and ask the media server for its files; Loomarr's
// own scanner owns those folders now, so the media server is out of the filler path again.
//
// ⚠ It differs from a watched folder in one way that matters: a watched folder is DRAINED (its
// files move into the catalog), while a library folder is the household's own media and is only
// ever COPIED from. Loomarr never moves, renames or deletes anything in it.
//
//   - A folder that cannot be read is logged and SKIPPED. It never fails the sync, so one
//     unmounted volume does not cost the clips every other source holds.
//   - Identity is still the content hash. A path never becomes a clip id, so a folder that is
//     reorganised, moved or deleted cannot orphan a catalogued clip.
//   - Clips are copied into the watch folder and then filed by the one intake like anything else.

// ErrLibraryUnreachableStorage reports that a library source's folder is not one Loomarr can read.
//
// ⚠ **A named error rather than a silent zero result**, because the two are indistinguishable to
// an operator staring at a source that found nothing, and the remedies are completely different:
// an empty folder needs clips added to it, while unreachable storage needs a volume mounted.
var ErrLibraryUnreachableStorage = errors.New("filler: the library folder is not on storage Loomarr can read")

// ScanSource is one source the sync reads from local/LAN storage rather than downloading (§10
// V38c) — a watched folder or a library folder.
//
// ⚠ Declared here, mirroring FetchSource, so `filler` does not import `store`. The adapter at the
// composition root maps `fillerstore.FillerSource` rows whose `Scannable()` is true onto these.
type ScanSource struct {
	// ID is the source row's id, used to attribute a failure to the row an operator can see.
	ID string
	// Kind is "folder" or "library". The two differ only in whether the folder is drained or
	// copied from; both end with files in the watch folder, filed by the one intake.
	Kind string
	// URI is the folder's path.
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

// LibraryScanResult reports what one library scan did.
type LibraryScanResult struct {
	// Found counts the clip files in the folder.
	Found int
	// Copied counts clips written into the watch folder for intake to file.
	Copied int
	// Catalogued counts clips skipped because the catalog already holds them (#1605).
	Catalogued int
	// Unreadable counts clip files Loomarr could not copy. Reported rather than folded into a
	// skip count: a nonzero count here means a STORAGE problem.
	Unreadable int
}

// LibraryScanner copies clips out of a library folder into the watch folder.
//
// ⚠ It writes to the WATCH folder, not the clip folder, and that is what keeps "one pipeline, no
// divergent paths" true. Everything a library yields is hashed, deduplicated, renamed and given a
// sidecar by the same intake that handles a hand-dropped file — so a clip already in the catalog
// from another source is recognised as the duplicate it is rather than filed twice.
type LibraryScanner struct {
	log func(string, ...any)
}

// NewLibraryScanner builds a scanner.
func NewLibraryScanner(log func(string, ...any)) *LibraryScanner {
	return &LibraryScanner{log: log}
}

// Scan copies each clip file under `folder` into `watchDir`, ready for intake.
//
// ⚠ **A clip already copied is not copied again.** Without this, every scheduled scan would
// re-copy the household's whole folder into the watch folder, reading it off disk on a timer
// forever. Two checks keep a re-scan of an unchanged folder at zero copies:
//
//   - a clip still waiting in the watch folder (the destination keeps the source's name) is left
//     alone;
//   - a clip the catalog already holds is skipped by its sparse content identity, which reads two
//     small windows rather than the file. `catalogued` answers that for the identity and the
//     extension the clip would be filed under. It is the only check that survives intake, which
//     moves the copy out of the watch folder, and the transcode, which re-keys the filed clip to
//     its derivative's hash so that intake's own duplicate check no longer sees it (#1605).
//     nil skips this check.
func (s *LibraryScanner) Scan(ctx context.Context, folder, watchDir string, catalogued func(clipID, ext string) bool) (LibraryScanResult, error) {
	var res LibraryScanResult
	if s == nil || folder == "" || watchDir == "" {
		return res, nil
	}
	if info, err := os.Stat(folder); err != nil || !info.IsDir() {
		return res, fmt.Errorf("read library folder %q: %w", folder, ErrLibraryUnreachableStorage)
	}
	if err := os.MkdirAll(watchDir, 0o755); err != nil {
		return res, fmt.Errorf("create watch folder: %w", err)
	}
	watchInside := filepath.Clean(watchDir)

	err := filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if path == folder {
				return err
			}
			return nil //nolint:nilerr // an unreadable subfolder is skipped, not the whole scan
		}
		if d.IsDir() {
			if path != folder && (filepath.Clean(path) == watchInside || strings.HasPrefix(d.Name(), ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if !clipExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		res.Found++
		// ⚠ The destination keeps the ORIGINAL FILENAME, because intake records it as
		// `originalName` and §8 grounds an era only on a year that literally appears in a text
		// signal. Copying `Frosted Flakes 1993.mp4` in as `lib-0007.mp4` would strip the year
		// before intake ever saw it, and the clip would become ungrounded on arrival.
		dst := filepath.Join(watchDir, filepath.Base(path))
		if _, err := os.Stat(dst); err == nil {
			return nil // a file of that name is still waiting for intake; the next scan retries
		}
		if catalogued != nil {
			id, err := ClipID(path)
			if err != nil {
				res.Unreadable++
				return nil
			}
			if catalogued(id, filepath.Ext(path)) {
				res.Catalogued++
				return nil
			}
		}
		if err := copyFile(path, dst); err != nil {
			res.Unreadable++
			return nil
		}
		res.Copied++
		return nil
	})
	if err != nil {
		return res, fmt.Errorf("walk library folder %q: %w", folder, err)
	}
	if res.Found > 0 && res.Copied == 0 && res.Unreadable == res.Found {
		return res, ErrLibraryUnreachableStorage
	}
	return res, nil
}

// cataloguedClips answers LibraryScanner.Scan's `catalogued` question for the clip folder. A clip
// is catalogued when it is filed under its own identity, when a filed clip's transcode re-keyed it
// from that identity, or when it is a known inferior copy that intake would refuse anyway.
//
// The re-keyed identities come from the filed clips' sidecars: the transcode records the source
// master's identity there, and the catalog row keeps only the derivative's hash. Read once per
// sync, and only when a library source is about to be scanned.
func cataloguedClips(clipDir string, inferior map[string]bool) func(clipID, ext string) bool {
	rekeyed := map[string]bool{}
	if filed, err := collectMedia(clipDir); err == nil {
		for _, path := range filed {
			if tags, ok := ReadSidecarTags(path); ok && tags.MediaAssets != nil && tags.MediaAssets.SourceMaster.ClipHash != "" {
				rekeyed[tags.MediaAssets.SourceMaster.ClipHash] = true
			}
		}
	}
	return func(clipID, ext string) bool {
		if rekeyed[clipID] || inferior[clipID] {
			return true
		}
		filed, err := ClipPath(clipDir, clipID, ext)
		if err != nil {
			return false
		}
		_, err = os.Stat(filed)
		return err == nil
	}
}
