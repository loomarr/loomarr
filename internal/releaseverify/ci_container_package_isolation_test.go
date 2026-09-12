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

func TestVerifyCIContainerDownloadsRequiresExactGoShardFlags(t *testing.T) {
	t.Parallel()
	for name, replacement := range map[string]string{
		"package concurrency restored": "GOFLAGS: -p=2",
		"tests filtered out":           "GOFLAGS: -p=1 -run=^$",
		"tool substitution":            "GOFLAGS: -p=1 -toolexec=/attacker",
		"dynamic flags":                "GOFLAGS: ${{ vars.GOFLAGS }}",
		"extra environment":            "GOFLAGS: -p=1\n          MAKEFLAGS: -n",
	} {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			if err := VerifyCIContainerDownloads(root); err != nil {
				t.Fatalf("valid isolated Go shard rejected: %v", err)
			}
			path := filepath.Join(root, ".github", "workflows", "ci-go.yml")
			source := readFixtureFile(t, path)
			const flags = "GOFLAGS: -p=1"
			if !strings.Contains(source, flags) {
				t.Fatal("fixture lacks the isolated Go shard flags")
			}
			writeFixtureFile(t, path, strings.Replace(source, flags, replacement, 1))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("altered Go shard execution contract accepted")
			}
		})
	}
}
