package releaseverify

import (
	"path/filepath"
	"testing"
)

// The merge queue's required `CI` check waits for the compile job and the four Playwright shards
// (#2147). Each mutation below would let a queued candidate merge without one of them.
func TestVerifyCIContainerDownloadsPinsMergeGate(t *testing.T) {
	t.Parallel()
	gate := readRepositoryWorkflow(t, "merge-gate.yml")
	unchanged := func(value string) string { return value }
	playwrightCondition := "    if: needs.changes.outputs.lane != 'pr-fast' && (needs.changes.outputs.impact_visual == 'true' || needs.changes.outputs.impact_e2e == 'true')\n"
	for name, tc := range map[string]struct {
		mutate  func(string) string
		wantErr bool
	}{
		"the gate as committed":                       {mutate: unchanged},
		"the required check stops waiting on shards":  {mutate: replaceOnce("    needs: [changes, compile, playwright]\n", "    needs: [changes, compile]\n", t), wantErr: true},
		"the required check skips with its gates":     {mutate: replaceOnce("    if: always()\n", "", t), wantErr: true},
		"the required check is renamed":               {mutate: replaceOnce("    name: CI\n", "    name: Merge gate\n", t), wantErr: true},
		"the shards run on another decision":          {mutate: replaceOnce(playwrightCondition, "    if: needs.changes.outputs.impact_e2e == 'true'\n", t), wantErr: true},
		"the shards come from another workflow":       {mutate: replaceOnce("    uses: ./.github/workflows/ci-playwright.yml\n", "    uses: ./.github/workflows/ci-tuner.yml\n", t), wantErr: true},
		"another family joins the gate":               {mutate: replaceOnce("  # The one check the ruleset requires.", "  tuner:\n    name: Tuner — Chromium + Firefox + WebKit\n    needs: changes\n    if: needs.changes.outputs.lane != 'pr-fast' && needs.changes.outputs.impact_tuner == 'true'\n    uses: ./.github/workflows/ci-tuner.yml\n\n  # The one check the ruleset requires.", t), wantErr: true},
		"the compile job runs on another decision":    {mutate: replaceOnce("    if: needs.changes.outputs.go == 'true'\n", "    if: false\n", t), wantErr: true},
		"the compile job stops compiling":             {mutate: replaceOnce("        run: make vet\n", "        run: make fmt\n", t), wantErr: true},
		"the compile job tolerates its own failure":   {mutate: replaceOnce("        run: make vet\n", "        continue-on-error: true\n        run: make vet\n", t), wantErr: true},
		"the selection checkout gains another commit": {mutate: replaceOnce("      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7\n      - name: Select", "      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7\n        with:\n          ref: main\n      - name: Select", t), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "merge-gate.yml"), tc.mutate(gate))
			err := VerifyCIContainerDownloads(root)
			if tc.wantErr && err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a merge gate that can skip a required gate")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("VerifyCIContainerDownloads: %v", err)
			}
		})
	}
}
