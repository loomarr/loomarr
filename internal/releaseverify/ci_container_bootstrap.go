package releaseverify

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const directCIPolicyBootstrap = "go run ./cmd/releaseverify -root ."

const (
	directCICheckoutAction     = "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"
	directCISetupGoAction      = "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"
	directCICacheRestoreAction = "actions/cache/restore@55cc8345863c7cc4c66a329aec7e433d2d1c52a9"
)

func directCIPolicyShellStepAuthorities() []struct {
	name string
	run  string
} {
	return []struct {
		name string
		run  string
	}{
		{name: "Repository policy bootstrap outside Make", run: directCIPolicyBootstrap},
		{name: "Workflow, publication, agent harness, and shell policy", run: "make -j4 --output-sync=target ci-lint release-verify agent-harness-test shellcheck"},
		{name: "Documentation contracts read by Go", run: "go test ./docs && make arch-docs-verify config-docs-verify dev-docs-verify"},
	}
}

// VerifyDirectCIPolicyBootstrap proves the root CI workflow reaches the
// repository policy verifier through one source-bound, unpoisoned execution
// context before any shell-controlled repository command can run.
func VerifyDirectCIPolicyBootstrap(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read root CI workflow: %w", err)
	}
	workflow, err := parseYAML(data)
	if err != nil {
		return fmt.Errorf("parse root CI workflow: %w", err)
	}
	if err := verifyOnlyKeys(workflow, "root CI workflow", "name", "on", "permissions", "concurrency", "env", "jobs"); err != nil {
		return err
	}
	if scalarValue(workflow, "name") != "CI" {
		return fmt.Errorf("root CI workflow name must equal CI")
	}
	environment, ok := mappingValue(workflow, "env")
	if !ok {
		return fmt.Errorf("root CI workflow must declare the audited tool-version environment")
	}
	if err := requireExactScalarMap(environment, "root CI workflow environment", standardWorkflowEnvironment()); err != nil {
		return err
	}
	jobs, err := requiredMap(workflow, "jobs")
	if err != nil {
		return fmt.Errorf("root CI workflow jobs: %w", err)
	}
	job, err := requiredMap(jobs, "ci-policy")
	if err != nil {
		return fmt.Errorf("root CI workflow ci-policy job: %w", err)
	}
	if err := verifyOnlyKeys(job, "root CI workflow ci-policy job", "name", "needs", "if", "runs-on", "steps"); err != nil {
		return err
	}
	if scalarValue(job, "name") != "CI policy — workflow, harness, and docs contracts" ||
		scalarValue(job, "needs") != "changes" ||
		scalarValue(job, "if") != "always()" ||
		scalarValue(job, "runs-on") != "ubuntu-latest" {
		return fmt.Errorf("root CI workflow ci-policy job differs from the fail-closed execution contract")
	}
	steps, err := requiredSequence(job, "steps")
	if err != nil {
		return fmt.Errorf("root CI workflow ci-policy steps: %w", err)
	}
	shellSteps := directCIPolicyShellStepAuthorities()
	if len(steps.Content) != 3+len(shellSteps) {
		return fmt.Errorf("root CI policy job must contain only the three audited setup actions and three policy shell steps")
	}
	if err := verifyDirectCICheckoutStep(steps.Content[0]); err != nil {
		return err
	}
	if err := verifyDirectCISetupGoStep(steps.Content[1]); err != nil {
		return err
	}
	if err := verifyDirectCIGoCacheStep(steps.Content[2]); err != nil {
		return err
	}
	for index, want := range shellSteps {
		step := steps.Content[index+3]
		label := fmt.Sprintf("root CI policy shell step %d", index+1)
		if step.Kind != yaml.MappingNode {
			return fmt.Errorf("%s must be a mapping", label)
		}
		if err := verifyOnlyKeys(step, label, "name", "run"); err != nil {
			return err
		}
		if scalarValue(step, "name") != want.name || scalarValue(step, "run") != want.run {
			return fmt.Errorf("%s differs from the source-bound policy command", label)
		}
	}
	return nil
}

func verifyDirectCICheckoutStep(step *yaml.Node) error {
	if step.Kind != yaml.MappingNode {
		return fmt.Errorf("root CI policy checkout step must be a mapping")
	}
	if err := verifyOnlyKeys(step, "root CI policy checkout step", "uses"); err != nil {
		return err
	}
	if scalarValue(step, "uses") != directCICheckoutAction {
		return fmt.Errorf("root CI policy checkout action differs from the source-bound revision")
	}
	return nil
}

func verifyDirectCISetupGoStep(step *yaml.Node) error {
	if step.Kind != yaml.MappingNode {
		return fmt.Errorf("root CI policy setup-go step must be a mapping")
	}
	if err := verifyOnlyKeys(step, "root CI policy setup-go step", "uses", "with"); err != nil {
		return err
	}
	if scalarValue(step, "uses") != directCISetupGoAction {
		return fmt.Errorf("root CI policy setup-go action differs from the source-bound revision")
	}
	inputs, err := requiredMap(step, "with")
	if err != nil {
		return fmt.Errorf("root CI policy setup-go inputs: %w", err)
	}
	return requireExactScalarMap(inputs, "root CI policy setup-go inputs", map[string]string{
		"go-version": "${{ env.GO_VERSION }}",
		"cache":      "false",
	})
}

// verifyDirectCIGoCacheStep allows one restore of the merge gate's cache, which only main can save.
func verifyDirectCIGoCacheStep(step *yaml.Node) error {
	if step.Kind != yaml.MappingNode {
		return fmt.Errorf("root CI policy Go cache step must be a mapping")
	}
	if err := verifyOnlyKeys(step, "root CI policy Go cache step", "name", "uses", "with"); err != nil {
		return err
	}
	if scalarValue(step, "name") != "Restore Go caches" || scalarValue(step, "uses") != directCICacheRestoreAction {
		return fmt.Errorf("root CI policy Go cache action differs from the source-bound revision")
	}
	inputs, err := requiredMap(step, "with")
	if err != nil {
		return fmt.Errorf("root CI policy Go cache inputs: %w", err)
	}
	return requireExactScalarMap(inputs, "root CI policy Go cache inputs", map[string]string{
		"path":         "~/go/pkg/mod\n~/.cache/go-build\n",
		"key":          "merge-gate-go-${{ runner.os }}-",
		"restore-keys": "merge-gate-go-${{ runner.os }}-\n",
	})
}
