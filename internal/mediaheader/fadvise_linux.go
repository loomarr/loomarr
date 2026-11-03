//go:build linux

package mediaheader

import (
	"os"

	"golang.org/x/sys/unix"
)

// adviseRandom turns off kernel readahead for f: header reads are a few small reads at known
// offsets, and on a NAS each extra readahead page is more disk time. Advice is best effort.
func adviseRandom(f *os.File) {
	_ = unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_RANDOM)
}
