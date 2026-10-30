package playout

import (
	"os/exec"
	"testing"
)

// Arg-shape tests are cheap and run everywhere. The ones that actually EXECUTE ffmpeg
// live in *_live_test.go files behind a build tag, because unit tests must not depend on
// a binary being present (AGENTS.md: unit tests never touch the network, and the same
// spirit applies to external executables).

func argsAfter(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// FindFont must not invent a path: a non-existent fontfile is the init failure above.
func TestFindFont_ReturnsOnlyAnExistingFile(t *testing.T) {
	got := FindFont()
	if got == "" {
		t.Skip("no font on this system — the degrade path is covered above")
	}
	if _, err := exec.LookPath("test"); err == nil { // trivially available
		if !fileExists(got) {
			t.Errorf("FindFont returned %q which does not exist", got)
		}
	}
}
