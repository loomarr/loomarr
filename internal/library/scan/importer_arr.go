package scan

import (
	"context"
	"errors"
	"fmt"
	"slices"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// ArrChanged notes that an arr changed item, or changed files without naming the item (nil): the
// next folder pass reads it again, or lists every arr again, before trusting its index.
func (im *Importer) ArrChanged(item *ArrItem) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if item == nil {
		im.relist = true
		return
	}
	im.arrChanged = append(im.arrChanged, *item)
}

// arrIndex is what the arrs say each local file is. A full pass lists every arr, as does a folder
// pass with no index yet, an index from a listing an arr failed, a changed path map, or a change
// it cannot read alone. Otherwise a folder pass reuses the last listing, reading again just the
// items an arr said changed: Sonarr's listing costs two requests per series. Call it holding run.
func (im *Importer) arrIndex(ctx context.Context, paths []librarystore.LibraryRootPath, full bool) managerIndex {
	im.mu.Lock()
	changed, relist := im.arrChanged, im.relist
	im.arrChanged, im.relist = nil, false
	im.mu.Unlock()

	var managers []Manager
	if im.Managers != nil {
		managers = im.Managers()
	}
	key := fmt.Sprint(paths)
	for _, m := range managers {
		if m.Address != nil {
			key += "|" + m.Authority + "=" + m.Address()
		}
	}
	retry := im.index != nil && !im.index.complete && im.now().Sub(im.indexedAt) >= arrRetry
	if full || relist || im.index == nil || retry || im.indexPaths != key {
		return im.listArrs(ctx, managers, paths, key)
	}
	if !im.index.complete {
		// An arr failed a moment ago: asking it again on every folder pass only waits on it
		// again. Its folders' files wait; the rest pass on what the others listed.
		return *im.index
	}
	for _, item := range changed {
		if !im.refreshArrItem(ctx, managers, paths, item) {
			return im.listArrs(ctx, managers, paths, key)
		}
	}
	return *im.index
}

// arrWithoutFolders reports whether a connected arr has no folder in the path map.
func (im *Importer) arrWithoutFolders(paths []librarystore.LibraryRootPath) bool {
	if im.Managers == nil {
		return false
	}
	for _, m := range im.Managers() {
		if m.Address == nil || m.Address() == "" {
			continue
		}
		if !slices.ContainsFunc(paths, func(p librarystore.LibraryRootPath) bool { return p.Authority == m.Authority }) {
			return true
		}
	}
	return false
}

func (im *Importer) listArrs(ctx context.Context, managers []Manager, paths []librarystore.LibraryRootPath, key string) managerIndex {
	idx, errs := indexManagers(ctx, managers, paths)
	for _, err := range errs {
		im.log().Warn("library scan: an arr did not list", "err", err)
	}
	im.index, im.indexPaths, im.indexedAt = &idx, key, im.now()
	return idx
}

// refreshArrItem reads one changed item into the index. It reports false when it cannot, and the
// arr must be listed whole instead.
func (im *Importer) refreshArrItem(ctx context.Context, managers []Manager, paths []librarystore.LibraryRootPath, item ArrItem) bool {
	for _, m := range managers {
		if m.Authority != item.Authority {
			continue
		}
		if m.Fetcher == nil {
			return false
		}
		rec, err := m.Fetcher.Fetch(ctx, item.Ref)
		switch {
		case errors.Is(err, mp.ErrNotFound):
			im.index.refresh(m.Authority, pathMapper(m.Authority, paths), item.Ref, nil)
		case err != nil:
			im.log().Warn("library scan: an arr did not answer for one item; listing it whole",
				"arr", m.Authority, "err", err)
			return false
		default:
			im.index.refresh(m.Authority, pathMapper(m.Authority, paths), item.Ref, &rec)
		}
		return true
	}
	return true // an arr no longer configured manages nothing
}
