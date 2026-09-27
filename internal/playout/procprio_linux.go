package playout

import (
	"os/exec"
	"runtime"
	"syscall"
)

// startLowPriority starts cmd from an OS thread lowered to probeNice. Linux forks from the calling
// thread and a child inherits that thread's nice value, so ffmpeg and every thread it spawns run
// at background priority from their first instruction. The goroutine never unlocks the thread, so
// the lowered thread exits with it: an unprivileged thread cannot raise its priority back.
//
// Not proctree.LowPriority: that suits batch work (nice 10, set after start, graceful stop). The
// probe is killed outright the moment a viewer tunes in, so its encoder is free for the viewer at
// once, and it must never run a moment at normal priority.
func startLowPriority(cmd *exec.Cmd) error {
	started := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		// Best effort: a refused nice leaves the probe at normal priority, still yielding to viewers.
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, syscall.Gettid(), probeNice)
		started <- cmd.Start()
	}()
	return <-started
}
