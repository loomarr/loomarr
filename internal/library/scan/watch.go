package scan

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/unix"
)

// watchResync is how often the watched roots are compared with the in-use ones.
const watchResync = time.Minute

// Watcher hands the scanner the folders a change names (Scanner.PassFolders), for each in-use root:
// through inotify on a local filesystem, and through the server's change notifications on an SMB
// share, which report changes any client makes. inotify sees only this machine's own changes, so
// another network or userspace filesystem (NFS, FUSE) is left to the hourly walk and the arrs'
// webhooks. A kernel queue overflow asks for a full pass instead.
type Watcher struct {
	Scanner *Scanner
	// Resync and ShareRetry are zero for their defaults.
	Resync     time.Duration
	ShareRetry time.Duration
}

// watchedRoots is the in-use roots by how their changes are heard.
type watchedRoots struct {
	local, shares []string
}

func (r watchedRoots) equal(o watchedRoots) bool {
	return slices.Equal(r.local, o.local) && slices.Equal(r.shares, o.shares)
}

// Run watches until ctx ends.
func (w *Watcher) Run(ctx context.Context) {
	resync := orDuration(w.Resync, watchResync)
	for ctx.Err() == nil {
		roots, err := w.roots(ctx)
		if err == nil {
			err = w.watch(ctx, roots)
		}
		if err != nil && ctx.Err() == nil {
			w.log().Warn("library scan: watching stopped; the next walk catches up", "err", err)
			w.Scanner.Trigger()
			t := time.NewTimer(resync)
			select {
			case <-ctx.Done():
				t.Stop()
			case <-t.C:
			}
		}
	}
}

// roots is the walked roots Loomarr can watch, sorted.
func (w *Watcher) roots(ctx context.Context) (watchedRoots, error) {
	all, err := w.Scanner.Importer.Store.LibraryRoots(ctx)
	if err != nil {
		return watchedRoots{}, err
	}
	var out watchedRoots
	for root := range scannedRoots(all.Roots) {
		switch {
		case localFilesystem(root):
			out.local = append(out.local, root)
		case cifsFilesystem(root):
			out.shares = append(out.shares, root)
		}
	}
	slices.Sort(out.local)
	slices.Sort(out.shares)
	return out, nil
}

// watch watches roots until the in-use set changes (nil) or watching fails.
func (w *Watcher) watch(ctx context.Context, watched watchedRoots) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer func() { _ = fsw.Close() }()
	roots := watched.local
	for _, root := range roots {
		w.addTree(fsw, root, root)
	}

	shareCtx, stopShares := context.WithCancel(ctx)
	defer stopShares()
	shareChanges := make(chan shareChange)
	for _, root := range watched.shares {
		go watchShare(shareCtx, root, orDuration(w.ShareRetry, shareRetry), shareChanges, func(err error, permanent bool) {
			w.shareFailed(root, err, permanent)
		})
	}

	resync := time.NewTicker(orDuration(w.Resync, watchResync))
	defer resync.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case change := <-shareChanges:
			w.Scanner.PassFolders(change.root, change.dirs...)
		case ev, ok := <-fsw.Events:
			if !ok {
				return errors.New("library scan: the watch closed")
			}
			root := rootOf(ev.Name, roots)
			if root == "" || ev.Op == fsnotify.Chmod || ignoredChange(ev.Name) {
				continue
			}
			if ev.Has(fsnotify.Create) {
				// A folder created or moved in is watched from now on, and walked by the pass.
				if info, err := os.Lstat(ev.Name); err == nil && info.IsDir() {
					w.addTree(fsw, root, ev.Name)
				}
			}
			w.Scanner.PassFolders(root, filepath.Dir(ev.Name))
		case err, ok := <-fsw.Errors:
			if !ok {
				return errors.New("library scan: the watch closed")
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				w.log().Warn("library scan: too many changes at once; walking every folder")
				w.Scanner.Trigger()
				continue
			}
			return err
		case <-resync.C:
			now, err := w.roots(ctx)
			if err != nil || !now.equal(watched) {
				return err
			}
		}
	}
}

// ignoredSuffixes are files still being written by a download or copy, and ignoredNames files a
// desktop leaves behind: neither is media, and each change to one would restart its folder's wait.
// The finished file's own name arrives as a change of its own.
var (
	ignoredSuffixes = []string{".part", ".partial", ".tmp", ".temp", ".crdownload", ".download", ".!qb", ".!ut", ".swp", "~"}
	ignoredNames    = map[string]bool{".ds_store": true, "thumbs.db": true, "desktop.ini": true}
)

// ignoredChange reports whether a change to name can be dropped before it reaches the folder queue.
func ignoredChange(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	if ignoredNames[base] || strings.HasPrefix(base, "._") {
		return true
	}
	for _, suffix := range ignoredSuffixes {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}

// shareFailed logs a share whose watch stopped. One that cannot report changes is left to the
// hourly walk; any other failure may have missed changes, so the whole root is passed once the
// watch is back.
func (w *Watcher) shareFailed(root string, err error, permanent bool) {
	if permanent {
		w.log().Warn("library scan: this share can't report changes; they wait for the hourly walk",
			"root", root, "err", err)
		return
	}
	w.log().Warn("library scan: watching this share stopped; trying again shortly", "root", root, "err", err)
	w.Scanner.PassFolders(root, root)
}

// addTree watches dir and every folder under it a walk would enter. A folder the kernel will not
// watch (the inotify limit) is logged once per tree; its changes wait for the hourly walk.
func (w *Watcher) addTree(fsw *fsnotify.Watcher, root, dir string) {
	walker := walker{root: root}
	if walker.within(dir) {
		return
	}
	var failed error
	var add func(string)
	add = func(d string) {
		if err := fsw.Add(d); err != nil {
			failed = err
			return
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.Name() == ".ignore" {
				_ = fsw.Remove(d)
				return
			}
		}
		for _, e := range entries {
			if !e.IsDir() || skippedDir(e.Name(), d != root) || discDirs[strings.ToLower(e.Name())] {
				continue
			}
			add(filepath.Join(d, e.Name()))
		}
	}
	add(dir)
	if failed != nil {
		w.log().Warn("library scan: some folders are not watched; their changes wait for the hourly walk",
			"root", root, "err", failed)
	}
}

func (w *Watcher) log() *slog.Logger { return w.Scanner.Importer.log() }

// rootOf is the root path is under, "" for none.
func rootOf(path string, roots []string) string {
	for _, root := range roots {
		if under(path, []string{root}) {
			return root
		}
	}
	return ""
}

// Network and userspace filesystems, on which inotify misses changes made elsewhere.
var remoteFilesystems = map[int64]bool{
	unix.CIFS_SUPER_MAGIC: true,
	unix.SMB2_SUPER_MAGIC: true,
	unix.SMB_SUPER_MAGIC:  true,
	unix.NFS_SUPER_MAGIC:  true,
	unix.FUSE_SUPER_MAGIC: true,
	unix.V9FS_MAGIC:       true,
	unix.AFS_SUPER_MAGIC:  true,
	unix.CEPH_SUPER_MAGIC: true,
}

// localFilesystem reports whether inotify sees every change under path.
func localFilesystem(path string) bool {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return false
	}
	return !remoteFilesystems[int64(fs.Type)]
}

func orDuration(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}
