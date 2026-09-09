//go:build !windows

package playoutcert

import (
	"golang.org/x/sys/unix"
	"os"
)

// Nonblocking open is inert for regular files and prevents a replaced FIFO from
// stalling before its descriptor can be rejected by the regular-file check.
const operatorReadFlags = os.O_RDONLY | unix.O_NONBLOCK
