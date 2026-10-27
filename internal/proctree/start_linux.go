package proctree

import (
	"os/exec"
	"runtime"
	"syscall"
)

// startProcess starts cmd, at BackgroundNice from its first instruction when LowPriority asks.
func startProcess(cmd *exec.Cmd, o options) error {
	if !o.lowPriority {
		return cmd.Start()
	}
	return StartAtNice(cmd, BackgroundNice)
}

// StartAtNice starts cmd from an OS thread lowered to nice. Linux forks from the calling thread and
// a child inherits that thread's nice value, so the child and everything it spawns run at that
// priority from their first instruction. A renice after Start leaves a window in which the child
// runs, and may spawn, at normal priority (#2020). The goroutine never unlocks the thread, so the
// lowered thread exits with it: an unprivileged thread cannot raise its priority back.
func StartAtNice(cmd *exec.Cmd, nice int) error {
	started := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		// Best effort: a refused nice leaves the child at normal priority, which beats not running.
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, syscall.Gettid(), nice)
		started <- cmd.Start()
	}()
	return <-started
}
