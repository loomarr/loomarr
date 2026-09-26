//go:build !windows

package mediameasure

import (
	"os/exec"
	"syscall"
)

// backgroundNice keeps measurement behind live playout and the app on a saturated host.
const backgroundNice = 10

// lowPriority starts cmd niced: the priority is set right after the fork, before the tool spawns
// its worker threads, which inherit it. Best effort — running at normal priority beats not running.
func lowPriority(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, backgroundNice)
	return nil
}
