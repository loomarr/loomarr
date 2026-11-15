package scan

import (
	"context"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/library/inventory"
)

// ForgetReleased forgets the files of every root no longer walked, now, outside a pass: "Stop
// using" takes effect at once rather than after the running pass's reading. A released root the
// running pass is walking is stopped, and the import forgets its files itself before it goes on
// to the next root; one it listed before the release is not written (importListed).
func (im *Importer) ForgetReleased(ctx context.Context) error {
	all, err := im.Store.LibraryRoots(ctx)
	if err != nil {
		return err
	}
	// A released root the running import is walking stops now, rather than being walked and
	// written in full before the import lets go of the lock below.
	inUse := scannedRoots(all.Roots)
	im.mu.Lock()
	for root, cancel := range im.walking {
		if !inUse[root] {
			cancel(errReleased)
		}
	}
	im.mu.Unlock()
	im.run.Lock()
	defer im.run.Unlock()
	if all, err = im.Store.LibraryRoots(ctx); err != nil {
		return err
	}
	return im.forgetReleased(ctx, scannedRoots(all.Roots))
}

// releasedWithin lists the folders inside root whose files this importer forgot when they stopped
// being used: those a Replace of root took the place of, among others. A folder added by hand is
// gone from the roots once released, so only the importer still knows it.
func (im *Importer) releasedWithin(root string) []string {
	im.mu.Lock()
	defer im.mu.Unlock()
	var out []string
	for r := range im.forgotten {
		if r != root && inside(r, root) {
			out = append(out, r)
		}
	}
	return out
}

// headerWithin is the header a released folder read for file, at the same path and with the same
// bytes: the file's own, read before the folder was released and added again, or before the
// outer folder replaced the inner. A file the root already knew, one whose bytes have changed
// since, or one no released folder read, has none.
func (im *Importer) headerWithin(ctx context.Context, released []string, file File, known bool) (*inventory.SourceFacts, int) {
	if known {
		return nil, 0
	}
	for _, folder := range released {
		if !inside(file.Path, folder) {
			continue
		}
		rel, err := filepath.Rel(folder, file.Path)
		if err != nil {
			continue
		}
		read, by := im.lastHeader(ctx, fileOrigin(folder, filepath.ToSlash(rel)))
		if read != nil && read.SizeBytes == file.Stat.Size && read.ModifiedUnixNano == file.Stat.ModifiedNs {
			return read, by
		}
	}
	return nil, 0
}

// forgetReleased marks every file of a root no longer walked unseen and forgets its rows: releasing
// a root is the maintainer's word, not a walk's guess, so the removal guard does not apply.
func (im *Importer) forgetReleased(ctx context.Context, inUse map[string]bool) error {
	scanned, err := im.Store.LibraryFileRoots(ctx)
	if err != nil {
		return err
	}
	for _, root := range scanned {
		if inUse[root] {
			continue
		}
		if err := im.Inventory.MarkUnseen(ctx, Authority(root), im.now(), nil); err != nil {
			return err
		}
		if err := im.Store.DeleteLibraryFiles(ctx, root); err != nil {
			return err
		}
		// The header stage may still hold the root's files: their reads are stale now, so the
		// stage neither reads them nor saves the forgotten rows back.
		im.gen++
		im.mu.Lock()
		if im.forgotten == nil {
			im.forgotten = map[string]int64{}
		}
		im.forgotten[root] = im.gen
		im.mu.Unlock()
		im.titlesChanged()
	}
	return nil
}
