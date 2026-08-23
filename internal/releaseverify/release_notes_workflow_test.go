package releaseverify

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVerifyReleaseNotesWorkflow(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	workflowPath := filepath.Join(filepath.Dir(source), "..", "..", ".github", "workflows", "release-notes.yml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	good := string(data)
	tests := []struct {
		name    string
		mutate  func(string) string
		wantErr bool
	}{
		{name: "validated categorized publication", mutate: func(value string) string { return value }},
		{name: "cannot run on pull requests", mutate: replaceOnce("push:\n    tags:", "pull_request:\n    tags:", t), wantErr: true},
		{name: "cannot gain signing identity", mutate: replaceOnce("  contents: write # create the GitHub Release; nothing else", "  contents: write\n  id-token: write", t), wantErr: true},
		{name: "cannot bypass generator", mutate: replaceOnce(`./scripts/generate-release-notes.sh "$TAG" "$RUNNER_TEMP/release-notes.md"`, `echo invented > "$RUNNER_TEMP/release-notes.md"`, t), wantErr: true},
		{name: "cannot omit release key", mutate: replaceOnce("          OPENROUTER_RELEASE_API_KEY: ${{ secrets.OPENROUTER_RELEASE_API_KEY }}\n", "", t), wantErr: true},
		{name: "cannot use another secret", mutate: replaceOnce("secrets.OPENROUTER_RELEASE_API_KEY", "secrets.OPENROUTER_API_KEY", t), wantErr: true},
		{name: "cannot skip model failure", mutate: replaceOnce("      - name: Generate validated release notes", "      - name: Generate validated release notes\n        continue-on-error: true", t), wantErr: true},
		{name: "cannot restore unvalidated native notes", mutate: replaceOnce(`--notes-file "$RUNNER_TEMP/release-notes.md"`, "--generate-notes", t), wantErr: true},
		{name: "cannot add a bypass job", mutate: replaceOnce("jobs:\n", "jobs:\n  bypass:\n    runs-on: ubuntu-latest\n    steps:\n      - run: gh release create unsafe\n", t), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "release-notes.yml")
			if err := os.WriteFile(path, []byte(tc.mutate(good)), 0o600); err != nil {
				t.Fatal(err)
			}
			err := VerifyReleaseNotesWorkflow(path)
			if tc.wantErr && err == nil {
				t.Fatal("VerifyReleaseNotesWorkflow accepted an unsafe workflow")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("VerifyReleaseNotesWorkflow: %v", err)
			}
		})
	}
}
