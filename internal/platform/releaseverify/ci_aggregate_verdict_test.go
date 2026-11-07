package releaseverify

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestRepositoryCIAggregateVerdict runs the required check's real step. A job that crosses its
// timeout reports `cancelled` with its tests unfinished, so only a pull request, where a newer
// push is authoritative, may read `cancelled` as anything but unverified.
func TestRepositoryCIAggregateVerdict(t *testing.T) {
	script := aggregateVerdictStep(t)
	for _, test := range []struct {
		name    string
		event   string
		results string
		passes  bool
	}{
		{name: "queue all green", event: "merge_group", results: "success skipped success", passes: true},
		{name: "queue failed job", event: "merge_group", results: "success failure", passes: false},
		{name: "queue timed-out lane", event: "merge_group", results: "success cancelled", passes: false},
		{name: "manual timed-out lane", event: "workflow_dispatch", results: "success cancelled", passes: false},
		{name: "pull request superseded", event: "pull_request", results: "success cancelled", passes: true},
		{name: "pull request failed job", event: "pull_request", results: "cancelled failure", passes: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := strings.NewReplacer(
				"${{ needs.changes.outputs.lane }}", "test",
				"${{ join(needs.*.result, ' ') }}", test.results,
			).Replace(script)
			if strings.Contains(run, "${{") {
				t.Fatalf("aggregate step has an expression this test does not stand in for:\n%s", run)
			}
			cmd := exec.CommandContext(t.Context(), "bash", "-e", "-c", run)
			cmd.Env = append(os.Environ(), "GITHUB_EVENT_NAME="+test.event)
			output, err := cmd.CombinedOutput()
			if passed := err == nil; passed != test.passes {
				t.Fatalf("event %s results %q: passed = %t, want %t\n%s", test.event, test.results, passed, test.passes, output)
			}
		})
	}
}

func aggregateVerdictStep(t *testing.T) string {
	t.Helper()
	root, err := parseYAML([]byte(readRepositoryWorkflow(t, "ci.yml")))
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := requiredMap(root, "jobs")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := requiredMap(jobs, "ci-ok")
	if err != nil {
		t.Fatal(err)
	}
	steps, err := requiredSequence(aggregate, "steps")
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range steps.Content {
		if scalarValue(step, "name") == "Every job that ran must have passed" {
			return scalarValue(step, "run")
		}
	}
	t.Fatal("ci-ok has no verdict step")
	return ""
}
