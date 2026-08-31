package releaseverify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

func TestRepositoryCIContainerDownloadsAreBounded(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "..", "..")
	if err := VerifyCIContainerDownloads(root); err != nil {
		t.Fatalf("repository CI container downloads: %v", err)
	}
}

func TestTestcontainersRyukImageMatchesBoundedMakeAuthority(t *testing.T) {
	//nolint:staticcheck // The deprecated symbol is the only public accessor for the pinned dependency's Ryuk default.
	if "docker.io/"+testcontainers.ReaperDefaultImage != testcontainersRyukImage {
		t.Fatalf("qualified testcontainers Ryuk default = %q, bounded Make authority = %q",
			"docker.io/"+testcontainers.ReaperDefaultImage, testcontainersRyukImage)
	}
	_, source, _, _ := runtime.Caller(0)
	authority := readFixtureFile(t, filepath.Join(filepath.Dir(source), "..", "testkit", "ryukimage", "image.txt"))
	if strings.TrimSpace(authority) != testcontainersRyukImage {
		t.Fatalf("repository Ryuk authority = %q, want %q", strings.TrimSpace(authority), testcontainersRyukImage)
	}
}

func TestVerifyCIContainerDownloadsRejectsBypasses(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	if err := VerifyCIContainerDownloads(root); err != nil {
		t.Fatalf("complete CI container download policy: %v", err)
	}
	t.Run("missing acquisition helper", func(t *testing.T) {
		helperPath := filepath.Join(root, "scripts", "ensure-container-image.sh")
		helper := readRepositoryContainerImageHelper(t)
		if err := os.Remove(helperPath); err != nil {
			t.Fatal(err)
		}
		if err := VerifyCIContainerDownloads(root); err == nil {
			t.Fatal("VerifyCIContainerDownloads accepted a missing acquisition helper")
		}
		writeFixtureExecutable(t, helperPath, helper)
	})

	frontendPath := filepath.Join(root, "mk", "frontend.mk")
	runnerPath := filepath.Join(root, "scripts", "run-playwright-container.sh")
	storePath := filepath.Join(root, "mk", "store.mk")
	workflowPath := filepath.Join(root, ".github", "workflows", "ci-postgres.yml")
	postgresCallsitePath := filepath.Join(root, "internal", "store", "postgres_test.go")
	frontend := readFixtureFile(t, frontendPath)
	runner := readFixtureFile(t, runnerPath)
	store := readFixtureFile(t, storePath)
	workflow := readFixtureFile(t, workflowPath)
	postgresCallsite := readFixtureFile(t, postgresCallsitePath)

	type fileMutation struct {
		path     string
		contents string
	}
	mutations := map[string]fileMutation{
		"Playwright runner is a no-op": {
			path:     runnerPath,
			contents: "#!/usr/bin/env bash\nexit 0\n",
		},
		"Playwright runner pulls at container start": {
			path:     runnerPath,
			contents: strings.Replace(runner, "run --rm --ipc=host", "run --pull=always --rm --ipc=host", 1),
		},
		"Playwright recipe duplicates image authority": {
			path:     runnerPath,
			contents: strings.Replace(runner, "mcr.microsoft.com/playwright:v1.62.1-noble", "mcr.microsoft.com/playwright:other", 1),
		},
		"commented Playwright recipe is inactive": {
			path:     frontendPath,
			contents: strings.Replace(frontend, "\t./scripts/run-playwright-container.sh ensure", "\t# ./scripts/run-playwright-container.sh ensure", 1),
		},
		"Postgres tests bypass acquisition": {
			path:     storePath,
			contents: strings.Replace(store, "test-pg: rust-dev-build ensure-postgres-test-image", "test-pg: rust-dev-build", 1),
		},
		"Postgres Make module duplicates image authority": {
			path: storePath,
			contents: strings.Replace(store,
				`./scripts/ensure-container-image.sh "$$(cat internal/testkit/postgresimage/image.txt)"`,
				`./scripts/ensure-container-image.sh "postgres:16-alpine"`, 1),
		},
		"Postgres testcontainers call duplicates image authority": {
			path:     postgresCallsitePath,
			contents: strings.Replace(postgresCallsite, "postgresimage.Name()", `"postgres:17-alpine"`, 1),
		},
		"Postgres workflow skips the guarded target": {
			path:     workflowPath,
			contents: strings.Replace(workflow, "make test-pg", "go test ./internal/store", 1),
		},
		"Postgres workflow invokes the guarded target twice": {
			path: workflowPath,
			contents: strings.Replace(
				workflow,
				"      - run: make test-pg\n",
				"      - run: make test-pg\n      - run: make test-pg\n",
				1,
			),
		},
		"Postgres workflow duplicates image authority": {
			path: workflowPath,
			contents: strings.Replace(
				workflow,
				"      - run: make test-pg\n",
				"      - run: docker pull postgres:16-alpine\n      - run: make test-pg\n",
				1,
			),
		},
		"Postgres workflow acquires an unrelated image": {
			path: workflowPath,
			contents: strings.Replace(
				workflow,
				"      - run: make test-pg\n",
				"      - run: docker pull busybox:stable\n      - run: make test-pg\n",
				1,
			),
		},
		"Postgres workflow acquires a service image": {
			path: workflowPath,
			contents: strings.Replace(
				workflow,
				"    runs-on: ubuntu-latest\n",
				"    runs-on: ubuntu-latest\n    services:\n      database:\n        image: postgres:17-alpine\n",
				1,
			),
		},
		"Postgres workflow acquires a job container": {
			path: workflowPath,
			contents: strings.Replace(
				workflow,
				"    runs-on: ubuntu-latest\n",
				"    runs-on: ubuntu-latest\n    container: alpine:3.23\n",
				1,
			),
		},
	}
	for _, runtime := range []string{"docker", "podman", "nerdctl"} {
		for _, operation := range []string{"run", "create", "pull"} {
			command := runtime + " " + operation + " postgres:17-alpine"
			mutations["Postgres workflow uses "+runtime+" "+operation] = fileMutation{
				path: workflowPath,
				contents: strings.Replace(
					workflow,
					"      - run: make test-pg\n",
					"      - run: "+command+"\n      - run: make test-pg\n",
					1,
				),
			}
		}
	}
	for name, command := range map[string]string{
		"docker compose up":        "docker compose up -d",
		"podman compose up":        "podman compose up -d",
		"nerdctl compose up":       "nerdctl compose up -d",
		"docker-compose up":        "docker-compose up -d",
		"acquisition helper":       "./scripts/ensure-container-image.sh postgres:17-alpine",
		"Postgres ensure target":   "make ensure-postgres-test-image",
		"Playwright ensure target": "make ensure-playwright-image",
	} {
		mutations["Postgres workflow uses "+name] = fileMutation{
			path: workflowPath,
			contents: strings.Replace(
				workflow,
				"      - run: make test-pg\n",
				"      - run: "+command+"\n      - run: make test-pg\n",
				1,
			),
		}
	}
	for name, use := range map[string]string{
		"docker protocol action": "docker://postgres:17-alpine",
		"container action":       "docker/setup-buildx-action@0123456789012345678901234567890123456789",
	} {
		mutations["Postgres workflow uses "+name] = fileMutation{
			path: workflowPath,
			contents: strings.Replace(
				workflow,
				"      - run: make test-pg\n",
				"      - uses: "+use+"\n      - run: make test-pg\n",
				1,
			),
		}
	}
	mutations["Postgres workflow acquires from a second job"] = fileMutation{
		path: workflowPath,
		contents: workflow + `  indirect:
    runs-on: ubuntu-latest
    steps:
      - run: docker run postgres:17-alpine
`,
	}
	mutations["Postgres workflow delegates to an unaudited reusable workflow"] = fileMutation{
		path: workflowPath,
		contents: workflow + `  indirect:
    uses: owner/container-acquisition/.github/workflows/pull.yml@0123456789012345678901234567890123456789
`,
	}
	mutations["Postgres workflow invokes test-pg from a second job"] = fileMutation{
		path: workflowPath,
		contents: workflow + `  indirect:
    runs-on: ubuntu-latest
    steps:
      - run: make test-pg
`,
	}
	mutations["Postgres workflow hides acquisition behind shell indirection"] = fileMutation{
		path: workflowPath,
		contents: strings.Replace(
			workflow,
			"      - run: make test-pg\n",
			"      - run: |\n          runtime=docker\n          \"$runtime\" run postgres:17-alpine\n      - run: make test-pg\n",
			1,
		),
	}
	for name, insertion := range map[string]string{
		"job environment":         "    env:\n      MAKEFLAGS: -n\n",
		"job defaults":            "    defaults:\n      run:\n        shell: bash\n",
		"job condition":           "    if: false\n",
		"job error tolerance":     "    continue-on-error: true\n",
		"job shell":               "    shell: bash\n",
		"job working directory":   "    working-directory: internal/store\n",
		"job dry-run MAKEFLAGS":   "    env:\n      MAKEFLAGS: -n\n",
		"unapproved job metadata": "    strategy:\n      fail-fast: false\n",
	} {
		mutations["Postgres workflow rejects "+name] = fileMutation{
			path:     workflowPath,
			contents: strings.Replace(workflow, "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n"+insertion, 1),
		}
	}
	for name, keys := range map[string]string{
		"step environment":       "        env:\n          MAKEFLAGS: -n\n",
		"step condition":         "        if: false\n",
		"step error tolerance":   "        continue-on-error: true\n",
		"step shell":             "        shell: sh\n",
		"step working directory": "        working-directory: internal/store\n",
		"unapproved step key":    "        timeout-minutes: 1\n",
	} {
		mutations["Postgres workflow rejects "+name] = fileMutation{
			path:     workflowPath,
			contents: strings.Replace(workflow, "      - run: make test-pg\n", "      - run: make test-pg\n"+keys, 1),
		}
	}
	for name, replacement := range map[string]string{
		"metadata-only step":  "      - name: skipped test\n",
		"run and action step": "      - run: make test-pg\n        uses: actions/checkout@0123456789012345678901234567890123456789\n",
		"mapping run":         "      - run:\n          command: make test-pg\n",
		"mapping action":      "      - uses:\n          action: actions/checkout@0123456789012345678901234567890123456789\n      - run: make test-pg\n",
		"empty run":           "      - run: \"\"\n",
	} {
		mutations["Postgres workflow rejects "+name] = fileMutation{
			path:     workflowPath,
			contents: strings.Replace(workflow, "      - run: make test-pg\n", replacement, 1),
		}
	}
	mutations["Postgres workflow rejects root dry-run MAKEFLAGS"] = fileMutation{
		path:     workflowPath,
		contents: strings.Replace(workflow, "on:\n  workflow_call:\n", "on:\n  workflow_call:\nenv:\n  MAKEFLAGS: -n\n", 1),
	}
	mutations["Postgres workflow rejects workflow defaults"] = fileMutation{
		path:     workflowPath,
		contents: strings.Replace(workflow, "on:\n  workflow_call:\n", "on:\n  workflow_call:\ndefaults:\n  run:\n    shell: sh\n", 1),
	}
	mutations["Postgres workflow rejects missing workflow name"] = fileMutation{
		path:     workflowPath,
		contents: strings.Replace(workflow, "name: CI / Store conformance (Postgres)\n", "", 1),
	}
	mutations["Postgres workflow rejects malformed trigger"] = fileMutation{
		path:     workflowPath,
		contents: strings.Replace(workflow, "on:\n  workflow_call:\n", "on: {}\n", 1),
	}
	mutations["Postgres Make prerequisite acquires an image indirectly"] = fileMutation{
		path: storePath,
		contents: strings.Replace(store,
			"test-pg: rust-dev-build ensure-postgres-test-image",
			"test-pg: rust-dev-build acquire-indirectly ensure-postgres-test-image\n\nacquire-indirectly: pull-indirectly\n\npull-indirectly:\n\tdocker pull postgres:17-alpine", 1),
	}
	mutations["Playwright audited prerequisite changes to container runtime"] = fileMutation{
		path: frontendPath,
		contents: strings.Replace(frontend,
			"cd $(WEB) && pnpm --filter @loomarr/web build-storybook",
			"docker run --rm busybox:stable true", 1),
	}
	mutations["Postgres protected target pulls directly"] = fileMutation{
		path: storePath,
		contents: strings.Replace(store,
			"test-pg: rust-dev-build ensure-postgres-test-image",
			"test-pg: rust-dev-build ensure-postgres-test-image\n\tdocker pull postgres:17-alpine", 1),
	}
	mutations["Postgres acquisition prerequisite is not phony"] = fileMutation{
		path:     storePath,
		contents: strings.Replace(store, ".PHONY: ensure-postgres-test-image\n", "", 1),
	}
	mutations["Playwright acquisition prerequisite is not phony"] = fileMutation{
		path:     frontendPath,
		contents: strings.Replace(frontend, ".PHONY: ensure-playwright-image\n", "", 1),
	}
	playwrightDependencies := map[string]string{
		"fe-visual":        "storybook-build",
		"fe-visual-update": "storybook-build",
		"e2e":              "fe-build",
		"tuner-e2e":        "fe-build",
		"e2e-update":       "fe-build",
	}
	for target, dependency := range playwrightDependencies {
		mutations["Playwright target "+target+" bypasses acquisition"] = fileMutation{
			path: frontendPath,
			contents: strings.Replace(
				frontend,
				target+": "+dependency+" ensure-playwright-image",
				target+": "+dependency,
				1,
			),
		}
		mutations["Playwright target "+target+" pulls directly"] = fileMutation{
			path: frontendPath,
			contents: strings.Replace(
				frontend,
				target+": "+dependency+" ensure-playwright-image",
				target+": "+dependency+" ensure-playwright-image\n\tdocker pull busybox:stable",
				1,
			),
		}
	}

	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			writeFixtureFile(t, frontendPath, frontend)
			writeFixtureExecutable(t, runnerPath, runner)
			writeFixtureFile(t, storePath, store)
			writeFixtureFile(t, workflowPath, workflow)
			writeFixtureFile(t, postgresCallsitePath, postgresCallsite)
			writeFixtureFile(t, mutation.path, mutation.contents)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a broken acquisition contract")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRequiresBoundedRyukAcquisition(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	storePath := filepath.Join(root, "mk", "store.mk")
	store := readFixtureFile(t, storePath)
	writeFixtureFile(t, storePath, strings.Replace(store,
		"\t./scripts/ensure-container-image.sh \"$$(cat internal/testkit/ryukimage/image.txt)\"\n", "", 1))
	if err := VerifyCIContainerDownloads(root); err == nil {
		t.Fatal("VerifyCIContainerDownloads accepted test-pg without a bounded Ryuk pre-pull")
	}
}

