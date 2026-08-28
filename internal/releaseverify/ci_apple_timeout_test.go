package releaseverify

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRepositoryAppleNativeTimeoutContract(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "..", "..")

	for _, want := range []struct {
		workflow string
		timeout  int
	}{
		{workflow: "ci-apple-mobile.yml", timeout: 75},
		{workflow: "ci-apple-tv.yml", timeout: 60},
	} {
		t.Run(want.workflow, func(t *testing.T) {
			workflow := readWorkflowFixture(t, filepath.Join(root, ".github", "workflows", want.workflow))
			on, ok := mappingValue(workflow, "on")
			if !ok || mappingValueMust(t, on, "workflow_call").Kind == 0 {
				t.Fatalf("%s must be reusable through workflow_call", want.workflow)
			}
			jobs := mappingValueMust(t, workflow, "jobs")
			run := mappingValueMust(t, jobs, "run")
			got, ok := mappingValue(run, "timeout-minutes")
			if !ok || got == nil || got.Tag != "!!int" || got.Value != fmt.Sprint(want.timeout) {
				gotValue := "<missing>"
				if got != nil {
					gotValue = got.Value
				}
				t.Fatalf("%s run timeout-minutes = %s, want integer %d", want.workflow, gotValue, want.timeout)
			}
		})
	}

	ci := readWorkflowFixture(t, filepath.Join(root, ".github", "workflows", "ci.yml"))
	jobs := mappingValueMust(t, ci, "jobs")
	for _, name := range []string{"apple-mobile", "apple-tv"} {
		job := mappingValueMust(t, jobs, name)
		want := "github.event_name != 'pull_request' && needs.changes.outputs.impact_apple_" + map[string]string{"apple-mobile": "mobile", "apple-tv": "tv"}[name] + " == 'true'"
		if got := yamlScalar(t, job, "if"); got != want {
			t.Fatalf("CI %s admission condition = %q, want %q", name, got, want)
		}
	}
	aggregate := mappingValueMust(t, jobs, "ci-ok")
	needs := mappingValueMust(t, aggregate, "needs")
	for _, name := range []string{"apple-mobile", "apple-tv"} {
		found := false
		for _, need := range needs.Content {
			if need.Value == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("CI aggregate does not require %s", name)
		}
	}
}

func readWorkflowFixture(t *testing.T, path string) *yaml.Node {
	t.Helper()
	data := readFixtureFile(t, path)
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(data), &document); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return document.Content[0]
}

func mappingValueMust(t *testing.T, scope *yaml.Node, key string) *yaml.Node {
	t.Helper()
	value, ok := mappingValue(scope, key)
	if !ok {
		t.Fatalf("missing YAML key %s", key)
	}
	return value
}

func yamlScalar(t *testing.T, scope *yaml.Node, key string) string {
	t.Helper()
	value := mappingValueMust(t, scope, key)
	if value.Kind != yaml.ScalarNode {
		t.Fatalf("YAML key %s is not scalar", key)
	}
	return value.Value
}
