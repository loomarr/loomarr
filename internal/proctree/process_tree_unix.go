//go:build !windows

package proctree

import (
	"os/exec"
	"syscall"
)

type processTree struct{ pgid int }

func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachProcessTree(cmd *exec.Cmd) (*processTree, error) {
	return &processTree{pgid: cmd.Process.Pid}, nil
}

func (p *processTree) terminate() { _ = syscall.Kill(-p.pgid, syscall.SIGTERM) }
func (p *processTree) kill()      { _ = syscall.Kill(-p.pgid, syscall.SIGKILL) }

// close sweeps descendants after the process Wait reaps exits naturally. A crashing parent
// must not strand helpers in the server's process group.
func (p *processTree) close() { p.kill() }
