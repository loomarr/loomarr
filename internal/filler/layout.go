package filler

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Layout is the immutable filesystem topology for one application generation.
//
// Catalog paths are relative to ClipDir, while WatchDir drains into ClipDir. Keeping the pair in
// one value prevents a settings refresh from making a running operation interpret a catalog row
// under one root and write its arriving files under another.
type Layout struct {
	configuredClipDir string
	clipDir           string
	watchDir          string
}

// NewLayout resolves the configured clip and watch folders for one application generation.
//
// Paths are absolute and cleaned so every consumer compares and logs the same spelling. A fully
// empty pair disables filler storage. A watch folder without a clip folder is invalid because
// intake would have nowhere to file arrivals. Filesystem roots are rejected as a safety boundary:
// a typo must not turn a scan or intake pass into a walk of the whole mounted filesystem.
func NewLayout(root, watch string) (Layout, error) {
	root = strings.TrimSpace(root)
	watch = strings.TrimSpace(watch)
	if root == "" {
		if watch == "" {
			return Layout{}, nil
		}
		return Layout{}, fmt.Errorf("filler layout: watch folder requires a clip folder")
	}

	clipDir, err := layoutPath("clip", root)
	if err != nil {
		return Layout{}, err
	}
	if watch == "" {
		watch = filepath.Join(clipDir, WatchDirName)
	}
	watchDir, err := layoutPath("watch", watch)
	if err != nil {
		return Layout{}, err
	}
	if err := existingDirectory("clip", clipDir); err != nil {
		return Layout{}, err
	}
	if err := existingDirectory("watch", watchDir); err != nil {
		return Layout{}, err
	}
	if pathContains(watchDir, clipDir) {
		return Layout{}, fmt.Errorf("filler layout: watch folder %q must not contain clip folder %q", watchDir, clipDir)
	}
	// Filesystem aliases can spell the same unsafe topology with unrelated path strings: symlinks,
	// Linux bind mounts, and case-insensitive macOS names. Compare the longest existing watch
	// prefix to every existing clip ancestor, carrying each not-yet-created suffix into the check.
	contains, err := sameFileContains(watchDir, clipDir)
	if err != nil {
		return Layout{}, err
	}
	if contains {
		return Layout{}, fmt.Errorf("filler layout: watch folder %q refers to a parent of clip folder %q", watchDir, clipDir)
	}
	clipTraversal, err := traversalPath(clipDir)
	if err != nil {
		return Layout{}, err
	}
	watchTraversal, err := traversalPath(watchDir)
	if err != nil {
		return Layout{}, err
	}
	// When two bind-mount or case aliases name a watch subtree beneath the clip root, normalize
	// the watch to the clip traversal spelling. WalkDir sees paths through the root it was given;
	// without this, scan/intake could not exclude the same directory reached through another name.
	if rel, inside, relErr := identityRelative(clipDir, watchDir); relErr != nil {
		return Layout{}, relErr
	} else if inside {
		watchTraversal = filepath.Join(append([]string{clipTraversal}, rel...)...)
	}
	return Layout{
		configuredClipDir: clipDir,
		clipDir:           clipTraversal,
		watchDir:          watchTraversal,
	}, nil
}

func layoutPath(kind, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("filler layout: %s folder must be absolute: %q", kind, path)
	}
	clean := filepath.Clean(path)
	if parent := filepath.Dir(clean); parent == clean {
		return "", fmt.Errorf("filler layout: %s folder must not be filesystem root %q", kind, clean)
	}
	return clean, nil
}

func existingDirectory(kind, path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("filler layout: %s folder %q is not a directory", kind, path)
		}
		return nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("filler layout: stat %s folder %q: %w", kind, path, err)
}

// pathContains reports whether child is parent itself or lies beneath it. A watch folder with
// that relationship to the clip root would walk already-filed clips as arrivals; duplicate
// handling would then remove each file because its computed destination is itself.
func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// sameFileContains catches aliases that pathname comparison cannot reveal, notably Linux bind
// mounts, symlinks, and case-insensitive macOS spellings. Neither final directory must exist.
// The watch path's longest existing prefix is compared to every existing clip ancestor; their
// unresolved suffixes are then compared component-by-component. EqualFold is deliberately used
// even on Linux so a layout saved there remains safe if the same database is moved to macOS.
func sameFileContains(watchDir, clipDir string) (bool, error) {
	_, contains, err := identityRelative(watchDir, clipDir)
	return contains, err
}

