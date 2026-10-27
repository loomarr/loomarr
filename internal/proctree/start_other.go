//go:build !linux

package proctree

import "os/exec"

// startProcess starts cmd. Off Linux a child does not take its nice from the forking thread, so
// LowPriority is applied as the tree is attached: renicing the process group on Unix, the
// creation priority class on Windows.
func startProcess(cmd *exec.Cmd, _ options) error { return cmd.Start() }

// StartAtNice has no race-free priority control off Linux, and starts cmd at normal priority.
func StartAtNice(cmd *exec.Cmd, _ int) error { return cmd.Start() }
