//go:build !linux

package playout

import "time"

// processCPUTime has no portable live source off Linux; measurements fall back to the process's
// total CPU at exit (costFromSamples).
func processCPUTime(int) (time.Duration, bool) { return 0, false }
