package app

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// fillerScanSourceAdapter bridges the store → cliplibrary.ScanSourceStore (§10 V38c).
//
// ⚠ The `Enabled && Scannable()` filter lives HERE rather than in the syncer, matching where the
// fetch path puts its `Enabled && Fetchable()` check. Both predicates are the store's domain
// knowledge about a row; re-deriving them inside the job is how the two start disagreeing about
// what a source is for.
type fillerScanSourceAdapter struct {
	st records.FillerSourceStore
	// cutover rewrites a library row still naming a media-server library before it is listed;
	// nil lists rows as they are.
	cutover *fillerLibraryCutover
}

func (a fillerScanSourceAdapter) ListScanSources(ctx context.Context) ([]cliplibrary.ScanSource, error) {
	srcs, err := a.st.ListFillerSources(ctx)
	if err != nil {
		return nil, err
	}
	if a.cutover != nil {
		a.cutover.rewrite(ctx, srcs)
	}
	out := make([]cliplibrary.ScanSource, 0, len(srcs))
	for _, s := range srcs {
		if s.EffectiveEnabled() && s.Scannable() {
			out = append(out, cliplibrary.ScanSource{ID: s.ID, Kind: s.Kind, URI: s.URI})
		}
	}
	return out, nil
}

// fillerLibraryCutover rewrites a library filler source that still names a media-server library
// into the household library's folder for it (#2151 decision 36): the library's folders, as the
// media server reports them, through the scan roots' path map. A library with several folders
// gets a row for each. A name that doesn't resolve yet (no media server, no mapped folder) is
// left for a later sync and does no work meanwhile.
type fillerLibraryCutover struct {
	st    records.FillerSourceStore
	roots interface {
		LibraryRoots(ctx context.Context) (librarystore.LibraryRoots, error)
	}
	lib *mediaserver.Client
	log *slog.Logger
	now func() time.Time

	mu     sync.Mutex
	warned map[string]bool
}

// rewrite rewrites srcs in place as it rewrites their rows.
func (c *fillerLibraryCutover) rewrite(ctx context.Context, srcs []records.FillerSource) {
	for i, s := range srcs {
		if s.Kind != string(source.LibraryID) || s.URI == "" || filepath.IsAbs(s.URI) {
			continue
		}
		folders, err := c.folders(ctx, s.URI)
		if len(folders) == 0 {
			c.warnOnce(s, err)
			continue
		}
		name := s.URI
		s.URI = folders[0]
		if err := c.st.UpsertFillerSource(ctx, s); err != nil {
			c.log.Warn("filler: could not rewrite a library source to its folder", "source", s.ID, "folder", s.URI, "err", err)
			continue
		}
		srcs[i] = s
		c.log.Info("filler: a library source now reads its library folder", "source", s.ID, "library", name, "folder", s.URI)
		for _, folder := range folders[1:] {
			extra := records.NewFillerSource(string(source.LibraryID)+":"+folder, s.Kind, folder, s.Label, c.now())
			extra.Enabled = s.Enabled
			if err := c.st.UpsertFillerSource(ctx, extra); err != nil {
				c.log.Warn("filler: could not add a library source for another of its folders", "library", name, "folder", folder, "err", err)
			}
		}
	}
}

// folders are the local folders of the media-server library called name.
func (c *fillerLibraryCutover) folders(ctx context.Context, name string) ([]string, error) {
	lib := c.lib.Snapshot()
	server, err := lib.LibraryFolders(ctx, name)
	if err != nil || len(server) == 0 {
		return nil, err
	}
	flavor, err := lib.FlavorName()
	if err != nil {
		return nil, err
	}
	roots, err := c.roots.LibraryRoots(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, folder := range server {
		for _, p := range roots.Paths {
			if p.Authority == flavor && p.AuthorityPath == folder && p.RootPath != "" {
				out = append(out, p.RootPath)
				break
			}
		}
	}
	return out, nil
}

// warnOnce says once per process why a library source still does no work.
func (c *fillerLibraryCutover) warnOnce(s records.FillerSource, err error) {
	if errors.Is(err, mediaserver.ErrConnectionRequired) {
		err = nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.warned[s.ID] {
		return
	}
	c.warned[s.ID] = true
	c.log.Warn("filler: a library source names a media-server library with no folder Loomarr can read; "+
		"it does nothing until it is set to a folder", "source", s.ID, "library", s.URI, "err", err)
}
