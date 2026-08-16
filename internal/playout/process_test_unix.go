//go:build !windows

package playout

import (
	"os"
	"syscall"
)

func writeHelperProgress(body string) {
	progress := os.NewFile(uintptr(progressFD), "progress")
	if progress == nil {
		os.Exit(6)
	}
	if _, err := progress.WriteString(body); err != nil {
		os.Exit(7)
	}
	_ = progress.Close()
}

func processRunning(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
