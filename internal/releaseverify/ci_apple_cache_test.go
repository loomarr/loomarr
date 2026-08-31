package releaseverify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestRepositoryAppleCachePublisherAndConsumersAreTrustBounded(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve repository test path")
	}
	root := filepath.Join(filepath.Dir(source), "..", "..")
	publisherPath := filepath.Join(root, ".github", "workflows", "apple-compilation-cache.yml")
	publisher := readWorkflowFixture(t, publisherPath)
	on := mappingValueMust(t, publisher, "on")
	if _, ok := mappingValue(on, "workflow_dispatch"); !ok || len(on.Content) != 2 {
		t.Fatal("Apple cache publisher must expose only workflow_dispatch")
	}
	permissions := mappingValueMust(t, publisher, "permissions")
	if yamlScalar(t, permissions, "actions") != "write" || yamlScalar(t, permissions, "contents") != "read" {
		t.Fatal("Apple cache publisher permissions must be actions:write and contents:read")
	}
	publish := mappingValueMust(t, mappingValueMust(t, publisher, "jobs"), "publish")
	if got := yamlScalar(t, publish, "if"); got != "github.ref == 'refs/heads/main'" {
		t.Fatalf("Apple cache publisher main guard = %q", got)
	}
	publishSteps := mappingValueMust(t, publish, "steps").Content
	workflowUsesStepIndex(t, publishSteps, "actions/cache/restore")
	workflowUsesStepIndex(t, publishSteps, "actions/cache/save")
	workflowRunStepIndex(t, publishSteps, `./web/scripts/validate-apple-compilation-cache.sh produce "$RUNNER_TEMP/apple-compilation-cache.tar.zst"`)

	for _, workflowName := range []string{"ci-apple-mobile.yml", "ci-apple-tv.yml"} {
		path := filepath.Join(root, ".github", "workflows", workflowName)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		workflow := readWorkflowFixture(t, path)
		run := mappingValueMust(t, mappingValueMust(t, workflow, "jobs"), "run")
		steps := mappingValueMust(t, run, "steps").Content
		workflowUsesStepIndex(t, steps, "actions/cache/restore")
		if strings.Contains(string(data), "uses: actions/cache@") {
			t.Fatalf("%s must not write sibling-scoped dependency caches", workflowName)
		}
		if strings.Contains(string(data), "actions/cache/save@") {
			t.Fatalf("%s must remain restore-only for the compilation cache", workflowName)
		}
		if strings.Contains(string(data), "apple-clients-") || strings.Contains(string(data), "apple-jsi-") {
			t.Fatalf("%s must not perform dependency-cache lookups with no trusted writer", workflowName)
		}
		if !strings.Contains(string(data), "LOOMARR_APPLE_CACHE_MODE") || !strings.Contains(string(data), "LOOMARR_APPLE_CACHE_STORE") {
			t.Fatalf("%s does not pass the prepared compilation cache to the full Apple gate", workflowName)
		}
	}
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
