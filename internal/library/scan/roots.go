// Package scan finds and walks the folders Loomarr reads media from (#2151): it discovers scan
// roots from the media server and the arrs, maps each authority's folder names onto local paths,
// and checks that Loomarr can open them.
package scan

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/store"
)

// ErrNotAbsolute rejects a folder that is not an absolute path.
var ErrNotAbsolute = errors.New("scan: a folder must be an absolute path")

// ErrUnreadable rejects a hand-typed folder Loomarr cannot open.
var ErrUnreadable = errors.New("scan: Loomarr cannot open this folder")

// Source is one authority that reports library folders by its own names: the media server
// ("emby" or "jellyfin"), "sonarr" or "radarr".
type Source struct {
	Authority string
	// Folders lists the authority's folders. ErrSkipped means the authority is not set up, which is
	// neither an answer nor a failure.
	Folders func(ctx context.Context) ([]string, error)
}

// ErrSkipped reports a source that is not configured.
var ErrSkipped = errors.New("scan: source not configured")

// Disk is what discovery needs from the local filesystem.
type Disk interface {
	// Readable reports whether a directory can be opened and listed.
	Readable(ctx context.Context, path string) bool
	// Mounts lists the mount points a folder named by another machine may live under.
	Mounts() []string
}

// SourceResult is how one source answered a discovery pass.
type SourceResult struct {
	Authority string
	// State is "ok", "skipped" (not set up) or "failed".
	State string
	Error string
}

// Roots discovers, checks and records scan roots. Each call reads its sources and path map anew,
// so a saved connection or mapping applies on the next call.
type Roots struct {
	Store   store.LibraryRootStore
	Sources func() []Source
	// PathMap is the `library.path_map` setting, tried before a mount search.
	PathMap func() library.PathMap
	Disk    Disk
	Now     func() time.Time

	// mu serializes discovery with the maintainer's edits, so a pass never applies stale checks
	// over an edit that landed while it ran.
	mu sync.Mutex
}

// Health is the library's one-line state for the status strip.
type Health string

const (
	// HealthNeedsFolders: no folder is in use yet, so nothing can be scanned.
	HealthNeedsFolders Health = "needs_folders"
	// HealthFolderOffline: a folder in use could not be opened at its last check.
	HealthFolderOffline Health = "folder_offline"
	// HealthOK: every folder in use opened at its last check.
	HealthOK Health = "ok"
)

// Status counts the roots for the status strip.
type Status struct {
	Health Health
	// InUse and Offline count confirmed folders; ToConfirm counts discovered folders that map to
	// a readable local folder and wait for the maintainer.
	InUse, Offline, ToConfirm int
}

// StatusOf summarizes recorded roots.
func StatusOf(all store.LibraryRoots) Status {
	var s Status
	for _, root := range all.Roots {
		switch {
		case root.InUse && !root.Readable:
			s.InUse++
			s.Offline++
		case root.InUse:
			s.InUse++
		case root.Readable:
			s.ToConfirm++
		}
	}
	switch {
	case s.InUse == 0:
		s.Health = HealthNeedsFolders
	case s.Offline > 0:
		s.Health = HealthFolderOffline
	default:
		s.Health = HealthOK
	}
	return s
}

// List reads the recorded roots and path map.
func (r *Roots) List(ctx context.Context) (store.LibraryRoots, error) {
	return r.Store.LibraryRoots(ctx)
}

