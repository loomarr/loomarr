package storagegovernor

import (
	"io/fs"
	"os"
)

// SetWalkStatHooks replaces the stat seams used while walking a private tree and returns a restore.
func SetWalkStatHooks(
	info func(fs.DirEntry) (fs.FileInfo, error), lstat func(string) (fs.FileInfo, error),
) (restore func()) {
	oldInfo, oldLstat := entryInfo, lstatEntry
	if info != nil {
		entryInfo = info
	}
	if lstat != nil {
		lstatEntry = lstat
	}
	return func() { entryInfo, lstatEntry = oldInfo, oldLstat }
}

var _ = os.Lstat
