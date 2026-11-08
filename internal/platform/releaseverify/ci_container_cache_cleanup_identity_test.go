package releaseverify

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyCIContainerDownloadsBindsCacheCleanupWorkflowIdentityAndTrigger(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestVerifyCIContainerDownloadsBindsAndroidCachePromotionToCompletedMergeQueueCI(t *testing.T) {
	t.Parallel()
	tests := map[string]func(string) string{
		"changed workflow name": func(source string) string {
			return strings.Replace(source, "name: Android ccache promotion", "name: Attacker promotion", 1)
		},
		"changed source workflow": func(source string) string {
			return strings.Replace(source, "workflows: [CI]", "workflows: [Attacker]", 1)
		},
		"extra source workflow": func(source string) string {
			return strings.Replace(source, "workflows: [CI]", "workflows: [CI, Attacker]", 1)
		},
		"extra run type": func(source string) string {
			return strings.Replace(source, "types: [completed]", "types: [completed, requested]", 1)
		},
		"extra trigger": func(source string) string {
			return strings.Replace(source, "on:\n  workflow_run:", "on:\n  workflow_dispatch:\n  workflow_run:", 1)
		},
		"push trigger": func(source string) string {
			return strings.Replace(source, "  workflow_run:\n    workflows: [CI]\n    types: [completed]\n", "  push:\n    branches: [main]\n", 1)
		},
		"pull request producer": func(source string) string {
			return strings.Replace(source, "github.event.workflow_run.event == 'merge_group'", "github.event.workflow_run.event == 'pull_request'", 1)
		},
		"failed producer": func(source string) string {
			return strings.Replace(source, " && github.event.workflow_run.conclusion == 'success'", "", 1)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, ".github", "workflows", "android-ccache-promotion.yml")
			writeFixtureFile(t, path, mutate(readRepositoryWorkflow(t, "android-ccache-promotion.yml")))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted changed Android cache promotion identity or trigger")
			}
		})
	}
}
