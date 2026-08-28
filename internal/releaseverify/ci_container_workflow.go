package releaseverify

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var fullCommitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

func postgresWorkflowActionAuthorities() map[string]map[string]struct{} {
	return map[string]map[string]struct{}{
		"actions/checkout":      {},
		"actions/setup-go":      {"cache": {}, "go-version": {}},
		"actions/cache/restore": {"path": {}, "key": {}, "restore-keys": {}},
		"actions/cache/save":    {"path": {}, "key": {}},
	}
}

func postgresWorkflowRunAuthorities() map[string]struct{} {
	return map[string]struct{}{
		`echo "week=$(date -u +%GW%V)" >> "$GITHUB_OUTPUT"`: {},
		"make test-pg": {},
	}
}

func verifyPostgresWorkflow(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read postgres CI workflow: %w", err)
	}
	workflow, err := parseYAML(data)
	if err != nil {
		return fmt.Errorf("parse postgres CI workflow: %w", err)
	}
	if err := verifyOnlyKeySet(workflow, "workflow", setOf("name", "on", "permissions", "env", "jobs")); err != nil {
		return err
	}
	if name, err := requiredScalar(workflow, "name"); err != nil || name != "CI / Store conformance (Postgres)" {
		return fmt.Errorf("postgres CI workflow name must equal CI / Store conformance (Postgres)")
	}
	if err := verifyWorkflowTrigger(workflow); err != nil {
		return err
	}
	if permissions, ok := mappingValue(workflow, "permissions"); ok {
		if err := requireExactScalarMap(permissions, "workflow permissions", map[string]string{"contents": "read"}); err != nil {
			return err
		}
	}
	if environment, ok := mappingValue(workflow, "env"); ok {
		if err := requireExactScalarMap(environment, "workflow environment", map[string]string{
			"GO_VERSION": "1.27", "NODE_VERSION": "22",
		}); err != nil {
			return err
		}
	}
	jobs, err := requiredMap(workflow, "jobs")
	if err != nil {
		return fmt.Errorf("postgres CI workflow jobs: %w", err)
	}
	if err := verifyOnlyKeySet(jobs, "postgres CI workflow jobs", setOf("run")); err != nil {
		return err
	}
	job, err := requiredMap(jobs, "run")
	if err != nil {
		return fmt.Errorf("postgres CI workflow run job: %w", err)
	}
	return verifyPostgresWorkflowJob(job)
}

func verifyWorkflowTrigger(workflow *yaml.Node) error {
	trigger, ok := mappingValue(workflow, "on")
	if !ok || trigger.Kind != yaml.MappingNode {
		return fmt.Errorf("postgres CI workflow on must be a mapping")
	}
	if err := verifyOnlyKeySet(trigger, "postgres CI workflow trigger", setOf("workflow_call")); err != nil {
		return err
	}
	call, ok := mappingValue(trigger, "workflow_call")
	if !ok || call.Kind != yaml.ScalarNode || call.Value != "" {
		return fmt.Errorf("postgres CI workflow_call must not accept unaudited inputs")
	}
	return nil
}

func verifyPostgresWorkflowJob(job *yaml.Node) error {
	if err := verifyOnlyKeySet(job, "postgres CI workflow run job", setOf("name", "runs-on", "steps")); err != nil {
		return err
	}
	if runner, err := requiredScalar(job, "runs-on"); err != nil || runner != "ubuntu-latest" {
		return fmt.Errorf("postgres CI workflow run job runs-on must equal ubuntu-latest")
	}
	if name, ok := mappingValue(job, "name"); ok && !nonEmptyScalar(name) {
		return fmt.Errorf("postgres CI workflow run job name must be a non-empty scalar")
	}
	steps, err := requiredSequence(job, "steps")
	if err != nil {
		return fmt.Errorf("postgres CI workflow steps: %w", err)
	}
	if len(steps.Content) == 0 {
		return fmt.Errorf("postgres CI workflow must contain executable steps")
	}
	testRuns := 0
	for index, step := range steps.Content {
		if err := verifyPostgresWorkflowStep(step, index+1); err != nil {
			return err
		}
		if value, ok := mappingValue(step, "run"); ok && value.Value == "make test-pg" {
			testRuns++
		}
	}
	if testRuns != 1 {
		return fmt.Errorf("postgres CI workflow must run make test-pg exactly once, found %d", testRuns)
	}
	return nil
}

