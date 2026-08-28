package releaseverify

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestVerifyCIContainerDownloadsRequiresEachContainerAuthorityExactlyOnce(t *testing.T) {
	tests := map[string]func(string) string{
		"removed Postgres route": func(workflow string) string {
			return strings.Replace(workflow, "      - run: make test-pg\n", "      - run: echo safe\n", 1)
		},
		"duplicated Postgres route": func(workflow string) string {
			return strings.Replace(workflow, "      - run: make test-pg\n", "      - run: make test-pg\n      - run: make test-pg\n", 1)
		},
		"moved Postgres route": func(workflow string) string {
			return strings.Replace(workflow, "      - run: make test-pg\n", "      - run: echo safe\n      - run: make test-pg\n", 1)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, ".github", "workflows", postgresWorkflowName)
			writeFixtureFile(t, path, mutate(readFixtureFile(t, path)))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted wrong Postgres authority cardinality")
			}
		})
	}

	playwrightMutations := map[string]func(string) string{
		"moved visual route": func(workflow string) string {
			return strings.Replace(workflow, "      - name: Visual + a11y over storybook-static\n", "      - run: echo safe\n      - name: Visual + a11y over storybook-static\n", 1)
		},
		"relabelled visual route": func(workflow string) string {
			return strings.Replace(workflow, "Visual + a11y over storybook-static", "Moved visual route", 1)
		},
		"relabelled e2e route": func(workflow string) string {
			return strings.Replace(workflow, "Wizard e2e smoke vs a mocked backend", "Relabelled e2e route", 1)
		},
		"duplicated visual mode": func(workflow string) string {
			needle := "      - name: Visual + a11y over storybook-static\n        run: " + playwrightVisualWorkflowStep + "\n"
			return strings.Replace(workflow, needle, needle+needle, 1)
		},
	}
	for name, mutate := range playwrightMutations {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, ".github", "workflows", playwrightWorkflowName)
			writeFixtureFile(t, path, mutate(readFixtureFile(t, path)))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a moved, relabelled, or duplicated Playwright authority")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsBindsRequiredAuthoritiesToAbsoluteWorkflowSteps(t *testing.T) {
	tests := map[string]struct {
		workflow string
		mutate   func(*testing.T, []*yaml.Node) []*yaml.Node
	}{
		"image route before checkout action": {
			workflow: "ci-image.yml",
			mutate: func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
				return moveWorkflowStep(steps, workflowRunStepIndex(t, steps, packagedImageMetadataInspection), 0)
			},
		},
		"image route between pinned actions": {
			workflow: "ci-image.yml",
			mutate: func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
				return moveWorkflowStep(steps, workflowRunStepIndex(t, steps, packagedImageMetadataInspection), 1)
			},
		},
		"image route before build action": {
			workflow: "ci-image.yml",
			mutate: func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
				return moveWorkflowStep(steps, workflowRunStepIndex(t, steps, packagedImageMetadataInspection), 2)
			},
		},
		"insert pinned action before image route": {
			workflow: "ci-image.yml",
			mutate: func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
				checkout := steps[workflowUsesStepIndex(t, steps, "actions/checkout")]
				index := workflowRunStepIndex(t, steps, packagedImageMetadataInspection)
				return insertWorkflowStep(steps, index, checkout)
			},
		},
		"remove pinned action before image route": {
			workflow: "ci-image.yml",
			mutate: func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
				return removeWorkflowStep(steps, workflowUsesStepIndex(t, steps, "docker/setup-buildx-action"))
			},
		},
		"move Playwright route across cache action": {
			workflow: playwrightWorkflowName,
			mutate: func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
				cache := workflowUsesStepIndex(t, steps, "actions/cache")
				visual := workflowRunStepIndex(t, steps, playwrightVisualWorkflowStep)
				return moveWorkflowStep(steps, cache, visual+1)
			},
		},
		"move Playwright route after upload action": {
			workflow: playwrightWorkflowName,
			mutate: func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
				e2e := workflowRunStepIndex(t, steps, "make e2e")
				upload := workflowUsesStepIndex(t, steps, "actions/upload-artifact")
				return moveWorkflowStep(steps, e2e, upload+1)
			},
		},
		"insert run step before image route": {
			workflow: "ci-image.yml",
			mutate: func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
				return insertWorkflowStep(steps, workflowRunStepIndex(t, steps, packagedImageMetadataInspection), workflowRunStep(t, "echo safe"))
			},
		},
		"remove run step before Playwright route": {
			workflow: playwrightWorkflowName,
			mutate: func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
				return removeWorkflowStep(steps, workflowRunStepIndex(t, steps, "corepack enable"))
			},
		},
		"swap Playwright run steps": {
			workflow: playwrightWorkflowName,
			mutate: func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
				visual := workflowRunStepIndex(t, steps, playwrightVisualWorkflowStep)
				e2e := workflowRunStepIndex(t, steps, "make e2e")
				steps[visual], steps[e2e] = steps[e2e], steps[visual]
				return steps
			},
		},
		"image authority has run and uses kinds": {
			workflow: "ci-image.yml",
			mutate: func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
				index := workflowRunStepIndex(t, steps, packagedImageMetadataInspection)
				uses, _ := mappingValue(steps[workflowUsesStepIndex(t, steps, "actions/checkout")], "uses")
				steps[index].Content = append(steps[index].Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: "uses"},
					&yaml.Node{Kind: yaml.ScalarNode, Value: uses.Value},
				)
				return steps
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, ".github", "workflows", test.workflow)
			writeFixtureFile(t, path, mutateWorkflowSteps(t, readFixtureFile(t, path), test.mutate))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a required authority outside its absolute workflow step")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsBindsEverySourceBoundAuthorityToItsStep(t *testing.T) {
	tests := map[string]func(*testing.T, []*yaml.Node) []*yaml.Node{
		"insert action before benign authority": func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
			checkout := steps[workflowUsesStepIndex(t, steps, "actions/checkout")]
			return insertWorkflowStep(steps, workflowRunStepIndex(t, steps, "make android"), checkout)
		},
		"name previously unnamed benign authority": func(t *testing.T, steps []*yaml.Node) []*yaml.Node {
			index := workflowRunStepIndex(t, steps, "make android")
			steps[index].Content = append(steps[index].Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "name"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "Moved Android authority"},
			)
			return steps
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, ".github", "workflows", "ci-android.yml")
			writeFixtureFile(t, path, mutateWorkflowSteps(t, readRepositoryWorkflow(t, "ci-android.yml"), mutate))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a source-bound authority outside its exact step identity")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsLegacyBacktickAmbiguity(t *testing.T) {
	commands := map[string]string{
		"variable executable":             "echo `$HELPER attacker.invalid/image:pinned`",
		"braced variable executable":      "echo `${HELPER} attacker.invalid/image:pinned`",
		"env wrapper":                     "echo `env \"$HELPER\" attacker.invalid/image:pinned`",
		"command wrapper":                 "echo `command \"${HELPER}\" attacker.invalid/image:pinned`",
		"interpreter wrapper":             "echo `bash -c '$HELPER attacker.invalid/image:pinned'`",
		"escaped nested backticks":        "echo `echo \\`$HELPER attacker.invalid/image:pinned\\``",
		"escaped nested braced backticks": "echo `echo \\`${HELPER} attacker.invalid/image:pinned\\``",
		"escaped nested backticks CRLF":   "echo `echo \\`\r\n$HELPER attacker.invalid/image:pinned\\``",
		"unbalanced legacy substitution":  "echo `$HELPER attacker.invalid/image:pinned",
		"unbound literal substitution":    "echo `printf safe`",
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			workflow := "name: extra\non: push\njobs:\n  run:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + strconv.Quote(command) + "\n"
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an unauthorised or ambiguous legacy backtick substitution")
			}
		})
	}
}
