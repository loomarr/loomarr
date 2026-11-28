//go:build !linux

package admission

import "time"

// ProcessCPUTime has no portable live source off Linux; measurements fall back to the process's
// total CPU at exit (costFromSamples).
func ProcessCPUTime(int) (time.Duration, bool) { return 0, false }
