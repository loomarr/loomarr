package releaseverify

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestVerifyCIContainerDownloadsBindsEveryPinnedActionAtItsExactStep(t *testing.T) {
	workflowJobs := map[string]string{
		"ci-agent.yml":               "run",
		"ci-android.yml":             "run",
		"ci-apple-mobile.yml":        "run",
		"ci-apple-tv.yml":            "run",
		"ci-clients.yml":             "run",
		"ci-docs.yml":                "run",
		"ci-frontend.yml":            "run",
		"ci-go-contracts.yml":        "run",
		"ci-go.yml":                  "run",
		"ci-image-certification.yml": "run",
		"ci-image.yml":               "run",
		"ci-playwright.yml":          "run",
		"ci-postgres.yml":            "run",
		"ci-rust-contracts.yml":      "run",
		"ci-tuner.yml":               "run",
		"ci.yml":                     "ci-policy",
		"image-benchmark.yml":        "benchmark",
	}
	workflows := make([]string, 0, len(workflowJobs))
	for workflow := range workflowJobs {
		workflows = append(workflows, workflow)
	}
	sort.Strings(workflows)
	for _, workflow := range workflows {
		job := workflowJobs[workflow]
		source := readRepositoryWorkflow(t, workflow)
		indices := workflowActionStepIndices(t, source, job)
		if len(indices) == 0 {
			t.Fatalf("%s job %s has no pinned actions", workflow, job)
		}
		for _, stepIndex := range indices {
			name := strings.TrimSuffix(workflow, ".yml") + "-step-" + string(rune('A'+stepIndex))
			t.Run(name, func(t *testing.T) {
				root := writeCIContainerDownloadsFixture(t)
				path := filepath.Join(root, ".github", "workflows", workflow)
				mutated := mutateWorkflowJobSteps(t, source, job, func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
					uses, ok := mappingValue(steps[stepIndex], "uses")
					if !ok {
						t.Fatalf("step %d is no longer an action", stepIndex)
					}
					uses.Value = "actions/cache@0000000000000000000000000000000000000000"
					return steps
				})
				writeFixtureFile(t, path, mutated)
				if err := VerifyCIContainerDownloads(root); err == nil {
					t.Fatal("VerifyCIContainerDownloads accepted a same-slot replacement of a pinned action")
				}
			})
		}
	}
}

func TestVerifyCIContainerDownloadsRegistersEveryPinnedRepositoryAction(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	workflowsDir := filepath.Join(filepath.Dir(source), "..", "..", ".github", "workflows")
	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		t.Fatal(err)
	}
	registered := make(map[workflowActionKey]struct{})
	for _, entry := range entries {
		if entry.IsDir() || (filepath.Ext(entry.Name()) != ".yml" && filepath.Ext(entry.Name()) != ".yaml") {
			continue
		}
		workflow := readRepositoryWorkflow(t, entry.Name())
		var document yaml.Node
		if err := yaml.Unmarshal([]byte(workflow), &document); err != nil {
			t.Fatal(err)
		}
		jobs, ok := mappingValue(document.Content[0], "jobs")
		if !ok || jobs.Kind != yaml.MappingNode {
			continue
		}
		for index := 0; index < len(jobs.Content); index += 2 {
			jobName, job := jobs.Content[index].Value, jobs.Content[index+1]
			steps, ok := mappingValue(job, "steps")
			if !ok || steps.Kind != yaml.SequenceNode {
				continue
			}
			for stepIndex, step := range steps.Content {
				uses, ok := mappingValue(step, "uses")
				if !ok {
					continue
				}
				if uses.Kind != yaml.ScalarNode || !actionPin.MatchString(strings.TrimSpace(uses.Value)) {
					t.Fatalf("%s job %s step %d is not pinned to a full SHA", entry.Name(), jobName, stepIndex+1)
				}
				key := workflowActionKey{workflow: entry.Name(), job: jobName, step: stepIndex}
				registered[key] = struct{}{}
				if _, registered := workflowAuthorityCatalog().actions[key]; !registered {
					t.Fatalf("%s job %s absolute step %d is not registered", entry.Name(), jobName, stepIndex+1)
				}
			}
		}
	}
	for key := range workflowAuthorityCatalog().actions {
		if _, present := registered[key]; !present {
			t.Fatalf("registered action %s job %s absolute step %d is absent from the repository", key.workflow, key.job, key.step+1)
		}
	}
}

