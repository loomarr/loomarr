package releaseverify

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyCIContainerDownloadsBindsCacheCleanupWorkflowIdentityAndTrigger(t *testing.T) {
	tests := map[string]func(string) string{
		"changed workflow name": func(source string) string {
			return strings.Replace(source, "name: Cache cleanup", "name: Attacker cleanup", 1)
		},
		"changed pull request type": func(source string) string {
			return strings.Replace(source, "types: [closed]", "types: [opened]", 1)
		},
		"extra pull request type": func(source string) string {
			return strings.Replace(source, "types: [closed]", "types: [closed, reopened]", 1)
		},
		"extra trigger": func(source string) string {
			return strings.Replace(source, "on:\n  pull_request:", "on:\n  push:\n  pull_request:", 1)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, ".github", "workflows", "cache-cleanup.yml")
			writeFixtureFile(t, path, mutate(readRepositoryWorkflow(t, "cache-cleanup.yml")))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted changed cache-cleanup workflow identity or trigger")
			}
		})
	}
}

func TestRepositoryCacheCleanupCoversPullRequestAndMergeQueueRefs(t *testing.T) {
	source := readRepositoryWorkflow(t, "cache-cleanup.yml")
	for _, want := range []string{
		"refs/pull/${{ github.event.pull_request.number }}/merge",
		"refs/heads/gh-readonly-queue/main/pr-${{ github.event.pull_request.number }}-",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("cache cleanup does not cover %s", want)
		}
	}
}
