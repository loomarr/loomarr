package releaseverify

import (
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func mutateWorkflowSteps(t *testing.T, source string, mutate func(*testing.T, []*yaml.Node) []*yaml.Node) string {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(source), &document); err != nil {
		t.Fatal(err)
	}
	workflow := document.Content[0]
	jobs, ok := mappingValue(workflow, "jobs")
	if !ok {
		t.Fatal("workflow fixture has no jobs")
	}
	job, ok := mappingValue(jobs, "run")
	if !ok {
		t.Fatal("workflow fixture has no run job")
	}
	steps, ok := mappingValue(job, "steps")
	if !ok || steps.Kind != yaml.SequenceNode {
		t.Fatal("workflow fixture has no step sequence")
	}
	steps.Content = mutate(t, append([]*yaml.Node(nil), steps.Content...))
	result, err := yaml.Marshal(&document)
	if err != nil {
		t.Fatal(err)
	}
	return string(result)
}

func workflowRunStepIndex(t *testing.T, steps []*yaml.Node, command string) int {
	t.Helper()
	for index, step := range steps {
		if run, ok := mappingValue(step, "run"); ok && run.Kind == yaml.ScalarNode && run.Value == command {
			return index
		}
	}
	t.Fatalf("workflow fixture has no run step %q", command)
	return -1
}

func workflowUsesStepIndex(t *testing.T, steps []*yaml.Node, action string) int {
	t.Helper()
	for index, step := range steps {
		if uses, ok := mappingValue(step, "uses"); ok && uses.Kind == yaml.ScalarNode && strings.HasPrefix(uses.Value, action+"@") {
			return index
		}
	}
	t.Fatalf("workflow fixture has no uses step for %s", action)
	return -1
}

func workflowRunStep(t *testing.T, command string) *yaml.Node {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte("run: "+strconv.Quote(command)+"\n"), &document); err != nil {
		t.Fatal(err)
	}
	return document.Content[0]
}

func moveWorkflowStep(steps []*yaml.Node, from, to int) []*yaml.Node {
	step := steps[from]
	steps = removeWorkflowStep(steps, from)
	if from < to {
		to--
	}
	return insertWorkflowStep(steps, to, step)
}

func insertWorkflowStep(steps []*yaml.Node, index int, step *yaml.Node) []*yaml.Node {
	steps = append(steps, nil)
	copy(steps[index+1:], steps[index:])
	steps[index] = step
	return steps
}

func removeWorkflowStep(steps []*yaml.Node, index int) []*yaml.Node {
	return append(steps[:index], steps[index+1:]...)
}
