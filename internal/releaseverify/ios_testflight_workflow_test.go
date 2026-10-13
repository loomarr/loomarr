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
		{name: "protected testflight release on push and dispatch", mutate: func(value string) string { return value }},
		{name: "(a) pull_request_target trigger is rejected", mutate: replaceOnce("on:\n  push:\n", "on:\n  pull_request_target:\n  push:\n", t), wantErr: true},
		{name: "pull_request trigger is rejected", mutate: replaceOnce("on:\n  push:\n", "on:\n  pull_request:\n  push:\n", t), wantErr: true},
		{name: "merge_group trigger is rejected", mutate: replaceOnce("on:\n  push:\n", "on:\n  merge_group:\n  push:\n", t), wantErr: true},
		{name: "push to another branch is rejected", mutate: replaceOnce("    branches: [main]\n", "    branches: [main, release]\n", t), wantErr: true},
		{name: "push on every branch is rejected", mutate: replaceOnce("    branches: [main]\n", "    branches: ['**']\n", t), wantErr: true},
		{name: "push tags are rejected", mutate: replaceOnce("    branches: [main]\n", "    branches: [main]\n    tags: ['v*']\n", t), wantErr: true},
		{name: "push path filter is rejected", mutate: replaceOnce("    branches: [main]\n", "    branches: [main]\n    paths: [web/apps/mobile/**]\n", t), wantErr: true},
		{name: "dispatch-only workflow is rejected", mutate: replaceOnce("  push:\n    branches: [main]\n", "", t), wantErr: true},
		{name: "(b) upload cannot run always", mutate: replaceOnce("        if: needs.plan.outputs.upload == 'true'\n", "        if: always()\n", t), wantErr: true},
		{name: "upload cannot ignore the plan", mutate: replaceOnce("        if: needs.plan.outputs.upload == 'true'\n", "        if: github.event_name == 'push'\n", t), wantErr: true},
		{name: "release cannot ignore the plan", mutate: replaceOnce(" && needs.plan.outputs.release == 'true'\n", "\n", t), wantErr: true},
		{name: "release must run only from main", mutate: replaceOnce("    if: github.ref == 'refs/heads/main' && needs.plan.outputs.release == 'true'\n", "    if: needs.plan.outputs.release == 'true'\n", t), wantErr: true},
		{name: "release must need the plan", mutate: replaceOnce("    needs: plan\n", "", t), wantErr: true},
		{name: "release concurrency cannot be dropped", mutate: replaceOnce("    concurrency:\n      group: ios-testflight-release\n      cancel-in-progress: false\n", "", t), wantErr: true},
		{name: "release concurrency group is fixed", mutate: replaceOnce("      group: ios-testflight-release\n", "      group: ios-testflight-${{ github.run_id }}\n", t), wantErr: true},
		{name: "workflow-level concurrency is rejected", mutate: replaceOnce("permissions:\n  contents: read\n", "concurrency:\n  group: ios-testflight-release\n  cancel-in-progress: false\n\npermissions:\n  contents: read\n", t), wantErr: true},
		{name: "plan job cannot reach the release environment", mutate: replaceOnce("    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n    environment: ios-testflight\n", t), wantErr: true},
		{name: "plan job cannot read a secret", mutate: replaceOnce("          EVENT_NAME: ${{ github.event_name }}\n", "          EVENT_NAME: ${{ github.event_name }}\n          ASC_API_KEY_ID: ${{ secrets.ASC_API_KEY_ID }}\n", t), wantErr: true},
		{name: "plan command cannot interpolate an input", mutate: replaceOnce(`plan "$EVENT_NAME"`, `plan "${{ github.event_name }}"`, t), wantErr: true},
		{name: "plan cannot be replaced by a constant", mutate: replaceOnce("      release: ${{ steps.plan.outputs.release }}\n", "      release: 'true'\n", t), wantErr: true},
		{name: "plan job cannot gain a step", mutate: replaceOnce("      - name: Classify the change and pick the version\n", "      - run: echo release=true >> \"$GITHUB_OUTPUT\"\n      - name: Classify the change and pick the version\n", t), wantErr: true},
		{name: "plan job runs only from main", mutate: replaceOnce("    if: github.ref == 'refs/heads/main'\n    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n", t), wantErr: true},
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
		{name: "secrets cannot reach the build step environment", mutate: replaceOnce("          RELEASE_VERSION: ${{ needs.plan.outputs.version }}\n", "          RELEASE_VERSION: ${{ needs.plan.outputs.version }}\n          ASC_API_KEY_P8_BASE64: ${{ secrets.ASC_API_KEY_P8_BASE64 }}\n", t), wantErr: true},
		{name: "build version comes from the plan", mutate: replaceOnce("          RELEASE_VERSION: ${{ needs.plan.outputs.version }}\n", "          RELEASE_VERSION: ${{ inputs.version }}\n", t), wantErr: true},
		{name: "validation cannot be skipped", mutate: replaceOnce("        run: ./web/scripts/build-ios-testflight.sh validate\n", "        if: false\n        run: ./web/scripts/build-ios-testflight.sh validate\n", t), wantErr: true},
		{name: "mutable actions are rejected", mutate: replaceOnce("actions/setup-node@820762786026740c76f36085b0efc47a31fe5020", "actions/setup-node@v7", t), wantErr: true},
		{name: "plan checkout is pinned", mutate: replaceOnce("actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7\n        with:\n          fetch-depth: 0", "actions/checkout@v7\n        with:\n          fetch-depth: 0", t), wantErr: true},
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
