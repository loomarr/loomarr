package releaseverify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyIOSTestFlightWorkflow(t *testing.T) {
	t.Parallel()
	good := readRepositoryWorkflow(t, "ios-testflight.yml")
	tests := []struct {
		name    string
		mutate  func(string) string
		wantErr bool
	}{
		{name: "protected manual testflight release", mutate: func(value string) string { return value }},
		{name: "(a) pull_request_target and push triggers are rejected", mutate: replaceOnce("on:\n  workflow_dispatch:\n", "on:\n  pull_request_target:\n  push:\n    branches: [main]\n  workflow_dispatch:\n", t), wantErr: true},
		{name: "(b) upload cannot run always", mutate: replaceOnce("        if: inputs.upload\n", "        if: always()\n", t), wantErr: true},
		{name: "(c) inputs cannot be interpolated into a script", mutate: replaceOnce(`build "$RELEASE_VERSION"`, `build "${{ inputs.version }}"`, t), wantErr: true},
		{name: "(d) the key cannot be read before upload", mutate: replaceOnce("        run: ./web/scripts/build-ios-testflight.sh upload\n", "        run: cat \"$LOOMARR_ASC_KEY_PATH\" && ./web/scripts/build-ios-testflight.sh upload\n", t), wantErr: true},
		{name: "(e) credential cleanup cannot be replaced", mutate: replaceOnce("        run: rm -rf -- \"$RUNNER_TEMP/asc\"\n", "        run: \"true\"\n", t), wantErr: true},
		{name: "credential cleanup cannot be conditional", mutate: replaceOnce("        if: always()\n", "        if: success()\n", t), wantErr: true},
		{name: "upload must default to false", mutate: replaceOnce("        default: false\n", "        default: true\n", t), wantErr: true},
		{name: "no extra dispatch input", mutate: replaceOnce("      upload:\n", "      extra:\n        type: string\n      upload:\n", t), wantErr: true},
		{name: "protected environment is fixed", mutate: replaceOnce("    environment: ios-testflight\n", "    environment: unprotected\n", t), wantErr: true},
		{name: "non-main dispatch is rejected", mutate: replaceOnce("    if: github.ref == 'refs/heads/main'\n", "    if: true\n", t), wantErr: true},
		{name: "workflow permissions stay read only", mutate: replaceOnce("  contents: read\n", "  contents: write\n", t), wantErr: true},
		{name: "runs serialize without cancellation", mutate: replaceOnce("  cancel-in-progress: false\n", "  cancel-in-progress: true\n", t), wantErr: true},
		{name: "secrets cannot reach the build step environment", mutate: replaceOnce("          RELEASE_VERSION: ${{ inputs.version }}\n", "          RELEASE_VERSION: ${{ inputs.version }}\n          ASC_API_KEY_P8_BASE64: ${{ secrets.ASC_API_KEY_P8_BASE64 }}\n", t), wantErr: true},
		{name: "validation cannot be skipped", mutate: replaceOnce("        run: ./web/scripts/build-ios-testflight.sh validate\n", "        if: false\n        run: ./web/scripts/build-ios-testflight.sh validate\n", t), wantErr: true},
		{name: "mutable actions are rejected", mutate: replaceOnce("actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1", "actions/checkout@v7", t), wantErr: true},
		{name: "second job is rejected", mutate: replaceOnce("jobs:\n", "jobs:\n  bypass:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo bypass\n", t), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "ios-testflight.yml")
			if err := os.WriteFile(path, []byte(tc.mutate(good)), 0o600); err != nil {
				t.Fatal(err)
			}
			err := VerifyIOSTestFlightWorkflow(path)
			if tc.wantErr && err == nil {
				t.Fatal("VerifyIOSTestFlightWorkflow accepted an unsafe workflow")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("VerifyIOSTestFlightWorkflow: %v", err)
			}
		})
	}
}
