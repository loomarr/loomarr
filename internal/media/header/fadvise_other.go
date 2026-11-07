//go:build !linux

package header

import "os"

// adviseRandom is a no-op where posix_fadvise is not available.
func adviseRandom(*os.File) {}
