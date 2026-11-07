package releaseverify

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// VerifyCodeQLWorkflow binds the events that provide full scans and the outputs that gate
// per-language PR scans. Step and permission authority is enforced by the shared workflow audit.
func VerifyCodeQLWorkflow(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	root, err := parseYAML(data)
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(root, "CodeQL workflow", "name", "on", "permissions", "concurrency", "jobs"); err != nil {
		return err
	}
	if scalarValue(root, "name") != "CodeQL" {
		return errors.New("CodeQL workflow name must remain CodeQL")
	}
	if err := verifyCodeQLTriggers(root); err != nil {
		return err
	}
	if err := verifyCodeQLConcurrency(root); err != nil {
		return err
	}
	jobs, err := requiredMap(root, "jobs")
	if err != nil {
		return err
	}
	changes, err := requiredMap(jobs, "changes")
	if err != nil {
		return err
	}
	return verifyCodeQLOutputs(changes)
}

func verifyCodeQLTriggers(root *yaml.Node) error {
	trigger, err := requiredMap(root, "on")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(trigger, "CodeQL trigger", "pull_request", "push", "schedule", "workflow_dispatch"); err != nil {
		return err
	}
	for _, name := range []string{"pull_request", "workflow_dispatch"} {
		value, ok := mappingValue(trigger, name)
		if !ok || !emptyWorkflowValue(value) {
			return fmt.Errorf("CodeQL %s trigger must remain unfiltered", name)
		}
	}
	push, err := requiredMap(trigger, "push")
	if err != nil || !mappingHasOnlyKeys(push, "branches") {
		return errors.New("CodeQL push trigger must target only main")
	}
	branches, ok := mappingValue(push, "branches")
	if !ok || branches.Kind != yaml.SequenceNode || len(branches.Content) != 1 || branches.Content[0].Value != "main" {
		return errors.New("CodeQL push trigger must target only main")
	}
	schedule, ok := mappingValue(trigger, "schedule")
	if !ok || schedule.Kind != yaml.SequenceNode || len(schedule.Content) != 1 || schedule.Content[0].Kind != yaml.MappingNode ||
		!mappingHasOnlyKeys(schedule.Content[0], "cron") || scalarValue(schedule.Content[0], "cron") != "17 3 * * 3" {
		return errors.New("CodeQL weekly full-scan schedule differs from its authority")
	}
	return nil
}

func emptyWorkflowValue(node *yaml.Node) bool {
	return (node.Kind == yaml.ScalarNode && node.Tag == "!!null") ||
		(node.Kind == yaml.MappingNode && len(node.Content) == 0)
}

func verifyCodeQLConcurrency(root *yaml.Node) error {
	concurrency, err := requiredMap(root, "concurrency")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(concurrency, "CodeQL concurrency", "group", "cancel-in-progress"); err != nil {
		return err
	}
	if scalarValue(concurrency, "group") != "codeql-${{ github.ref }}" ||
		scalarValue(concurrency, "cancel-in-progress") != "${{ github.event_name == 'pull_request' }}" {
		return errors.New("CodeQL concurrency must cancel only superseded pull-request scans")
	}
	return nil
}

func verifyCodeQLOutputs(changes *yaml.Node) error {
	outputs, err := requiredMap(changes, "outputs")
	if err != nil {
		return err
	}
	want := map[string]string{
		"actions":               "${{ steps.impact.outputs.actions }}",
		"go":                    "${{ steps.impact.outputs.go }}",
		"javascript-typescript": "${{ steps.impact.outputs.javascript-typescript }}",
		"python":                "${{ steps.impact.outputs.python }}",
		"ruby":                  "${{ steps.impact.outputs.ruby }}",
		"rust":                  "${{ steps.impact.outputs.rust }}",
	}
	keys := make([]string, 0, len(want))
	for key := range want {
		keys = append(keys, key)
	}
	if !mappingHasOnlyKeys(outputs, keys...) {
		return errors.New("CodeQL language outputs differ from their authority")
	}
	for key, value := range want {
		if scalarValue(outputs, key) != value {
			return fmt.Errorf("CodeQL language output %s differs from its authority", key)
		}
	}
	return nil
}
