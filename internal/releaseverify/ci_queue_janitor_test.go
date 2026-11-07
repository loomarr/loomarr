package releaseverify

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestQueueJanitorScript(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("bash", filepath.Join("scripts", "ci-queue-janitor-test.sh"))
	cmd.Dir = filepath.Clean(filepath.Join("..", ".."))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("queue janitor contract: %v\n%s", err, output)
	}
}

// The janitor's token can cancel runs, so what starts it and what it may do stay pinned: it runs
// from the default branch, only for queue runs, and holds nothing beyond cancelling.
func TestVerifyCIContainerDownloadsBindsQueueJanitor(t *testing.T) {
	t.Parallel()
	baseline := readRepositoryWorkflow(t, "queue-janitor.yml")
	for name, mutation := range map[string]struct{ old, new string }{
		"trigger on queued code": {old: "  workflow_run:\n    workflows: [CI]\n    types: [requested]\n", new: "  merge_group:\n"},
		"another source":         {old: "workflows: [CI]", new: "workflows: [Release]"},
		"another state":          {old: "types: [requested]", new: "types: [completed]"},
		"wider token":            {old: "  contents: read\n", new: "  contents: write\n"},
		"every run, not queue":   {old: "    if: github.event.workflow_run.event == 'merge_group'\n", new: ""},
		"another script":         {old: "run: ./scripts/ci-queue-janitor.sh", new: "run: ./scripts/ci-queue-janitor-test.sh"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(baseline, mutation.old) {
				t.Fatalf("queue-janitor.yml no longer contains %q", mutation.old)
			}
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "queue-janitor.yml"), strings.Replace(baseline, mutation.old, mutation.new, 1))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a changed queue janitor")
			}
		})
	}
}
