package releaseverify

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyCIContainerDownloadsRequiresIsolatedCompletePostgresTests(t *testing.T) {
	t.Parallel()
	for name, replacement := range map[string]string{
		"package concurrency restored": "test -p=2 -race",
		"race disabled":                "test -p=1",
		"tests filtered out":           "test -p=1 -race -run=^$",
	} {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			if err := VerifyCIContainerDownloads(root); err != nil {
				t.Fatalf("valid isolated Postgres target rejected: %v", err)
			}
			path := filepath.Join(root, "mk", "store.mk")
			source := readFixtureFile(t, path)
			const command = "test -p=1 -race"
			if !strings.Contains(source, command) {
				t.Fatal("fixture lacks the isolated race command")
			}
			writeFixtureFile(t, path, strings.Replace(source, command, replacement, 1))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("altered Postgres execution contract accepted")
			}
		})
	}
}

// TestVerifyCIContainerDownloadsRequiresEveryPostgresSuiteInTheMatrix: the Postgres gate runs one
// suite per job (#1570), so the matrix is what makes it complete. A suite dropped, duplicated,
// renamed, or cancelled by a sibling's failure must not pass as the whole gate.
func TestVerifyCIContainerDownloadsRequiresEveryPostgresSuiteInTheMatrix(t *testing.T) {
	t.Parallel()
	const matrix = `lane: ["store", "backendtransition", "app"]`
	for name, replacement := range map[string]string{
		"suite dropped":      `lane: ["store", "backendtransition"]`,
		"suite duplicated":   `lane: ["store", "store", "backendtransition", "app"]`,
		"suite renamed":      `lane: ["store", "backendtransition", "api"]`,
		"suites reordered":   `lane: ["app", "store", "backendtransition"]`,
		"fail-fast restored": "fail-fast: true\n      matrix:\n        " + matrix,
	} {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			if err := VerifyCIContainerDownloads(root); err != nil {
				t.Fatalf("valid Postgres suite matrix rejected: %v", err)
			}
			path := filepath.Join(root, ".github", "workflows", "ci-postgres.yml")
			source := readFixtureFile(t, path)
			old := matrix
			if name == "fail-fast restored" {
				old = "fail-fast: false\n      matrix:\n        " + matrix
			}
			if !strings.Contains(source, old) {
				t.Fatalf("fixture lacks %q", old)
			}
			writeFixtureFile(t, path, strings.Replace(source, old, replacement, 1))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("incomplete Postgres suite matrix accepted")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRequiresLaneOwnedGoFlags(t *testing.T) {
	t.Parallel()
	for name, environment := range map[string]string{
		"workflow package concurrency": "GOFLAGS: -p=2",
		"tests filtered out":           "GOFLAGS: -run=^$",
		"tool substitution":            "GOFLAGS: -toolexec=/attacker",
		"dynamic flags":                "GOFLAGS: ${{ vars.GOFLAGS }}",
		"Make dry run":                 "MAKEFLAGS: -n",
	} {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			if err := VerifyCIContainerDownloads(root); err != nil {
				t.Fatalf("valid Go lane rejected: %v", err)
			}
			path := filepath.Join(root, ".github", "workflows", "ci-go.yml")
			source := readFixtureFile(t, path)
			const command = "      - run: make test GO_TEST_LANE=${{ matrix.lane }}"
			if !strings.Contains(source, command) {
				t.Fatal("fixture lacks the Go lane command")
			}
			replacement := command + "\n        env:\n          " + environment
			writeFixtureFile(t, path, strings.Replace(source, command, replacement, 1))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("workflow-controlled Go lane flags accepted")
			}
		})
	}
}
