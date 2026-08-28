package releaseverify

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestVerifyCIContainerDownloadsClosesRecursiveMakeGraphOptionsAndSubstitutions(t *testing.T) {
	commands := map[string]string{
		"shell command substitution":         `echo "$$(make test-pg)"`,
		"short makefile option":              `$(MAKE) -f attacker.mk safe`,
		"short makefile option after target": `$(MAKE) safe -f attacker.mk`,
		"long makefile option":               `$(MAKE) --makefile attacker.mk safe`,
		"equals makefile option":             `$(MAKE) safe --file=attacker.mk`,
		"directory changes graph":            `$(MAKE) -C attacker safe`,
		"dynamic makefile path":              `$(MAKE) -f "$$(printf attacker.mk)" safe`,
		"safe parallel option before target": `$(MAKE) -j2 test-pg`,
		"safe parallel option after target":  `$(MAKE) test-pg --jobs=2`,
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			modulePath := filepath.Join(root, "mk", "recursive-graph.mk")
			writeFixtureFile(t, modulePath, ".PHONY: android safe\nandroid:\n\t"+command+"\nsafe:\n\t@true\n")
			makefilePath := filepath.Join(root, "Makefile")
			writeFixtureFile(t, makefilePath, readFixtureFile(t, makefilePath)+"include mk/recursive-graph.mk\n")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), readRepositoryWorkflow(t, "ci-android.yml"))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted recursive Make execution outside the audited root graph")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRequiresBenignRunAuthorities(t *testing.T) {
	tests := map[string]struct {
		workflow string
		command  string
	}{
		"agent harness": {workflow: "ci-agent.yml", command: "make agent-harness-test"},
		"Android":       {workflow: "ci-android.yml", command: "make android"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, ".github", "workflows", test.workflow)
			workflow := readRepositoryWorkflow(t, test.workflow)
			writeFixtureFile(t, path, strings.Replace(workflow, "run: "+test.command, "run: echo authority removed", 1))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted deletion of a registered benign run authority")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsBindsSourceJobsToExactExecutionContext(t *testing.T) {
	tests := map[string]struct {
		workflow string
		mutate   func(string) string
	}{
		"Playwright runner replaced": {
			workflow: playwrightWorkflowName,
			mutate: func(source string) string {
				return strings.Replace(source, "runs-on: ubuntu-latest", "runs-on: windows-latest", 1)
			},
		},
		"Playwright runner removed": {
			workflow: playwrightWorkflowName,
			mutate: func(source string) string {
				return strings.Replace(source, "    runs-on: ubuntu-latest\n", "", 1)
			},
		},
		"Playwright strategy removed": {
			workflow: playwrightWorkflowName,
			mutate: func(source string) string {
				var document yaml.Node
				if err := yaml.Unmarshal([]byte(source), &document); err != nil {
					panic(err)
				}
				removeWorkflowJobKey(document.Content[0], "run", "strategy")
				result, err := yaml.Marshal(&document)
				if err != nil {
					panic(err)
				}
				return string(result)
			},
		},
		"Playwright matrix replaced": {
			workflow: playwrightWorkflowName,
			mutate: func(source string) string {
				return strings.Replace(source, "shard: [1, 2, 3, 4]", "shard: [1, 2]", 1)
			},
		},
		"Playwright needs added": {
			workflow: playwrightWorkflowName,
			mutate: func(source string) string {
				return strings.Replace(source, "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n    needs: attacker\n", 1)
			},
		},
		"Playwright timeout added": {
			workflow: playwrightWorkflowName,
			mutate: func(source string) string {
				return strings.Replace(source, "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n    timeout-minutes: 1\n", 1)
			},
		},
		"agent strategy added": {
			workflow: "ci-agent.yml",
			mutate: func(source string) string {
				return strings.Replace(source, "    runs-on: macos-latest\n", "    runs-on: macos-latest\n    strategy:\n      fail-fast: false\n      matrix:\n        shard: [1]\n", 1)
			},
		},
		"policy needs removed": {
			workflow: "ci.yml",
			mutate: func(source string) string {
				return strings.Replace(source,
					"  ci-policy:\n    name: CI policy — workflow, harness, and docs contracts\n    needs: changes\n",
					"  ci-policy:\n    name: CI policy — workflow, harness, and docs contracts\n", 1)
			},
		},
		"policy needs replaced": {
			workflow: "ci.yml",
			mutate: func(source string) string {
				return strings.Replace(source,
					"  ci-policy:\n    name: CI policy — workflow, harness, and docs contracts\n    needs: changes\n",
					"  ci-policy:\n    name: CI policy — workflow, harness, and docs contracts\n    needs: attacker\n", 1)
			},
		},
		"image runner replaced": {
			workflow: "ci-image.yml",
			mutate: func(source string) string {
				return strings.Replace(source, "runs-on: ${{ matrix.runner }}", "runs-on: ubuntu-latest", 1)
			},
		},
		"image timeout removed": {
			workflow: "ci-image.yml",
			mutate: func(source string) string {
				return strings.Replace(source, "    timeout-minutes: 45\n", "", 1)
			},
		},
		"image timeout replaced": {
			workflow: "ci-image.yml",
			mutate: func(source string) string {
				return strings.Replace(source, "    timeout-minutes: 45\n", "    timeout-minutes: 44\n", 1)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, ".github", "workflows", test.workflow)
			writeFixtureFile(t, path, test.mutate(readRepositoryWorkflow(t, test.workflow)))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted changed source-bound job execution context")
			}
		})
	}
}

func removeWorkflowJobKey(workflow *yaml.Node, jobName, key string) {
	jobs, _ := mappingValue(workflow, "jobs")
	job, _ := mappingValue(jobs, jobName)
	for index := 0; index < len(job.Content); index += 2 {
		if job.Content[index].Value == key {
			job.Content = append(job.Content[:index], job.Content[index+2:]...)
			return
		}
	}
}
