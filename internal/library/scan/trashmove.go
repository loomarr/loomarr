package scan

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// A restore moves a file without ever replacing one: another program can write its old place at any
// moment, so the move itself refuses a taken place (fs.ErrExist) and no earlier look decides it.

// moveNoReplace moves src to dst, leaving the file at exactly one of them. A hard link then removes
// the old name; where there are no hard links, and for anything but a regular file (macOS links a
// symlink's target, not the link), the OS's no-replace rename does it, and where there is neither,
// nothing moves. link and remove are os.Link and os.Remove.
func moveNoReplace(src, dst string, link func(string, string) error, remove func(string) error) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return renameNoReplace(src, dst)
	}
	switch err := link(src, dst); {
	case err == nil:
	case errors.Is(err, syscall.EPERM), errors.Is(err, syscall.EXDEV), errors.Is(err, errors.ErrUnsupported):
		return renameNoReplace(src, dst)
	default:
		return err
	}
	err = remove(src)
	// A source gone by now leaves dst as the only name, so dst stays.
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return errors.Join(err, remove(dst))
}

// resolveUnder is dir with its symlinks resolved, refused unless that is still under root, resolved
// the same way. The part of dir that is not there yet is kept as named, so it can be asked before
// anything is made.
func resolveUnder(root, dir string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	missing := ""
	for at := dir; ; at = filepath.Dir(at) {
		resolved, err := filepath.EvalSymlinks(at)
		if errors.Is(err, fs.ErrNotExist) && filepath.Dir(at) != at {
			missing = filepath.Join(filepath.Base(at), missing)
			continue
		}
		if err != nil {
			return "", err
		}
		resolved = filepath.Join(resolved, missing)
		if rel, err := filepath.Rel(realRoot, resolved); err != nil || !filepath.IsLocal(rel) {
			return "", fmt.Errorf("%s leads outside %s", dir, root)
		}
		return resolved, nil
	}
}
