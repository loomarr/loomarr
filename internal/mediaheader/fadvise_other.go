//go:build !linux

package mediaheader

import "os"

// adviseRandom is a no-op where posix_fadvise is not available.
func adviseRandom(*os.File) {}