func verifyPostgresWorkflowStep(step *yaml.Node, index int) error {
	label := fmt.Sprintf("postgres CI workflow step %d", index)
	if err := verifyOnlyKeySet(step, label, setOf("name", "id", "run", "uses", "with", "if")); err != nil {
		return err
	}
	for _, key := range []string{"name", "id"} {
		if value, ok := mappingValue(step, key); ok && !nonEmptyScalar(value) {
			return fmt.Errorf("%s %s must be a non-empty scalar", label, key)
		}
	}
	run, hasRun := mappingValue(step, "run")
	uses, hasUses := mappingValue(step, "uses")
	if hasRun == hasUses {
		return fmt.Errorf("%s must contain exactly one of run or uses", label)
	}
	if hasRun {
		if !nonEmptyScalar(run) {
			return fmt.Errorf("%s run must be a non-empty scalar", label)
		}
		if _, allowed := postgresWorkflowRunAuthorities()[strings.TrimSpace(run.Value)]; !allowed {
			return fmt.Errorf("%s uses unaudited shell command", label)
		}
		if _, ok := mappingValue(step, "with"); ok {
			return fmt.Errorf("%s shell command must not declare with", label)
		}
		if _, ok := mappingValue(step, "if"); ok {
			return fmt.Errorf("%s shell command must not be conditional", label)
		}
		return nil
	}
	if !nonEmptyScalar(uses) {
		return fmt.Errorf("%s uses must be a non-empty scalar", label)
	}
	action, revision, found := strings.Cut(uses.Value, "@")
	inputs, allowed := postgresWorkflowActionAuthorities()[action]
	if !found || !allowed || !fullCommitSHA.MatchString(revision) {
		return fmt.Errorf("%s uses unaudited or unpinned action %s", label, uses.Value)
	}
	if condition, ok := mappingValue(step, "if"); ok {
		if !nonEmptyScalar(condition) || condition.Value != "always() && github.ref == 'refs/heads/main'" || action != "actions/cache/save" {
			return fmt.Errorf("%s has an unaudited condition", label)
		}
	}
	with, hasWith := mappingValue(step, "with")
	if len(inputs) == 0 {
		if hasWith {
			return fmt.Errorf("%s action %s must not declare inputs", label, action)
		}
		return nil
	}
	if !hasWith {
		return fmt.Errorf("%s action %s is missing audited inputs", label, action)
	}
	if err := verifyOnlyKeySet(with, label+" inputs", inputs); err != nil {
		return err
	}
	for key := range inputs {
		value, ok := mappingValue(with, key)
		if !ok || !nonEmptyScalar(value) {
			return fmt.Errorf("%s action %s input %s must be a non-empty scalar", label, action, key)
		}
	}
	return nil
}

func requireExactScalarMap(node *yaml.Node, label string, want map[string]string) error {
	if err := verifyOnlyKeySet(node, label, setOfMap(want)); err != nil {
		return err
	}
	if len(node.Content)/2 != len(want) {
		return fmt.Errorf("%s must contain exactly the audited values", label)
	}
	for key, expected := range want {
		value, ok := mappingValue(node, key)
		if !ok || value.Kind != yaml.ScalarNode || value.Value != expected {
			return fmt.Errorf("%s %s must equal %s", label, key, expected)
		}
	}
	return nil
}

func requiredScalar(node *yaml.Node, key string) (string, error) {
	value, ok := mappingValue(node, key)
	if !ok || !nonEmptyScalar(value) {
		return "", fmt.Errorf("%s must be a non-empty scalar", key)
	}
	return value.Value, nil
}

func nonEmptyScalar(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && strings.TrimSpace(node.Value) != ""
}

func setOf(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func setOfMap(values map[string]string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for key := range values {
		out[key] = struct{}{}
	}
	return out
}
