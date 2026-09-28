package playout

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// PreparedReclaim reports one pass over the retired prepared-media library (#1563).
type PreparedReclaim struct {
	Entries    int   // publications and staging workspaces removed
	Bytes      int64 // bytes of regular files they held
	DirRemoved bool  // the library directory was left empty and removed
}

// ReclaimRetiredPrepared removes the media the beta.7 prepared library left on disk (decision
// 0040: nothing reads it since beta.8). It removes only what that library wrote: a directory named
// by a publication key (64 lowercase hex) that holds .publication.json, and a .staging-<key>-*
// workspace. Symlinks, unrecognised files and directories, and capability evidence the state
// directory move could not take stay, and so does the directory unless this pass emptied it. A
// missing directory is a no-op, so every boot after the first finds nothing. Entries that fail to
// remove are reported in the error; the rest are still removed.
func ReclaimRetiredPrepared(dir string) (PreparedReclaim, error) {
	var res PreparedReclaim
	if strings.TrimSpace(dir) == "" {
		return res, nil
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return res, nil
	} else if err != nil {
		return res, err
	}
	var errs []error
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if !e.IsDir() || !isPreparedArtefact(path, e.Name()) {
			continue
		}
		size := regularBytes(path)
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, err)
			continue
		}
		res.Entries++
		res.Bytes += size
	}
	if res.Entries > 0 {
		if rest, err := os.ReadDir(dir); err == nil && len(rest) == 0 {
			res.DirRemoved = os.Remove(dir) == nil
		}
	}
	return res, errors.Join(errs...)
}

func isPreparedArtefact(path, name string) bool {
	if rest, ok := strings.CutPrefix(name, ".staging-"); ok {
		key, _, found := strings.Cut(rest, "-")
		return found && isPublicationKey(key)
	}
	if !isPublicationKey(name) {
		return false
	}
	info, err := os.Lstat(filepath.Join(path, ".publication.json"))
	return err == nil && info.Mode().IsRegular()
}

// isPublicationKey matches the prepared library's hex-encoded SHA-256 publication key.
func isPublicationKey(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func regularBytes(root string) int64 {
	var n int64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, ierr := d.Info(); ierr == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}
