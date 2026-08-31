package releaseverify

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyCIContainerDownloadsRejectsProtectedWorkflowMakeControls(t *testing.T) {
	type workflowMutation struct {
		workflow string
		command  string
	}
	base := `name: CI / Playwright
on: push
jobs:
  run:
    runs-on: ubuntu-latest
    steps:
      - run: make e2e
`
	tests := map[string]workflowMutation{
		"workflow MAKEFLAGS dry-run": {
			workflow: strings.Replace(base, "on: push\n", "on: push\nenv:\n  MAKEFLAGS: -n\n", 1),
		},
		"workflow GNUMAKEFLAGS touch": {
			workflow: strings.Replace(base, "on: push\n", "on: push\nenv:\n  GNUMAKEFLAGS: --touch\n", 1),
		},
		"workflow MFLAGS question": {
			workflow: strings.Replace(base, "on: push\n", "on: push\nenv:\n  MFLAGS: --question\n", 1),
		},
		"job MAKEFLAGS old-file": {
			workflow: strings.Replace(base, "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n    env:\n      MAKEFLAGS: --old-file=ensure-playwright-image\n", 1),
		},
		"job GNUMAKEFLAGS alternate Makefile": {
			workflow: strings.Replace(base, "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n    env:\n      GNUMAKEFLAGS: --makefile=attacker.mk\n", 1),
		},
		"step MFLAGS eval": {
			workflow: strings.Replace(base, "      - run: make e2e\n", "      - run: make e2e\n        env:\n          MFLAGS: --eval=e2e:\n", 1),
		},
		"step MAKEFLAGS include": {
			workflow: strings.Replace(base, "      - run: make e2e\n", "      - run: make e2e\n        env:\n          MAKEFLAGS: --include-dir=attacker\n", 1),
		},
		"recursive environment value": {
			workflow: strings.Replace(base, "on: push\n", "on: push\nenv:\n  ATTACK_FLAGS: -n\n  MAKEFLAGS: ${{ env.ATTACK_FLAGS }}\n", 1),
		},
		"workflow BASH_ENV redirect": {
			workflow: strings.Replace(base, "on: push\n", "on: push\nenv:\n  BASH_ENV: attacker.sh\n", 1),
		},
		"job PATH redirect": {
			workflow: strings.Replace(base, "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n    env:\n      PATH: /attacker\n", 1),
		},
		"step ENV redirect": {
			workflow: strings.Replace(base, "      - run: make e2e\n", "      - run: make e2e\n        env:\n          ENV: attacker.sh\n", 1),
		},
		"workflow alternate shell": {
			workflow: strings.Replace(base, "on: push\n", "on: push\ndefaults:\n  run:\n    shell: attacker-shell {0}\n", 1),
		},
		"job alternate working directory": {
			workflow: strings.Replace(base, "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n    defaults:\n      run:\n        working-directory: attacker\n", 1),
		},
		"step alternate shell": {
			workflow: strings.Replace(base, "      - run: make e2e\n", "      - run: make e2e\n        shell: attacker-shell {0}\n", 1),
		},
		"step alternate working directory": {
			workflow: strings.Replace(base, "      - run: make e2e\n", "      - run: make e2e\n        working-directory: attacker\n", 1),
		},
		"protected command outside jobs": {
			workflow: "name: CI / Playwright\non: push\nrun: make e2e\njobs: {}\n",
		},
		"protected command in malformed steps": {
			workflow: "name: CI / Playwright\non: push\njobs:\n  run:\n    runs-on: ubuntu-latest\n    steps:\n      run: make e2e\n",
		},
		"command dry-run":                  {command: "make -n e2e"},
		"command touch":                    {command: "make --touch e2e"},
		"command question":                 {command: "make --question e2e"},
		"command old-file":                 {command: "make --old-file=ensure-playwright-image e2e"},
		"command alternate Makefile":       {command: "make --makefile=attacker.mk e2e"},
		"command eval":                     {command: "make --eval='e2e:' e2e"},
		"command include":                  {command: "make --include-dir=attacker e2e"},
		"command recursive MAKEFLAGS":      {command: "MAKEFLAGS='${ATTACK_FLAGS}' make e2e"},
		"command constructed GNUMAKEFLAGS": {command: "GNUMAKEFLAGS=\"${LEFT}${RIGHT}\" make e2e"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			workflow := test.workflow
			if workflow == "" {
				workflow = strings.Replace(base, "make e2e", test.command, 1)
			}
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", playwrightWorkflowName), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted unsafe Make execution controls around a protected workflow route")
			}
		})
	}
}
