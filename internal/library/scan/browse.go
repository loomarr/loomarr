package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The folder browser (#2151): Settings → Library picks a folder by clicking down from what this
// machine has mounted, the folders the arrs name first, instead of typing its path. It shows
// folders only, never above a mount, and hides what the scan never reads (neverMedia). Each
// folder's summary, what one look inside it finds, is counted after the list is answered and
// heard through Summarized: a share that answers slowly never holds the list up.

// ErrNotBrowsable refuses a folder outside every mount and every folder an app names.
var ErrNotBrowsable = errors.New("scan: the folder is outside every mount")

// RootOverlap refuses a folder that overlaps one in use: it is inside one, or holds some.
type RootOverlap struct {
	// Within is the folder in use that holds it, "" when none does.
	Within string
	// Holds are the folders in use inside it.
	Holds []string
}

func (e *RootOverlap) Error() string {
	if e.Within != "" {
		return fmt.Sprintf("scan: the folder is part of %s, which is in use", e.Within)
	}
	return fmt.Sprintf("scan: the folder holds %d folders in use", len(e.Holds))
}

// BrowseFolder is one folder the browser lists.
type BrowseFolder struct {
	Path string
	// Name is what the row says: the folder's own name, or its whole path at the top.
	Name string
	// Named: an app names it, so it is listed first at the top.
	Named bool
	InUse bool
	// Within is the folder in use that holds it, "" when none does.
	Within string
	// Holds are the folders in use inside it.
	Holds []string
	// Summary is what a look inside found, nil until it is counted.
	Summary *FolderSummary
}

// Browsed is one folder's listing; at the top, Path is "".
type Browsed struct {
	Path string
	// Parent is the folder above, "" when that is the top.
	Parent string
	// InUse, Within and Holds are Path's own, as a row of its parent says them: what "Use this
	// folder" may do with it.
	InUse   bool
	Within  string
	Holds   []string
	Folders []BrowseFolder
}

// SummaryState is how a look inside a folder went.
type SummaryState string

const (
	SummaryCounted    SummaryState = "counted"
	SummarySlow       SummaryState = "slow"
	SummaryUnreadable SummaryState = "unreadable"
)

// FolderSummary is what one look inside a folder found: the folders the scan would walk and the
// videos it would read, one level down.
type FolderSummary struct {
	Path    string
	Folders int
	Videos  int
	State   SummaryState
}

const (
	// summaryTimeout bounds one look inside a folder; past it the folder reads "slow".
	summaryTimeout = 3 * time.Second
	// summaryFresh is how long a summary is answered from memory before it is counted again.
	summaryFresh = 5 * time.Minute
	// summaryWorkers is how many folders are looked inside at once.
	summaryWorkers = 4
)

type summaryAt struct {
	summary FolderSummary
	at      time.Time
}

// Browse lists the folders in path; "" lists the top: the folders the apps name, then the mounts.
func (r *Roots) Browse(ctx context.Context, path string) (Browsed, error) {
	all, err := r.Store.LibraryRoots(ctx)
	if err != nil {
		return Browsed{}, err
	}
	var inUse, named []string
	for _, root := range all.Roots {
		if root.InUse {
			inUse = append(inUse, root.Path)
		}
	}
	for _, p := range all.Paths {
		if p.RootPath != "" && !slices.Contains(named, p.RootPath) {
			named = append(named, p.RootPath)
		}
	}
	slices.Sort(named)
	mounts := slices.Sorted(slices.Values(r.Disk.Mounts()))
	bases := append(slices.Clone(named), mounts...)

	var out Browsed
	if strings.TrimSpace(path) == "" {
		for _, p := range named {
			out.Folders = append(out.Folders, BrowseFolder{Path: p, Name: p, Named: true})
		}
		for _, p := range mounts {
			if !slices.Contains(named, p) {
				out.Folders = append(out.Folders, BrowseFolder{Path: p, Name: p})
			}
		}
	} else {
		if path, err = absoluteFolder(path); err != nil {
			return Browsed{}, err
		}
		if !browsable(path, bases) {
			return Browsed{}, ErrNotBrowsable
		}
		names, err := listFolders(ctx, path)
		if err != nil {
			return Browsed{}, err
		}
		out.Path = path
		if up := filepath.Dir(path); !slices.Contains(bases, path) && browsable(up, bases) {
			out.Parent = up
		}
		for _, name := range names {
			p := filepath.Join(path, name)
			if browsable(p, bases) {
				out.Folders = append(out.Folders, BrowseFolder{Path: p, Name: name, Named: slices.Contains(named, p)})
			}
		}
	}
	if out.Path != "" {
		out.InUse = slices.Contains(inUse, out.Path)
		out.Within, out.Holds = overlapOf(out.Path, inUse)
	}
	paths := make([]string, len(out.Folders))
	for i := range out.Folders {
		f := &out.Folders[i]
		paths[i] = f.Path
		f.InUse = slices.Contains(inUse, f.Path)
		f.Within, f.Holds = overlapOf(f.Path, inUse)
		f.Summary = r.knownSummary(f.Path)
	}
	r.summarize(paths)
	return out, nil
}

// browsable says whether path, as it really is with its links followed, is a base or inside one:
// a link inside a mount that leads out of every mount is never followed.
func browsable(path string, bases []string) bool {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	for _, base := range bases {
		b, err := filepath.EvalSymlinks(base)
		if err != nil {
			b = base
		}
		if real == b || strings.HasPrefix(real, strings.TrimSuffix(b, string(filepath.Separator))+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// overlapOf is the folder in use that holds path, and the folders in use path holds.
func overlapOf(path string, inUse []string) (within string, holds []string) {
	for _, root := range inUse {
		switch {
		case root == path:
		case inside(path, root):
			within = root
		case inside(root, path):
			holds = append(holds, root)
		}
	}
	slices.Sort(holds)
	return within, holds
}

// inside says whether path is strictly inside dir.
func inside(path, dir string) bool {
	return strings.HasPrefix(path, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

// listFolders is the names of the folders in path the scan would walk, in name order, within
// readableTimeout. A folder reached by a link counts when it leads to a folder.
func listFolders(ctx context.Context, path string) ([]string, error) {
	entries, err := readDirWithin(ctx, path, readableTimeout)
	if err != nil {
		return nil, ErrUnreadable
	}
	var names []string
	for _, e := range entries {
		if neverMedia(e.Name()) {
			continue
		}
		if e.IsDir() {
			names = append(names, e.Name())
			continue
		}
		if e.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(path, e.Name())); err == nil && info.IsDir() {
				names = append(names, e.Name())
			}
		}
	}
	slices.SortFunc(names, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return names, nil
}

// errSlow is a listing that did not answer within its time.
var errSlow = errors.New("scan: the folder did not answer in time")

// readDirWithin lists path, or gives up after wait: a share that has gone away can hang a listing
// far longer, and its goroutine is left to finish on its own.
func readDirWithin(ctx context.Context, path string, wait time.Duration) ([]os.DirEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	type listed struct {
		entries []os.DirEntry
		err     error
	}
	done := make(chan listed, 1)
	go func() {
		entries, err := os.ReadDir(path) //nolint:gosec // a folder under a mount, checked by browsable
		done <- listed{entries, err}
	}()
	select {
	case l := <-done:
		return l.entries, l.err
	case <-ctx.Done():
		return nil, errSlow
	}
}

// countFolder is one look inside path: the folders the scan would walk and the videos it would read.
func countFolder(path string) FolderSummary {
	entries, err := readDirWithin(context.Background(), path, summaryTimeout)
	switch {
	case errors.Is(err, errSlow):
		return FolderSummary{Path: path, State: SummarySlow}
	case err != nil:
		return FolderSummary{Path: path, State: SummaryUnreadable}
	}
	s := FolderSummary{Path: path, State: SummaryCounted}
	for _, e := range entries {
		switch {
		case neverMedia(e.Name()):
		case e.IsDir():
			s.Folders++
		case videoExtensions[strings.ToLower(filepath.Ext(e.Name()))]:
			s.Videos++
		}
	}
	return s
}

// knownSummary is path's summary counted within summaryFresh, nil when there is none.
func (r *Roots) knownSummary(path string) *FolderSummary {
	r.sumMu.Lock()
	defer r.sumMu.Unlock()
	if s, ok := r.sums[path]; ok && r.Now().Sub(s.at) < summaryFresh {
		return &s.summary
	}
	return nil
}

// summarize counts each of paths not counted within summaryFresh, nor being counted, in the
// background, and tells Summarized each as it is counted.
func (r *Roots) summarize(paths []string) {
	r.sumMu.Lock()
	if r.sums == nil {
		r.sums, r.counting = map[string]summaryAt{}, map[string]bool{}
		r.sumSlots = make(chan struct{}, summaryWorkers)
	}
	var todo []string
	for _, p := range paths {
		if s, ok := r.sums[p]; (ok && r.Now().Sub(s.at) < summaryFresh) || r.counting[p] {
			continue
		}
		r.counting[p] = true
		todo = append(todo, p)
	}
	slots := r.sumSlots
	r.sumMu.Unlock()
	for _, p := range todo {
		go func() {
			slots <- struct{}{}
			s := countFolder(p)
			<-slots
			r.sumMu.Lock()
			r.sums[p] = summaryAt{s, r.Now()}
			delete(r.counting, p)
			r.sumMu.Unlock()
			if r.Summarized != nil {
				r.Summarized(s)
			}
		}()
	}
}
