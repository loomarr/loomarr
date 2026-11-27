package scan

import (
	"os"

	"golang.org/x/sys/unix"
)

// renameNoReplace is a rename the kernel refuses when dst exists. A filesystem that can't promise
// that refuses the call.
func renameNoReplace(src, dst string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, src, unix.AT_FDCWD, dst, unix.RENAME_NOREPLACE); err != nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: err}
	}
	return nil
}