// Discover asks every source for its folders, derives a local folder for each, checks every root
// and records the outcome. A source that fails keeps what it reported last time.
func (r *Roots) Discover(ctx context.Context) (store.LibraryRoots, []SourceResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	before, err := r.Store.LibraryRoots(ctx)
	if err != nil {
		return store.LibraryRoots{}, nil, err
	}
	handSet := map[[2]string]bool{}
	for _, p := range before.Paths {
		if p.ByHand {
			handSet[[2]string{p.Authority, p.AuthorityPath}] = true
		}
	}

	var d store.LibraryRootDiscovery
	var results []SourceResult
	mounts := r.Disk.Mounts()
	for _, src := range r.Sources() {
		folders, err := src.Folders(ctx)
		switch {
		case errors.Is(err, ErrSkipped):
			results = append(results, SourceResult{Authority: src.Authority, State: "skipped"})
			continue
		case err != nil:
			results = append(results, SourceResult{Authority: src.Authority, State: "failed", Error: err.Error()})
			continue
		}
		results = append(results, SourceResult{Authority: src.Authority, State: "ok"})
		d.Complete = append(d.Complete, src.Authority)
		for _, folder := range folders {
			folder = cleanFolder(folder)
			if folder == "" {
				continue
			}
			found := store.LibraryRootPath{Authority: src.Authority, AuthorityPath: folder}
			if !handSet[[2]string{src.Authority, folder}] {
				found.RootPath = r.derive(ctx, folder, mounts)
			}
			d.Found = append(d.Found, found)
		}
	}

	// Check every root that will exist after this pass: the ones kept and the ones just derived.
	candidates := map[string]bool{}
	for _, root := range before.Roots {
		candidates[root.Path] = true
	}
	for _, p := range before.Paths {
		if p.RootPath != "" {
			candidates[p.RootPath] = true
		}
	}
	for _, f := range d.Found {
		if f.RootPath != "" {
			candidates[f.RootPath] = true
		}
	}
	for path := range candidates {
		d.Checks = append(d.Checks, store.LibraryRootCheck{Path: path, Readable: r.Disk.Readable(ctx, path)})
	}
	slices.SortFunc(d.Checks, func(a, b store.LibraryRootCheck) int { return strings.Compare(a.Path, b.Path) })

	if err := r.Store.RecordLibraryRootDiscovery(ctx, d, r.Now()); err != nil {
		return store.LibraryRoots{}, results, err
	}
	after, err := r.Store.LibraryRoots(ctx)
	return after, results, err
}

// Add records a folder the maintainer typed, confirmed. It must be absolute and readable.
func (r *Roots) Add(ctx context.Context, path string) error {
	path, err := absoluteFolder(path)
	if err != nil {
		return err
	}
	if !r.Disk.Readable(ctx, path) {
		return ErrUnreadable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Store.AddLibraryRoot(ctx, store.LibraryRootCheck{Path: path, Readable: true}, r.Now())
}

// SetInUse confirms or releases a root.
func (r *Roots) SetInUse(ctx context.Context, path string, inUse bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Store.SetLibraryRootInUse(ctx, path, inUse)
}

// SetMapping points an authority's folder at a local folder by hand, and checks it. An empty
// local folder hands the row back to discovery.
func (r *Roots) SetMapping(ctx context.Context, authority, authorityPath, local string) error {
	if local != "" {
		var err error
		if local, err = absoluteFolder(local); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	at := r.Now()
	if err := r.Store.SetLibraryRootPath(ctx, authority, authorityPath, local, at); err != nil {
		return err
	}
	if local == "" {
		return nil
	}
	return r.Store.RecordLibraryRootDiscovery(ctx, store.LibraryRootDiscovery{
		Checks: []store.LibraryRootCheck{{Path: local, Readable: r.Disk.Readable(ctx, local)}},
	}, at)
}

// derive finds the local folder for an authority's folder: the same path when Loomarr can read it,
// else the `library.path_map` translation, else the one mount that holds the folder's trailing
// names. "" when nothing, or more than one folder, fits.
//
// A root is always absolute: a relative name would resolve against wherever Loomarr happens to run.
func (r *Roots) derive(ctx context.Context, folder string, mounts []string) string {
	if filepath.IsAbs(folder) && r.Disk.Readable(ctx, folder) {
		return folder
	}
	if r.PathMap != nil {
		if local, ok := r.PathMap().Apply(folder); ok && filepath.IsAbs(local) && r.Disk.Readable(ctx, filepath.Clean(local)) {
			return filepath.Clean(local)
		}
	}
	names := strings.Split(strings.Trim(folder, "/"), "/")
	// The longest matching tail is the most specific; a tie at that length is ambiguous.
	for k := len(names); k >= 1; k-- {
		tail := filepath.Join(names[len(names)-k:]...)
		var hits []string
		for _, mount := range mounts {
			candidates := []string{filepath.Join(mount, tail)}
			if k == 1 && filepath.Base(mount) == tail {
				candidates = append(candidates, mount)
			}
			for _, c := range candidates {
				if c != folder && !slices.Contains(hits, c) && r.Disk.Readable(ctx, c) {
					hits = append(hits, c)
				}
			}
		}
		switch len(hits) {
		case 0:
			continue
		case 1:
			return hits[0]
		default:
			return ""
		}
	}
	return ""
}

func cleanFolder(folder string) string {
	folder = strings.TrimSpace(folder)
	if folder == "" || !strings.HasPrefix(folder, "/") {
		// A Windows media server's `D:\TV` cannot be opened here as named; only a mapping can.
		return strings.TrimRight(folder, `\`)
	}
	return filepath.Clean(folder)
}

func absoluteFolder(path string) (string, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: %q", ErrNotAbsolute, path)
	}
	return filepath.Clean(path), nil
}
