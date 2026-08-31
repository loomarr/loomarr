package releaseverify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVerifyCIContainerDownloadsRequiresDirectCIBootstrap(t *testing.T) {
	mutations := map[string]func(string) string{
		"Make invocation": func(workflow string) string {
			return strings.Replace(workflow, "go run ./cmd/releaseverify -root .", "make release-verify", 1)
		},
		"conditional direct step": func(workflow string) string {
			return strings.Replace(workflow, "        run: go run ./cmd/releaseverify -root .", "        if: false\n        run: go run ./cmd/releaseverify -root .", 1)
		},
		"error-tolerant direct step": func(workflow string) string {
			return strings.Replace(workflow, "        run: go run ./cmd/releaseverify -root .", "        continue-on-error: true\n        run: go run ./cmd/releaseverify -root .", 1)
		},
		"job-level dry run": func(workflow string) string {
			return insertBeforeCIPolicySteps(workflow, "    env:\n      MAKEFLAGS: -n\n")
		},
		"Make step before direct bootstrap": func(workflow string) string {
			return strings.Replace(workflow,
				"      - name: Repository policy bootstrap outside Make\n        run: go run ./cmd/releaseverify -root .\n",
				"      - run: make release-verify\n      - name: Repository policy bootstrap outside Make\n        run: go run ./cmd/releaseverify -root .\n", 1)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			workflowPath := filepath.Join(root, ".github", "workflows", "ci.yml")
			writeFixtureFile(t, workflowPath, mutate(readFixtureFile(t, workflowPath)))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a Make-skippable CI bootstrap")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsBootstrapInheritedContext(t *testing.T) {
	mutations := map[string]func(string) string{
		"workflow BASH_ENV": func(workflow string) string {
			return strings.Replace(workflow, "jobs:\n", "env:\n  BASH_ENV: attacker.sh\njobs:\n", 1)
		},
		"workflow ENV": func(workflow string) string {
			return strings.Replace(workflow, "jobs:\n", "env:\n  ENV: attacker.sh\njobs:\n", 1)
		},
		"workflow PATH": func(workflow string) string {
			return strings.Replace(workflow, "jobs:\n", "env:\n  PATH: /attacker\njobs:\n", 1)
		},
		"workflow GOENV": func(workflow string) string {
			return strings.Replace(workflow, "jobs:\n", "env:\n  GOENV: /tmp/attacker-goenv\njobs:\n", 1)
		},
		"workflow GOFLAGS": func(workflow string) string {
			return strings.Replace(workflow, "jobs:\n", "env:\n  GOFLAGS: -toolexec=/tmp/attacker\njobs:\n", 1)
		},
		"workflow constructed value": func(workflow string) string {
			return strings.Replace(workflow, "jobs:\n", "env:\n  GO_VERSION: ${{ vars.ATTACKER_VERSION }}\njobs:\n", 1)
		},
		"workflow default shell": func(workflow string) string {
			return strings.Replace(workflow, "jobs:\n", "defaults:\n  run:\n    shell: attacker-shell {0}\njobs:\n", 1)
		},
		"workflow default working directory": func(workflow string) string {
			return strings.Replace(workflow, "jobs:\n", "defaults:\n  run:\n    working-directory: attacker\njobs:\n", 1)
		},
		"bootstrap step environment": func(workflow string) string {
			return strings.Replace(workflow, "        run: go run ./cmd/releaseverify -root .", "        env:\n          PATH: /attacker\n        run: go run ./cmd/releaseverify -root .", 1)
		},
		"bootstrap alternate shell": func(workflow string) string {
			return strings.Replace(workflow, "        run: go run ./cmd/releaseverify -root .", "        shell: attacker-shell {0}\n        run: go run ./cmd/releaseverify -root .", 1)
		},
		"bootstrap alternate working directory": func(workflow string) string {
			return strings.Replace(workflow, "        run: go run ./cmd/releaseverify -root .", "        working-directory: attacker\n        run: go run ./cmd/releaseverify -root .", 1)
		},
		"job PATH": func(workflow string) string {
			return insertBeforeCIPolicySteps(workflow, "    env:\n      PATH: /attacker\n")
		},
		"job default shell": func(workflow string) string {
			return insertBeforeCIPolicySteps(workflow, "    defaults:\n      run:\n        shell: attacker-shell {0}\n")
		},
		"job default working directory": func(workflow string) string {
			return insertBeforeCIPolicySteps(workflow, "    defaults:\n      run:\n        working-directory: attacker\n")
		},
		"bootstrap constructed GOFLAGS": func(workflow string) string {
			return strings.Replace(workflow, "        run: go run ./cmd/releaseverify -root .", "        env:\n          GOFLAGS: ${{ vars.ATTACKER_FLAGS }}\n        run: go run ./cmd/releaseverify -root .", 1)
		},
		"unapproved preceding action": func(workflow string) string {
			return insertAfterCIPolicySteps(workflow, "      - uses: attacker/path-poison@0123456789012345678901234567890123456789\n")
		},
		"preceding action environment": func(workflow string) string {
			return insertAfterCIPolicySteps(workflow, "      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1\n        env:\n          PATH: /attacker\n")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			workflowPath := filepath.Join(root, ".github", "workflows", "ci.yml")
			writeFixtureFile(t, workflowPath, mutate(readFixtureFile(t, workflowPath)))
			if err := VerifyDirectCIPolicyBootstrap(workflowPath); err == nil {
				t.Fatal("VerifyDirectCIPolicyBootstrap accepted a poisoned inherited execution context")
			}
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a poisoned direct-bootstrap execution context")
			}
		})
	}
}

