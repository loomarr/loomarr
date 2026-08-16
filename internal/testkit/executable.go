package testkit

import (
	"os"
	"path/filepath"
	"testing"
)

// Executable writes a shared system-boundary test double for code that invokes a required local
// tool. Keep these doubles in testkit rather than growing one private shell mock per caller.
func Executable(t *testing.T, name, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write executable %s: %v", name, err)
	}
	return path
}
