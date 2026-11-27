package scan

import (
	"context"
	"fmt"
	"path/filepath"
)

// RunFolders makes a pass over the named folders of one walked root: the ones a change signal
// named. It writes and removes only what is under them, under the same removal guard, and does not
// count as the root's scan. A root that is not walked on its own is left to the full pass.
func (im *Importer) RunFolders(ctx context.Context, root string, dirs []string) error {
	read, err := im.WalkFolders(ctx, root, dirs)
	if err != nil {
		return err
	}
	return read()
}

// WalkFolders is RunFolders without its header reads, which it returns, as Walk does: a caller
// running them in the background passes the next folders at once, rather than behind a stage that
// waits while the household watches (Scanner.passFolders).
func (im *Importer) WalkFolders(ctx context.Context, root string, dirs []string) (read func() error, err error) {
	if len(dirs) == 0 || (im.Enabled != nil && !im.Enabled()) {
		return func() error { return nil }, nil
	}
	queue, err := im.importFolders(ctx, root, dirs)
	if err != nil {
		return nil, err
	}
	return func() error {
		_, err := im.readHeaders(ctx, queue)
		return err
	}, nil
}

// importFolders walks and imports the named folders of one root, and returns the files waiting
// for their header.
func (im *Importer) importFolders(ctx context.Context, root string, dirs []string) ([]pending, error) {
	im.run.Lock()
	defer im.run.Unlock()
	all, err := im.Store.LibraryRoots(ctx)
	if err != nil {
		return nil, err
	}
	if !scannedRoots(all.Roots)[root] {
		return nil, nil
	}
	idx := im.arrIndex(ctx, all.Paths, false)
	im.setProgress(func(p *Progress) { p.ArrIncomplete = !idx.complete })
	// A folder pass is a few folders a change named: it is not step 1 of an import, and shows
	// only the header reads it queues.
	result, queue, err := im.scanRoot(ctx, root, idx, dirs, nil)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	im.recordReadability(ctx, result)
	im.setProgress(func(p *Progress) {
		for i := range p.Roots {
			if p.Roots[i].Root == root {
				// The names it could not read outside the folders it walked still stand.
				for _, rel := range p.Roots[i].Unreadable {
					if !under(filepath.Join(root, filepath.FromSlash(rel)), dirs) {
						result.Unreadable = append(result.Unreadable, rel)
					}
				}
				p.Roots[i] = result
			}
		}
	})
	return queue, nil
}
