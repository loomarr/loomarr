// Package execfixture owns filesystem-backed executable test doubles without
// importing application packages. It is the cycle-free leaf beneath testkit.
package execfixture

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Executable writes one executable test double. The write holds syscall.ForkLock, which every
// fork in this process takes exclusively: a child forked while the file is open for writing would
// inherit that descriptor until it execs, and running the double then fails with ETXTBSY ("text
// file busy"). Renaming a finished file would not help: the busy state belongs to the inode.
func Executable(t testing.TB, name, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	syscall.ForkLock.RLock()
	err := os.WriteFile(path, []byte(script), 0o700)
	syscall.ForkLock.RUnlock()
	if err != nil {
		t.Fatalf("write executable %s: %v", name, err)
	}
	return path
}