// identityRelative reports child's relative component path beneath parent when filesystem
// identity proves the relationship. It works before either leaf exists and across symlink,
// bind-mount, and case aliases by comparing the parent's longest existing prefix to each existing
// child ancestor, then matching the unresolved suffixes.
func identityRelative(parentPath, childPath string) ([]string, bool, error) {
	_, parentInfo, parentSuffix, err := existingPrefix(parentPath)
	if err != nil {
		return nil, false, err
	}
	current := filepath.Clean(childPath)
	var childSuffix []string
	for {
		info, statErr := os.Stat(current)
		switch {
		case statErr == nil && os.SameFile(parentInfo, info) && componentPrefix(parentSuffix, childSuffix):
			return append([]string(nil), childSuffix[len(parentSuffix):]...), true, nil
		case statErr != nil && !errors.Is(statErr, fs.ErrNotExist):
			return nil, false, fmt.Errorf("filler layout: stat folder ancestor %q: %w", current, statErr)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil, false, nil
		}
		childSuffix = append([]string{filepath.Base(current)}, childSuffix...)
		current = parent
	}
}

func existingPrefix(path string) (string, fs.FileInfo, []string, error) {
	current := filepath.Clean(path)
	var suffix []string
	for {
		info, err := os.Stat(current)
		if err == nil {
			return current, info, suffix, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", nil, nil, fmt.Errorf("filler layout: stat folder ancestor %q: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", nil, nil, fmt.Errorf("filler layout: no existing parent for %q", path)
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
	}
}

func traversalPath(path string) (string, error) {
	prefix, _, suffix, err := existingPrefix(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(prefix)
	if err != nil {
		return "", fmt.Errorf("filler layout: resolve folder prefix %q: %w", prefix, err)
	}
	return filepath.Clean(filepath.Join(append([]string{resolved}, suffix...)...)), nil
}

func componentPrefix(parent, child []string) bool {
	if len(parent) > len(child) {
		return false
	}
	for i := range parent {
		if !strings.EqualFold(parent[i], child[i]) {
			return false
		}
	}
	return true
}

// intakeSource resolves a registered folder for local traversal and rejects every overlap with
// the catalog root. Overlap in either direction is unsafe: an ancestor walks the whole catalog,
// while a descendant can name an already-filed shard. In both cases TakeIn's duplicate cleanup
// can remove the source path, which is the live catalog file when the paths are aliases.
func (l Layout) intakeSource(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("filler layout: source folder is empty")
	}
	clean, err := layoutPath("source", path)
	if err != nil {
		return "", err
	}
	if err := existingDirectory("source", clean); err != nil {
		return "", err
	}
	for _, clip := range []string{l.configuredClipDir, l.clipDir} {
		if clip != "" && (pathContains(clean, clip) || pathContains(clip, clean)) {
			return "", fmt.Errorf("filler layout: source folder %q overlaps clip folder %q", clean, clip)
		}
	}
	for _, pair := range [][2]string{{clean, l.clipDir}, {l.clipDir, clean}} {
		if pair[0] == "" || pair[1] == "" {
			continue
		}
		_, overlaps, relErr := identityRelative(pair[0], pair[1])
		if relErr != nil {
			return "", relErr
		}
		if overlaps {
			return "", fmt.Errorf("filler layout: source folder %q refers to clip folder %q or one of its parents", clean, l.configuredClipDir)
		}
	}
	resolved, err := traversalPath(clean)
	if err != nil {
		return "", err
	}
	if l.clipDir != "" && (pathContains(resolved, l.clipDir) || pathContains(l.clipDir, resolved)) {
		return "", fmt.Errorf("filler layout: resolved source folder %q overlaps clip folder %q", resolved, l.clipDir)
	}
	return resolved, nil
}

// ConfiguredClipDir is the operator-provided shared-volume spelling registered with Tunarr.
// Loomarr may traverse the same directory through a resolved symlink spelling, but an external
// process can only use the mount namespace path it was configured to share.
func (l Layout) ConfiguredClipDir() string { return l.configuredClipDir }

// ClipDir is the absolute root against which stored clip paths are interpreted.
func (l Layout) ClipDir() string { return l.clipDir }

// WatchDir is the absolute arrival folder drained into ClipDir.
func (l Layout) WatchDir() string { return l.watchDir }

// FS exposes the clip library as an fs.FS for consumers of relative catalog paths.
// A zero layout returns nil rather than inventing a process-relative root.
func (l Layout) FS() fs.FS {
	if l.clipDir == "" {
		return nil
	}
	return os.DirFS(l.clipDir)
}
