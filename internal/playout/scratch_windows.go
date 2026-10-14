//go:build windows

package playout

// Without flock, ownership cannot be proved, so nothing is swept: a leaked root costs disk, a
// wrongly swept one breaks a live process's playback.

func holdScratch(string) (func(), error) { return func() {}, nil }

func scratchAbandoned(string) bool { return false }
