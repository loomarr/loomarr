package scan

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/unix"
)

// Watch defaults (decision 9).
const (
	// watchDebounce is how long a root must be quiet before its changed folders are passed over.
	// A file being copied in keeps it busy, so a pass never reads half a file.
	watchDebounce = 5 * time.Second
	// watchResync is how often the watched roots are compared with the in-use ones.
	watchResync = time.Minute
)

// Watcher passes over the folders a change names, a few seconds after it lands, for each in-use
// root on a local filesystem. inotify sees only changes made through this machine's kernel, so a
// network share (CIFS, NFS, FUSE) is left to the hourly walk and the arrs' webhooks. A kernel
// queue overflow asks for a full pass instead.
type Watcher struct {
	Scanner *Scanner
	// Debounce and Resync are zero for their defaults.
	Debounce time.Duration
	Resync   time.Duration
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

// roots is the walked roots on a local filesystem, sorted.
func (w *Watcher) roots(ctx context.Context) ([]string, error) {
	all, err := w.Scanner.Importer.Store.LibraryRoots(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for root := range scannedRoots(all.Roots) {
		if localFilesystem(root) {
			out = append(out, root)
		}
	}
	slices.Sort(out)
	return out, nil
}

// watch watches roots until the in-use set changes (nil) or watching fails.
func (w *Watcher) watch(ctx context.Context, roots []string) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer func() { _ = fsw.Close() }()
	for _, root := range roots {
		w.addTree(fsw, root, root)
	}

	debounce := orDuration(w.Debounce, watchDebounce)
	quiet := time.NewTimer(debounce)
	quiet.Stop()
	resync := time.NewTicker(orDuration(w.Resync, watchResync))
	defer resync.Stop()
	changed := map[string]map[string]bool{}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-fsw.Events:
			if !ok {
				return errors.New("library scan: the watch closed")
			}
			root := rootOf(ev.Name, roots)
			if root == "" {
				continue
			}
			if ev.Has(fsnotify.Create) {
				// A folder created or moved in is watched from now on, and walked by the pass.
				if info, err := os.Lstat(ev.Name); err == nil && info.IsDir() {
					w.addTree(fsw, root, ev.Name)
				}
			}
			if changed[root] == nil {
				changed[root] = map[string]bool{}
			}
			changed[root][filepath.Dir(ev.Name)] = true
			quiet.Reset(debounce)
		case err, ok := <-fsw.Errors:
			if !ok {
				return errors.New("library scan: the watch closed")
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				w.log().Warn("library scan: too many changes at once; walking every folder")
				clear(changed)
				w.Scanner.Trigger()
				continue
			}
			return err
		case <-quiet.C:
			for root, dirs := range changed {
				if err := w.Scanner.Importer.RunFolders(ctx, root, slices.Sorted(maps.Keys(dirs))); err != nil && ctx.Err() == nil {
					w.log().Warn("library scan: a folder pass failed", "root", root, "err", err)
				}
			}
			clear(changed)
		case <-resync.C:
			now, err := w.roots(ctx)
			if err != nil || !slices.Equal(now, roots) {
				return err
			}
		}
	}
}

// addTree watches dir and every folder under it a walk would enter. A folder the kernel will not
// watch (the inotify limit) is logged once per tree; its changes wait for the hourly walk.
func (w *Watcher) addTree(fsw *fsnotify.Watcher, root, dir string) {
	walker := walker{root: root}
	if skip, _ := walker.within(dir); skip {
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
			lower := strings.ToLower(e.Name())
			if !e.IsDir() || skippedDirs[lower] || strings.HasPrefix(lower, ".trash-") || discDirs[lower] {
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
