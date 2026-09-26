//go:build !windows

package proctree

import (
	"os/exec"
	"syscall"
)

type processTree struct{ pgid int }

func configureProcessTree(cmd *exec.Cmd, _ options) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachProcessTree(cmd *exec.Cmd, o options) (*processTree, error) {
	if o.lowPriority {
		// PRIO_PGRP covers every thread of every process already in the group; anything spawned
		// later inherits. Best effort: an unprivileged process may always RAISE its niceness, so a
		// failure here is not expected, and running at normal priority beats not running.
		_ = syscall.Setpriority(syscall.PRIO_PGRP, cmd.Process.Pid, BackgroundNice)
	}
	return &processTree{pgid: cmd.Process.Pid}, nil
}

func (p *processTree) terminate() { _ = syscall.Kill(-p.pgid, syscall.SIGTERM) }
func (p *processTree) kill()      { _ = syscall.Kill(-p.pgid, syscall.SIGKILL) }

// close sweeps descendants after the process Wait reaps exits naturally. A crashing parent
// must not strand helpers in the server's process group.
func (p *processTree) close() { p.kill() }
