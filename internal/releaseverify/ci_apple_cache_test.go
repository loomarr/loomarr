package releaseverify

import (
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRepositoryAppleCacheValidationIsIsolatedAndPortable(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve repository test path")
	}
	root := filepath.Join(filepath.Dir(source), "..", "..")
	ci := readWorkflowFixture(t, filepath.Join(root, ".github", "workflows", "ci.yml"))
	on := mappingValueMust(t, ci, "on")
	dispatch := mappingValueMust(t, on, "workflow_dispatch")
	inputs := mappingValueMust(t, dispatch, "inputs")
	scope := mappingValueMust(t, inputs, "scope")
	options := mappingValueMust(t, scope, "options")
	if options.Kind != yaml.SequenceNode || !yamlSequenceContains(options, "apple-cache-validation") {
		t.Fatal("manual CI does not expose the Apple cache validation scope")
	}

	jobs := mappingValueMust(t, ci, "jobs")
	validation := mappingValueMust(t, jobs, "apple-cache-validation")
	if got := yamlScalar(t, validation, "if"); got != "github.event_name == 'workflow_dispatch' && inputs.scope == 'apple-cache-validation'" {
		t.Fatalf("Apple cache validation admission = %q", got)
	}
	if got := yamlScalar(t, validation, "uses"); got != "./.github/workflows/ci-apple-cache-validation.yml" {
		t.Fatalf("Apple cache validation reusable workflow = %q", got)
	}
	aggregate := mappingValueMust(t, jobs, "ci-ok")
	if !yamlSequenceContains(mappingValueMust(t, aggregate, "needs"), "apple-cache-validation") {
		t.Fatal("CI aggregate does not require Apple cache validation")
	}

	workflow := readWorkflowFixture(t, filepath.Join(root, ".github", "workflows", "ci-apple-cache-validation.yml"))
	validationJobs := mappingValueMust(t, workflow, "jobs")
	producer := mappingValueMust(t, validationJobs, "producer")
	consumer := mappingValueMust(t, validationJobs, "consumer")
	if yamlScalar(t, producer, "runs-on") != "macos-26" || yamlScalar(t, consumer, "runs-on") != "macos-26" {
		t.Fatal("Apple cache portability must use two supported macos-26 jobs")
	}
	if !yamlSequenceContains(mappingValueMust(t, consumer, "needs"), "producer") {
		t.Fatal("Apple cache consumer does not depend on the producer runner")
	}
	producerSteps := mappingValueMust(t, producer, "steps").Content
	consumerSteps := mappingValueMust(t, consumer, "steps").Content
	workflowRunStepIndex(t, producerSteps, "./web/scripts/validate-apple-compilation-cache.sh produce \"$RUNNER_TEMP/apple-compilation-cache.tar.zst\"")
	workflowRunStepIndex(t, consumerSteps, "./web/scripts/validate-apple-compilation-cache.sh consume \"$RUNNER_TEMP/apple-compilation-cache.tar.zst\"")
	workflowUsesStepIndex(t, producerSteps, "actions/upload-artifact")
	workflowUsesStepIndex(t, consumerSteps, "actions/download-artifact")
}

func yamlSequenceContains(sequence *yaml.Node, want string) bool {
	if sequence.Kind != yaml.SequenceNode {
		return false
	}
	for _, item := range sequence.Content {
		if item.Value == want {
			return true
		}
	}
	return false
}
