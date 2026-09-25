package setup

import "time"

// SetRefreshBackoffForTest replaces the pause schedule between media-server refresh attempts
// (len+1 attempts) and returns a restore func. Tests that call it must not run in parallel.
func SetRefreshBackoffForTest(steps ...time.Duration) (restore func()) {
	prev := refreshBackoff
	refreshBackoff = steps
	return func() { refreshBackoff = prev }
}