const ciPolicyStepsAnchor = "    if: needs.changes.outputs.impact_policy == 'true'\n    runs-on: ubuntu-latest\n    steps:\n"

func insertBeforeCIPolicySteps(workflow, insertion string) string {
	return strings.Replace(workflow, ciPolicyStepsAnchor, strings.TrimSuffix(ciPolicyStepsAnchor, "    steps:\n")+insertion+"    steps:\n", 1)
}

func insertAfterCIPolicySteps(workflow, insertion string) string {
	return strings.Replace(workflow, ciPolicyStepsAnchor, ciPolicyStepsAnchor+insertion, 1)
}

func writeCIContainerDownloadsFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, "Makefile"), "GO ?= go\nCARGO ?= cargo\ninclude mk/store.mk\ninclude mk/frontend.mk\n")
	writeFixtureExecutable(t, filepath.Join(root, "scripts", "ensure-container-image.sh"), readRepositoryContainerImageHelper(t))
	writeFixtureExecutable(t, filepath.Join(root, "scripts", "run-playwright-container.sh"), readRepositoryPlaywrightContainerRunner(t))
	writeFixtureFile(t, filepath.Join(root, "internal", "testkit", "postgresimage", "image.txt"), "library/postgres:16-alpine\n")
	writeFixtureFile(t, filepath.Join(root, "internal", "testkit", "postgresimage", "image.go"), `// Package postgresimage owns the single image reference used by Postgres
// testcontainers and the Make pre-pull that runs before those tests.
package postgresimage

import (
	_ "embed"
	"strings"
)

//go:embed image.txt
var image string

// Name returns the pinned Postgres test image from the Make-readable authority.
func Name() string {
	return strings.TrimSpace(image)
}
`)
	writeFixtureFile(t, filepath.Join(root, "internal", "testkit", "ryukimage", "image.txt"), "docker.io/testcontainers/ryuk:0.14.0\n")
	writeFixtureFile(t, filepath.Join(root, "internal", "store", "postgres_test.go"), `package store

import (
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func startPostgres() {
	_, _ = postgres.Run(nil, postgresimage.Name())
}
`)
	writeFixtureFile(t, filepath.Join(root, "mk", "frontend.mk"), `unexport PW_DOCKER_USER PW_CI PW_REAL_CI PW_IMAGE PW_SHARD

.PHONY: ensure-playwright-image
ensure-playwright-image:
	./scripts/run-playwright-container.sh ensure

.PHONY: fe-visual fe-visual-update e2e tuner-e2e e2e-update
storybook-build:
	cd $(WEB) && pnpm --filter @loomarr/web build-storybook
fe-build:
	cd $(WEB) && pnpm codegen && pnpm --filter @loomarr/web build
fe-visual: storybook-build ensure-playwright-image
	./scripts/run-playwright-container.sh visual
fe-visual-update: storybook-build ensure-playwright-image
	./scripts/run-playwright-container.sh visual-update
e2e: fe-build ensure-playwright-image
	./scripts/run-playwright-container.sh e2e
tuner-e2e: fe-build ensure-playwright-image
	./scripts/run-playwright-container.sh tuner-e2e
e2e-update: fe-build ensure-playwright-image
	./scripts/run-playwright-container.sh e2e-update
`)
	writeFixtureFile(t, filepath.Join(root, "mk", "store.mk"), `.PHONY: ensure-postgres-test-image
ensure-postgres-test-image:
	./scripts/ensure-container-image.sh "$$(cat internal/testkit/postgresimage/image.txt)"
	./scripts/ensure-container-image.sh "$$(cat internal/testkit/ryukimage/image.txt)"

.PHONY: test-pg
rust-dev-build:
	LOOMARR_RELEASE=dev $(CARGO) build --locked -p loomarr-image
test-pg: rust-dev-build ensure-postgres-test-image
	TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX=docker.io TESTCONTAINERS_RYUK_DISABLED=false $(GO) test -race -tags=integration -timeout=20m ./internal/store/ ./internal/backendtransition/ ./internal/app/
`)
	_, source, _, _ := runtime.Caller(0)
	workflowEntries, err := os.ReadDir(filepath.Join(filepath.Dir(source), "..", "..", ".github", "workflows"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range workflowEntries {
		if entry.IsDir() || (filepath.Ext(entry.Name()) != ".yml" && filepath.Ext(entry.Name()) != ".yaml") {
			continue
		}
		workflow := entry.Name()
		writeFixtureFile(t, filepath.Join(root, ".github", "workflows", workflow), readRepositoryWorkflow(t, workflow))
	}
	return root
}

func readRepositoryContainerImageHelper(t *testing.T) string {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	return readFixtureFile(t, filepath.Join(filepath.Dir(source), "..", "..", "scripts", "ensure-container-image.sh"))
}

func readRepositoryPlaywrightContainerRunner(t *testing.T) string {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	return readFixtureFile(t, filepath.Join(filepath.Dir(source), "..", "..", "scripts", "run-playwright-container.sh"))
}

func readRepositoryWorkflow(t *testing.T, name string) string {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	return readFixtureFile(t, filepath.Join(filepath.Dir(source), "..", "..", ".github", "workflows", name))
}

func readFixtureFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func writeFixtureFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeFixtureExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
}
