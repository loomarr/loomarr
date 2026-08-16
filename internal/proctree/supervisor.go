package proctree

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// stopGrace is how long a process tree gets to exit after a graceful stop before the
// supervisor terminates it. This preserves the bounded shutdown contract of the callers
// this module replaced: a stuck child must not retain capacity its replacement needs.
const stopGrace = 2 * time.Second

// Supervisor owns one command and its complete descendant tree.
type Supervisor struct {
	cmd  *exec.Cmd
	tree *processTree

	stopping atomic.Bool
	stopOnce sync.Once
	waitOnce sync.Once
	waitErr  error
	exited   chan struct{}
}

// Start configures cmd for platform tree ownership, starts it, attaches its process tree,
// and binds the tree lifetime to ctx. Callers must configure stdio pipes before calling Start.
func Start(ctx context.Context, cmd *exec.Cmd) (*Supervisor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	configureProcessTree(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	tree, err := attachProcessTree(cmd)
	if err != nil {
		// On Windows the process is still suspended if attachment failed, so parent-only
		// termination is sufficient. If the Job Object was already attached, its failed-path
		// cleanup terminates the whole tree before returning.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("attach process tree: %w", err)
	}

	s := &Supervisor{cmd: cmd, tree: tree, exited: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			s.Stop()
		case <-s.exited:
		}
	}()
	return s, nil
}

// Stop requests a graceful tree-wide stop, escalates after the fixed grace period, and
// returns only after the command has been reaped. Concurrent calls all wait for the same stop.
func (s *Supervisor) Stop() {
	s.stopOnce.Do(func() {
		s.stopping.Store(true)
		s.tree.terminate()

		done := make(chan struct{})
		go func() {
			_ = s.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(stopGrace):
			s.tree.kill()
			<-done
		}
	})
}

// Wait reaps the command once. A non-zero exit caused by Stop is suppressed; a natural
// non-zero exit remains visible. Descendants are swept even when the parent exits naturally.
func (s *Supervisor) Wait() error {
	s.waitOnce.Do(func() {
		err := s.cmd.Wait()
		s.tree.close()
		if err != nil && s.stopping.Load() {
			err = nil
		}
		s.waitErr = err
		close(s.exited)
	})
	return s.waitErr
}

// Stopped reports whether teardown was requested through Stop, including context cancellation.
func (s *Supervisor) Stopped() bool { return s.stopping.Load() }
