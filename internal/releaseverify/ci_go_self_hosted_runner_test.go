package releaseverify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGoWorkflowSelfHostedOnlyForTrustedEvents(t *testing.T) {
	t.Parallel()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(readRepositoryWorkflow(t, "ci-go.yml")), &doc); err != nil {
		t.Fatal(err)
	}
	job := workflowJobNode(t, doc.Content[0], "run")
	runsOn, ok := mappingValue(job, "runs-on")
	if !ok || runsOn.Value != goWorkflowSelfHostedRunsOn {
		t.Fatalf("ci-go.yml run job runs-on differs from its exact pinned authority, got %q", runsOn.Value)
	}
	// Belt-and-braces on the pinned string itself: `pull_request` must never appear inside the
	// expression that selects the self-hosted labels — only `merge_group`, a write-access
	// `workflow_dispatch`, and a `push` to `main` specifically may route to self-hosted. `push`
	// alone matches every branch, so the ref guard is load-bearing, not cosmetic — this assertion
	// fails if a future edit drops it and widens self-hosted to any push.
	if strings.Contains(goWorkflowSelfHostedRunsOn, "pull_request") {
		t.Fatal("ci-go.yml self-hosted runs-on expression must never mention pull_request")
	}
	if !strings.Contains(goWorkflowSelfHostedRunsOn, "github.event_name == 'push' && github.ref == 'refs/heads/main'") {
		t.Fatal("ci-go.yml self-hosted runs-on expression must restrict push to refs/heads/main, not every branch")
	}
	if !strings.Contains(goWorkflowSelfHostedRunsOn, "|| 'ubuntu-latest'") {
		t.Fatal("ci-go.yml self-hosted runs-on expression must fail safe to ubuntu-latest for every other event")
	}
}

// Only ci-go.yml may name the household loomarr-go runner; every other workflow — above all the
// `pull_request`-triggered root ci.yml — must stay on GitHub-hosted runners.
func TestOnlyGoWorkflowNamesLoomarrGoRunner(t *testing.T) {
	t.Parallel()
	_, source, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(source), "..", "..", ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "ci-go.yml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "loomarr-go") {
			t.Errorf("%s names the self-hosted loomarr-go runner; only ci-go.yml may", entry.Name())
		}
	}
}
