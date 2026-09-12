package releaseverify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVerifyCodeQLWorkflow(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(source), "..", "..", ".github", "workflows", "codeql.yml")
	if err := VerifyCodeQLWorkflow(path); err != nil {
		t.Fatalf("repository CodeQL workflow: %v", err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string][2]string{
		"weekly full scan": {`- cron: "17 3 * * 3"`, `- cron: "17 3 * * 4"`},
		"manual full scan": {"  workflow_dispatch:\n", ""},
		"main branch scan": {"    branches: [main]", "    branches: [other]"},
		"language output":  {"${{ steps.impact.outputs.rust }}", "${{ steps.impact.outputs.go }}"},
		"PR cancellation":  {"${{ github.event_name == 'pull_request' }}", "true"},
	}
	for name, replacement := range tests {
		t.Run(name, func(t *testing.T) {
			mutated := strings.Replace(string(original), replacement[0], replacement[1], 1)
			fixture := filepath.Join(t.TempDir(), "codeql.yml")
			if err := os.WriteFile(fixture, []byte(mutated), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := VerifyCodeQLWorkflow(fixture); err == nil {
				t.Fatal("VerifyCodeQLWorkflow accepted a weakened selection or full-scan contract")
			}
		})
	}
}