func TestVerifyCIContainerDownloadsBindsPinnedActionShapeAndContext(t *testing.T) {
	tests := map[string]func(*testing.T, *yaml.Node, []*yaml.Node) []*yaml.Node{
		"checkout replaced by safe run": func(t *testing.T, _ *yaml.Node, steps []*yaml.Node) []*yaml.Node {
			steps[0] = workflowRunStep(t, "echo safe")
			return steps
		},
		"checkout gains name": func(_ *testing.T, _ *yaml.Node, steps []*yaml.Node) []*yaml.Node {
			appendYAMLScalar(steps[0], "name", "Different checkout")
			return steps
		},
		"build action changes name": func(_ *testing.T, _ *yaml.Node, steps []*yaml.Node) []*yaml.Node {
			name, _ := mappingValue(steps[2], "name")
			name.Value = "Different build"
			return steps
		},
		"build action changes inputs": func(_ *testing.T, _ *yaml.Node, steps []*yaml.Node) []*yaml.Node {
			with, _ := mappingValue(steps[2], "with")
			tags, _ := mappingValue(with, "tags")
			tags.Value = "attacker.invalid/image:pinned"
			return steps
		},
		"build action gains environment": func(_ *testing.T, _ *yaml.Node, steps []*yaml.Node) []*yaml.Node {
			appendYAMLMapping(steps[2], "env", "ATTACK", "1")
			return steps
		},
		"build action gains condition": func(_ *testing.T, _ *yaml.Node, steps []*yaml.Node) []*yaml.Node {
			appendYAMLScalar(steps[2], "if", "false")
			return steps
		},
		"workflow permissions change": func(_ *testing.T, workflow *yaml.Node, steps []*yaml.Node) []*yaml.Node {
			permissions, _ := mappingValue(workflow, "permissions")
			contents, _ := mappingValue(permissions, "contents")
			contents.Value = "write"
			return steps
		},
		"job permissions added": func(_ *testing.T, workflow *yaml.Node, steps []*yaml.Node) []*yaml.Node {
			jobs, _ := mappingValue(workflow, "jobs")
			job, _ := mappingValue(jobs, "run")
			appendYAMLMapping(job, "permissions", "contents", "write")
			return steps
		},
		"checkout duplicated": func(_ *testing.T, _ *yaml.Node, steps []*yaml.Node) []*yaml.Node {
			return insertWorkflowStep(steps, 1, steps[0])
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, ".github", "workflows", "ci-image.yml")
			writeFixtureFile(t, path, mutateWorkflowJob(t, readRepositoryWorkflow(t, "ci-image.yml"), "run", mutate))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted changed pinned-action identity or execution context")
			}
		})
	}
}

func workflowActionStepIndices(t *testing.T, source, jobName string) []int {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(source), &document); err != nil {
		t.Fatal(err)
	}
	workflow := document.Content[0]
	jobs, _ := mappingValue(workflow, "jobs")
	job, _ := mappingValue(jobs, jobName)
	steps, _ := mappingValue(job, "steps")
	var result []int
	for index, step := range steps.Content {
		if _, ok := mappingValue(step, "uses"); ok {
			result = append(result, index)
		}
	}
	return result
}

func mutateWorkflowJobSteps(t *testing.T, source, jobName string, mutate func(*testing.T, []*yaml.Node) []*yaml.Node) string {
	t.Helper()
	return mutateWorkflowJob(t, source, jobName, func(t *testing.T, _ *yaml.Node, steps []*yaml.Node) []*yaml.Node {
		return mutate(t, steps)
	})
}

func mutateWorkflowJob(t *testing.T, source, jobName string, mutate func(*testing.T, *yaml.Node, []*yaml.Node) []*yaml.Node) string {
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
	job, ok := mappingValue(jobs, jobName)
	if !ok {
		t.Fatalf("workflow fixture has no job %s", jobName)
	}
	steps, ok := mappingValue(job, "steps")
	if !ok || steps.Kind != yaml.SequenceNode {
		t.Fatal("workflow fixture has no step sequence")
	}
	steps.Content = mutate(t, workflow, append([]*yaml.Node(nil), steps.Content...))
	result, err := yaml.Marshal(&document)
	if err != nil {
		t.Fatal(err)
	}
	return string(result)
}

func appendYAMLScalar(mapping *yaml.Node, key, value string) {
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

func appendYAMLMapping(mapping *yaml.Node, key, childKey, childValue string) {
	child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	appendYAMLScalar(child, childKey, childValue)
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		child,
	)
}
