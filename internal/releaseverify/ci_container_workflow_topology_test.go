package releaseverify

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestVerifyCIContainerDownloadsRequiresExactWorkflowTopology(t *testing.T) {
	tests := map[string]func(*testing.T, string){
		"extra workflow": func(t *testing.T, root string) {
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), `name: unregistered
on: push
jobs:
  benign:
    runs-on: ubuntu-latest
    steps:
      - run: echo unregistered
`)
		},
		"extra job": func(t *testing.T, root string) {
			path := filepath.Join(root, ".github", "workflows", "ci-docs.yml")
			writeFixtureFile(t, path, readFixtureFile(t, path)+`  unregistered:
    name: Unregistered metadata-only job
    runs-on: ubuntu-latest
`)
		},
		"extra unrelated run step": func(t *testing.T, root string) {
			path := filepath.Join(root, ".github", "workflows", "ci-docs.yml")
			source := readFixtureFile(t, path)
			anchor := "          pnpm build\n"
			if !strings.Contains(source, anchor) {
				t.Fatal("CI docs final run step fixture changed")
			}
			writeFixtureFile(t, path, strings.Replace(source, anchor, anchor+"\n      - run: echo unregistered\n", 1))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			mutate(t, root)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted workflow topology outside the registered authority")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRequiresExactReusableWorkflowCallerShape(t *testing.T) {
	fields := map[string]string{
		"inherited secrets":  "    secrets: inherit\n",
		"caller inputs":      "    with:\n      attacker: value\n",
		"caller permissions": "    permissions:\n      contents: write\n",
	}
	for name, field := range fields {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, ".github", "workflows", "ci.yml")
			source := readFixtureFile(t, path)
			anchor := "  docs:\n    name: Docs — links + structure + prose\n    needs: changes\n    if: needs.changes.outputs.impact_docs == 'true'\n    uses: ./.github/workflows/ci-docs.yml\n"
			if !strings.Contains(source, anchor) {
				t.Fatal("CI docs reusable-workflow caller fixture changed")
			}
			writeFixtureFile(t, path, strings.Replace(source, anchor, anchor+field, 1))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted fields outside the exact reusable-workflow caller authority")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsBindsCacheCleanupCommandAndContext(t *testing.T) {
	tests := map[string]func(*yaml.Node){
		"changed run command": func(workflow *yaml.Node) {
			job := workflowJobNode(t, workflow, "cleanup")
			steps, _ := mappingValue(job, "steps")
			setYAMLMappingValue(steps.Content[0], "run", scalarYAML("echo changed"))
		},
		"workflow environment": func(workflow *yaml.Node) {
			setYAMLMappingValue(workflow, "env", yamlMapping("PATH", "/tmp/attacker"))
		},
		"workflow permissions": func(workflow *yaml.Node) {
			setYAMLMappingValue(workflow, "permissions", yamlMapping("actions", "read"))
		},
		"job runner": func(workflow *yaml.Node) {
			setYAMLMappingValue(workflowJobNode(t, workflow, "cleanup"), "runs-on", scalarYAML("windows-latest"))
		},
		"job environment": func(workflow *yaml.Node) {
			setYAMLMappingValue(workflowJobNode(t, workflow, "cleanup"), "env", yamlMapping("PATH", "/tmp/attacker"))
		},
		"job defaults": func(workflow *yaml.Node) {
			setYAMLMappingValue(workflowJobNode(t, workflow, "cleanup"), "defaults", yamlMapping("run", "attacker"))
		},
		"job condition": func(workflow *yaml.Node) {
			setYAMLMappingValue(workflowJobNode(t, workflow, "cleanup"), "if", scalarYAML("false"))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			var document yaml.Node
			if err := yaml.Unmarshal([]byte(readRepositoryWorkflow(t, "cache-cleanup.yml")), &document); err != nil {
				t.Fatal(err)
			}
			mutate(document.Content[0])
			data, err := yaml.Marshal(&document)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "cache-cleanup.yml"), string(data))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted changed cache-cleanup command or inherited context")
			}
		})
	}
}