func TestVerifyCIContainerDownloadsRejectsHelperSemanticDrift(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	helperPath := filepath.Join(root, "scripts", "ensure-container-image.sh")
	tests := map[string]string{
		"empty helper": "",
		"no-op helper": "#!/usr/bin/env bash\nexit 0\n",
		"six pull attempts": `#!/usr/bin/env bash
set -u
if docker image inspect "$1" >/dev/null 2>&1; then exit 0; fi
status=1
for attempt in 1 2 3 4 5 6; do
  if docker pull "$1"; then exit 0; else status=$?; fi
  if [[ $attempt -lt 6 ]]; then sleep 2; fi
done
exit "$status"
`,
		"one-second waits":       strings.ReplaceAll(readRepositoryContainerImageHelper(t), "sleep 2", "sleep 1"),
		"ignored sleep failure":  strings.Replace(readRepositoryContainerImageHelper(t), "sleep 2 || exit $?", "sleep 2", 1),
		"discarded final status": strings.Replace(readRepositoryContainerImageHelper(t), `exit "$pull_status"`, "exit 1", 1),
		"hard-coded pull image":  strings.Replace(readRepositoryContainerImageHelper(t), `docker pull "$image"`, "docker pull wrong.invalid/hard-coded:pinned", 1),
	}
	for name, helper := range tests {
		t.Run(name, func(t *testing.T) {
			writeFixtureExecutable(t, helperPath, helper)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted helper behavior outside the bounded contract")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsTransitiveMakeAcquisitionSurfaces(t *testing.T) {
	tests := map[string]string{
		"container runtime":   "docker run --rm busybox:stable true",
		"acquisition helper":  "./scripts/ensure-container-image.sh busybox:stable",
		"ensure target":       "$(MAKE) ensure-playwright-image",
		"service declaration": "docker compose up database",
		"docker action URI":   "echo docker://busybox:stable",
		"alternative runtime": "buildah from busybox:stable",
	}
	for name, recipe := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			storePath := filepath.Join(root, "mk", "store.mk")
			store := readFixtureFile(t, storePath)
			writeFixtureFile(t, storePath, strings.Replace(store,
				"test-pg: rust-dev-build ensure-postgres-test-image",
				"test-pg: rust-dev-build indirect ensure-postgres-test-image\n\nindirect:\n\t"+recipe, 1))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted transitive Make container acquisition")
			}
		})
	}
}
