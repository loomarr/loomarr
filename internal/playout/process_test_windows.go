//go:build windows

package playout

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func writeHelperProgress(body string) {
	if _, err := fmt.Fprint(os.Stderr, body); err != nil {
		os.Exit(7)
	}
}

func processRunning(pid int) bool {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(process) }()
	status, err := windows.WaitForSingleObject(process, 0)
	return err == nil && status == uint32(windows.WAIT_TIMEOUT)
}
