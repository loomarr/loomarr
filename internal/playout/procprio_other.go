//go:build !linux

package playout

import "os/exec"

// startLowPriority has no race-free priority control off Linux; the probe still yields to viewers.
func startLowPriority(cmd *exec.Cmd) error { return cmd.Start() }
