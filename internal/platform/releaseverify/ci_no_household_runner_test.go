package releaseverify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The loomarr-go runners belong to another repository's homelab and were never Loomarr's to use;
// with eight merge groups building, two of them left post-merge assurance hours behind. No
// workflow may route a job to them again.
func TestNoWorkflowNamesLoomarrGoRunner(t *testing.T) {
	t.Parallel()
	_, source, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(source), "..", "..", "..", ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "loomarr-go") {
			t.Errorf("%s names the loomarr-go self-hosted runner; Loomarr CI runs on GitHub-hosted runners", entry.Name())
		}
	}
}
