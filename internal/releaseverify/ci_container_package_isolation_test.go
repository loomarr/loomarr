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
