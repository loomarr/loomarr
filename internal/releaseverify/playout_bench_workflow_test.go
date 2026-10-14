package releaseverify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The repository is public. A self-hosted runner on the maintainer's GPU machines must never be
// reachable from a pull request, so the hardware bench workflow may only be dispatched or scheduled,
// and only it may name a self-hosted playout runner.
func TestPlayoutBenchWorkflowNeverRunsOnPullRequest(t *testing.T) {
	t.Parallel()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(readRepositoryWorkflow(t, "playout-bench.yml")), &doc); err != nil {
		t.Fatal(err)
	}
	root := doc.Content[0]
	triggers, ok := mappingValue(root, "on")
	if !ok || triggers.Kind != yaml.MappingNode {
		t.Fatal("playout-bench.yml must declare its triggers")
	}
	for i := 0; i < len(triggers.Content); i += 2 {
		switch name := triggers.Content[i].Value; name {
		case "workflow_dispatch", "schedule":
		default:
			t.Errorf("playout-bench.yml trigger %q is not allowed: only workflow_dispatch and schedule", name)
		}
	}
	jobs, _ := mappingValue(root, "jobs")
	for i := 0; i < len(jobs.Content); i += 2 {
		name, job := jobs.Content[i].Value, jobs.Content[i+1]
		runsOn, _ := mappingValue(job, "runs-on")
		if runsOn == nil || !strings.HasPrefix(runsOn.Value, "loomarr-playout-") {
			continue
		}
		condition := scalarValue(job, "if")
		if !strings.Contains(condition, "github.event_name == 'workflow_dispatch'") {
			t.Errorf("self-hosted job %s must check for workflow_dispatch itself, has %q", name, condition)
		}
		if strings.Contains(condition, "schedule") {
			t.Errorf("self-hosted job %s must not run on a schedule: %q", name, condition)
		}
	}
}

func TestOnlyPlayoutBenchWorkflowUsesSelfHostedPlayoutRunners(t *testing.T) {
	t.Parallel()
	_, source, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(source), "..", "..", ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "playout-bench.yml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "loomarr-playout-") {
			t.Errorf("%s names a self-hosted playout runner; only playout-bench.yml may", entry.Name())
		}
	}
}
