//go:build !windows

package fetch

import (
	"errors"
	"syscall"
)

func testProcessGone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func killTestProcess(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
