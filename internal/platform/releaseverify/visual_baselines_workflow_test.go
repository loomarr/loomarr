package releaseverify

import (
	"os"
	"path/filepath"
	"testing"
)

type visualBaselinesMutation struct {
	name    string
	mutate  func(string) string
	wantErr bool
}

func runVisualBaselinesMutations(t *testing.T, file string, verify func(string) error, tests []visualBaselinesMutation) {
	t.Helper()
	good := readRepositoryWorkflow(t, file)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), file)
			if err := os.WriteFile(path, []byte(tc.mutate(good)), 0o600); err != nil {
				t.Fatal(err)
			}
			err := verify(path)
			if tc.wantErr && err == nil {
				t.Fatalf("%s verifier accepted an unsafe workflow", file)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("%s verifier: %v", file, err)
			}
		})
	}
}

func TestVerifyVisualBaselinesWorkflow(t *testing.T) {
	t.Parallel()
	pushLine := `git push origin "HEAD:refs/heads/$BRANCH"`
	unchanged := func(value string) string { return value }
	runVisualBaselinesMutations(t, "visual-baselines.yml", VerifyVisualBaselinesWorkflow, []visualBaselinesMutation{
		{name: "baseline regeneration for one pull request, on a push or by hand", mutate: unchanged},
		{name: "pull_request_target trigger is rejected", mutate: replaceOnce("on:\n  pull_request:\n", "on:\n  pull_request_target:\n", t), wantErr: true},
		{name: "push trigger is rejected", mutate: replaceOnce("on:\n  pull_request:\n", "on:\n  push:\n  pull_request:\n", t), wantErr: true},
		{name: "merge_group trigger is rejected", mutate: replaceOnce("on:\n  pull_request:\n", "on:\n  merge_group:\n  pull_request:\n", t), wantErr: true},
		{name: "the dispatch cannot be dropped", mutate: replaceOnce("  workflow_dispatch:\n    inputs:\n", "  workflow_call:\n    inputs:\n", t), wantErr: true},
		{name: "pull_request types stay opened, synchronize, reopened", mutate: replaceOnce("    types: [opened, synchronize, reopened]\n", "    types: [opened, synchronize, reopened, labeled]\n", t), wantErr: true},
		{name: "pull_request paths stay web/", mutate: replaceOnce(`      - "web/**"`+"\n", `      - "**"`+"\n", t), wantErr: true},
		{name: "pull_request paths cannot be dropped", mutate: replaceOnce("    paths:\n      - \"web/**\"\n", "", t), wantErr: true},
		{name: "resolve keeps its fork and bot condition", mutate: replaceOnce("    if: "+visualBaselinesResolveCondition+"\n", "", t), wantErr: true},
		{name: "resolve cannot admit a fork", mutate: replaceOnce("github.event.pull_request.head.repo.full_name == github.repository && ", "", t), wantErr: true},
		{name: "resolve cannot admit the bot's own commit", mutate: replaceOnce(" && github.actor != 'github-actions[bot]'", "", t), wantErr: true},
		{name: "render runs only when the change renders", mutate: replaceOnce("    if: needs.resolve.outputs.render == 'true'\n", "", t), wantErr: true},
		{name: "classification cannot be skipped", mutate: replaceOnce(`      if ! grep -qxE 'visual=true|e2e=true' "$RUNNER_TEMP/impact.txt"; then`, `      if true; then`, t), wantErr: true},
		{name: "the pull request input is required", mutate: replaceOnce("        required: true\n", "        required: false\n", t), wantErr: true},
		{name: "no extra dispatch input", mutate: replaceOnce("      pr:\n", "      branch:\n        type: string\n      pr:\n", t), wantErr: true},
		{name: "the force input stays optional", mutate: replaceOnce("        required: false\n", "        required: true\n", t), wantErr: true},
		{name: "the force input defaults to a normal update", mutate: replaceOnce(`        default: ""`+"\n", `        default: ".*"`+"\n", t), wantErr: true},
		{name: "the force input cannot lose its default", mutate: replaceOnce(`        default: ""`+"\n", "", t), wantErr: true},
		{name: "force cannot be interpolated into the dispatch", mutate: replaceOnce(`inputs+=(-f force="$FORCE")`, `inputs+=(-f force="${{ inputs.force }}")`, t), wantErr: true},
		{name: "force reaches the render only when set", mutate: replaceOnce(`  if [ -n "$FORCE" ]; then`+"\n            inputs+=", `  if true; then`+"\n            inputs+=", t), wantErr: true},
		{name: "workflow permissions stay read only", mutate: replaceOnce("\npermissions:\n  contents: read\n", "\npermissions:\n  contents: write\n", t), wantErr: true},
		{name: "no workflow environment for branch tooling", mutate: replaceOnce("\njobs:\n", "\nenv:\n  NODE_VERSION: \"22\"\n\njobs:\n", t), wantErr: true},
		{name: "only a push cancels an older render", mutate: replaceOnce("  cancel-in-progress: ${{ github.event_name == 'pull_request' }}\n", "  cancel-in-progress: true\n", t), wantErr: true},
		{name: "concurrency stays per pull request", mutate: replaceOnce("  group: visual-baselines-${{ inputs.pr || github.event.pull_request.number }}", "  group: visual-baselines-${{ github.run_id }}", t), wantErr: true},
		{name: "the bot's commit keeps a group of its own", mutate: replaceOnce("${{ github.actor == 'github-actions[bot]' && '-bot' || '' }}\n", "\n", t), wantErr: true},
		{name: "resolve job cannot write", mutate: replaceOnce("      contents: read\n      pull-requests: read\n", "      contents: write\n      pull-requests: read\n", t), wantErr: true},
		{name: "dispatch must come from the default branch", mutate: replaceOnce(`if [ "$EVENT" = workflow_dispatch ] && [ "$REF" != "refs/heads/$DEFAULT_BRANCH" ]; then`, `if false; then`, t), wantErr: true},
		{name: "render job cannot write contents", mutate: replaceOnce("      actions: write\n      contents: read\n    outputs:\n      run_id:", "      actions: write\n      contents: write\n    outputs:\n      run_id:", t), wantErr: true},
		{name: "render job cannot run the branch's code", mutate: replaceOnce("      - name: Dispatch the render and wait for it\n", "      - run: make fe-install\n      - name: Dispatch the render and wait for it\n", t), wantErr: true},
		{name: "render job cannot dispatch at another ref", mutate: replaceOnce(`--ref "$BRANCH" "${inputs[@]}"`, `--ref main "${inputs[@]}"`, t), wantErr: true},
		{name: "render job cannot accept a failed render", mutate: replaceOnce(`if [ "$conclusion" != success ]; then`, `if false; then`, t), wantErr: true},
		{name: "render run is found by its request, not by recency", mutate: replaceOnce(`'map(select(.displayTitle == $title)) | .[0].databaseId // ""'`, `'.[0].databaseId // ""'`, t), wantErr: true},
		{name: "commit job cannot dispatch workflows", mutate: replaceOnce("      actions: read\n      contents: write\n", "      actions: write\n      contents: write\n", t), wantErr: true},
		{name: "commit job cannot skip verifying the render run", mutate: replaceOnce("        and .head_branch == $branch and .head_sha == $sha\n", "", t), wantErr: true},
		{name: "commit job cannot drop the render run check", mutate: replaceOnce("      - name: Verify the render run\n", "      - name: Verify the render run\n        continue-on-error: true\n", t), wantErr: true},
		{name: "release job cannot write contents", mutate: replaceOnce("      actions: write\n      contents: read\n    steps:\n      # The push above", "      actions: write\n      contents: write\n    steps:\n      # The push above", t), wantErr: true},
		{name: "release job runs only after a push", mutate: replaceOnce("    if: needs.commit.outputs.pushed == 'true'\n", "", t), wantErr: true},
		{name: "release approves only the pushed commit's runs", mutate: replaceOnce("              and .head_sha == $sha and .repository.full_name == $repo\n", "              and .repository.full_name == $repo\n", t), wantErr: true},
		{name: "release approves only the bot's runs", mutate: replaceOnce(`              and .triggering_actor.login == "github-actions[bot]")`, `              and true)`, t), wantErr: true},
		{name: "release approves only held runs", mutate: replaceOnce(`and .conclusion == "action_required"`, `and .status == "completed"`, t), wantErr: true},
		{name: "a failed approval fails the run", mutate: replaceOnce(`          if [ "$failed" -ne 0 ]; then`+"\n            exit 1\n", `          if [ "$failed" -ne 0 ]; then`+"\n            :\n", t), wantErr: true},
		{name: "commit allow-list stays visual and e2e snapshots", mutate: replaceOnce(`allowed='^web/apps/web/tests/(visual|e2e)/`, `allowed='^web/apps/web/tests/`, t), wantErr: true},
		{name: "commit job cannot be skipped by a condition", mutate: replaceOnce("    needs: [resolve, render]\n", "    needs: [resolve, render]\n    if: always()\n", t), wantErr: true},
		{name: "input cannot be interpolated into a script", mutate: replaceOnce(`gh pr view "$PR"`, `gh pr view "${{ inputs.pr }}"`, t), wantErr: true},
		{name: "branch cannot be interpolated into the push", mutate: replaceOnce(pushLine, `git push origin "HEAD:refs/heads/${{ needs.resolve.outputs.branch }}"`, t), wantErr: true},
		{name: "push cannot be forced", mutate: replaceOnce(pushLine, `git push --force origin "HEAD:refs/heads/$BRANCH"`, t), wantErr: true},
		{name: "fork guard cannot be dropped", mutate: replaceOnce(`fork branches are refused"`+"\n            exit 1\n", `fork branches are refused"`+"\n", t), wantErr: true},
		{name: "merge queue guard cannot be dropped", mutate: replaceOnce(`  if [ "$queued" != false ]; then`, `  if false; then`, t), wantErr: true},
		{name: "baselines cannot be written through the worktree", mutate: replaceOnce(`git update-index --add --cacheinfo "100644,$blob,$path"`, `cp -- "$in/files/$path" "$path" && git add -- "$path"`, t), wantErr: true},
		{name: "a tracked symlink cannot be replaced", mutate: replaceOnce(`  if [ -n "$mode" ] && [ "$mode" != 100644 ]; then`, `  if false; then`, t), wantErr: true},
		{name: "commit step environment cannot gain a token", mutate: replaceOnce("          FORCE: ${{ inputs.force }}\n        run: |\n          set -euo pipefail\n          in=", "          FORCE: ${{ inputs.force }}\n          GH_TOKEN: ${{ github.token }}\n        run: |\n          set -euo pipefail\n          in=", t), wantErr: true},
		{name: "dispatched CI stays the full scope", mutate: replaceOnce("-f scope=full", "-f scope=release-candidate", t), wantErr: true},
		{name: "mutable actions are rejected", mutate: replaceOnce("actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333", "actions/download-artifact@v8", t), wantErr: true},
		{name: "outputs cannot be replaced by a constant", mutate: replaceOnce("      pushed: ${{ steps.commit.outputs.pushed }}\n", "      pushed: 'true'\n", t), wantErr: true},
		{name: "another job is rejected", mutate: replaceOnce("jobs:\n", "jobs:\n  bypass:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo bypass\n", t), wantErr: true},
	})
}

func TestVerifyVisualBaselinesRenderWorkflow(t *testing.T) {
	t.Parallel()
	unchanged := func(value string) string { return value }
	runVisualBaselinesMutations(t, "visual-baselines-render.yml", VerifyVisualBaselinesRenderWorkflow, []visualBaselinesMutation{
		{name: "read-only render at a pull request's branch", mutate: unchanged},
		{name: "pull_request trigger is rejected", mutate: replaceOnce("on:\n  workflow_dispatch:\n", "on:\n  pull_request:\n  workflow_dispatch:\n", t), wantErr: true},
		{name: "push trigger is rejected", mutate: replaceOnce("on:\n  workflow_dispatch:\n", "on:\n  push:\n  workflow_dispatch:\n", t), wantErr: true},
		{name: "the run is named after its request", mutate: replaceOnce("run-name: Render visual baselines ${{ inputs.request }}\n", "run-name: Render visual baselines\n", t), wantErr: true},
		{name: "the run name cannot be dropped", mutate: replaceOnce("run-name: Render visual baselines ${{ inputs.request }}\n", "", t), wantErr: true},
		{name: "the commit input is required", mutate: replaceOnce("still be at it\n        required: true\n", "still be at it\n        required: false\n", t), wantErr: true},
		{name: "no extra dispatch input", mutate: replaceOnce("      sha:\n", "      ref:\n        type: string\n      sha:\n", t), wantErr: true},
		{name: "the force input stays optional", mutate: replaceOnce("        required: false\n", "        required: true\n", t), wantErr: true},
		{name: "the force input defaults to a normal update", mutate: replaceOnce(`        default: ""`+"\n", `        default: ".*"`+"\n", t), wantErr: true},
		{name: "force reaches the update only through env", mutate: replaceOnce("        env:\n          LOOMARR_PLAYWRIGHT_FORCE: ${{ inputs.force }}\n        run: make fe-visual-update\n", "        run: LOOMARR_PLAYWRIGHT_FORCE='${{ inputs.force }}' make fe-visual-update\n", t), wantErr: true},
		{name: "force cannot be dropped from the update", mutate: replaceOnce("        env:\n          LOOMARR_PLAYWRIGHT_FORCE: ${{ inputs.force }}\n", "", t), wantErr: true},
		{name: "workflow permissions stay read only", mutate: replaceOnce("\npermissions:\n  contents: read\n", "\npermissions:\n  contents: write\n", t), wantErr: true},
		{name: "render job cannot write", mutate: replaceOnce("    permissions:\n      contents: read\n", "    permissions:\n      contents: write\n", t), wantErr: true},
		{name: "render job cannot gain another permission", mutate: replaceOnce("    permissions:\n      contents: read\n", "    permissions:\n      contents: read\n      actions: write\n", t), wantErr: true},
		{name: "the default branch guard cannot be dropped", mutate: replaceOnce(`if [[ $REF != refs/heads/* ]] || [ "$REF" = "refs/heads/$DEFAULT_BRANCH" ]; then`, `if false; then`, t), wantErr: true},
		{name: "the resolved commit guard cannot be dropped", mutate: replaceOnce(`if [ "$SHA" != "$WANT_SHA" ]; then`, `if false; then`, t), wantErr: true},
		{name: "the guard runs before the branch is fetched", mutate: replaceOnce("      - name: Refuse the default branch\n", "      - run: corepack enable\n      - name: Refuse the default branch\n", t), wantErr: true},
		{name: "the guard cannot tolerate its own failure", mutate: replaceOnce("      - name: Refuse the default branch\n", "      - name: Refuse the default branch\n        continue-on-error: true\n", t), wantErr: true},
		{name: "the guard reads the dispatched commit", mutate: replaceOnce("          SHA: ${{ github.sha }}\n", "          SHA: ${{ inputs.sha }}\n", t), wantErr: true},
		{name: "input cannot be interpolated into a script", mutate: replaceOnce(`if [ "$SHA" != "$WANT_SHA" ]; then`, `if [ "$SHA" != "${{ inputs.sha }}" ]; then`, t), wantErr: true},
		{name: "collection cannot accept any changed file", mutate: replaceOnce(`  if [[ ($status == ' M' || $status == '??') && $path =~ $allowed ]]; then`, `  if true; then`, t), wantErr: true},
		{name: "update step cannot be skipped", mutate: replaceOnce("        run: make fe-visual-update\n", "        if: false\n        run: make fe-visual-update\n", t), wantErr: true},
		{name: "update step cannot tolerate its own failure", mutate: replaceOnce("        run: make fe-visual-update\n", "        continue-on-error: true\n        run: make fe-visual-update\n", t), wantErr: true},
		{name: "render job cannot gain a step", mutate: replaceOnce("      - run: corepack enable\n", "      - run: corepack enable\n      - run: git config --global user.name bot\n", t), wantErr: true},
		{name: "the e2e update cannot be dropped", mutate: replaceOnce("      - name: "+visualBaselinesE2EUpdateStepName+"\n        run: make e2e-update\n", "", t), wantErr: true},
		{name: "the e2e update cannot tolerate its own failure", mutate: replaceOnce("        run: make e2e-update\n", "        continue-on-error: true\n        run: make e2e-update\n", t), wantErr: true},
		{name: "collection allow-list stays visual and e2e snapshots", mutate: replaceOnce(`allowed='^web/apps/web/tests/(visual|e2e)/`, `allowed='^web/apps/web/tests/`, t), wantErr: true},
		{name: "failure images upload only on failure", mutate: replaceOnce("        if: failure()\n", "        if: always()\n", t), wantErr: true},
		{name: "a newer render of the branch cancels the older", mutate: replaceOnce("  cancel-in-progress: true\n", "  cancel-in-progress: false\n", t), wantErr: true},
		{name: "render concurrency stays per branch", mutate: replaceOnce("  group: visual-baselines-render-${{ github.ref }}\n", "  group: visual-baselines-render\n", t), wantErr: true},
		{name: "mutable actions are rejected", mutate: replaceOnce("actions/setup-node@949feb2413d6458794dcd2491c4babbbce0c15c1", "actions/setup-node@v7", t), wantErr: true},
		{name: "another job is rejected", mutate: replaceOnce("jobs:\n", "jobs:\n  bypass:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo bypass\n", t), wantErr: true},
	})
}

// The checkout and artifact inputs are pinned by the shared action catalog, not the workflows' own
// verifiers: the render keeps no token and renders the commit it was dispatched at, the push job
// checks out the branch it resolved, and it reads only the render run's artifact.
func TestVerifyCIContainerDownloadsPinsVisualBaselinesCheckouts(t *testing.T) {
	t.Parallel()
	dispatcher := readRepositoryWorkflow(t, "visual-baselines.yml")
	render := readRepositoryWorkflow(t, "visual-baselines-render.yml")
	unchanged := func(value string) string { return value }
	for name, tc := range map[string]struct {
		dispatcher, render func(string) string
		wantErr            bool
	}{
		"both workflows as committed":            {dispatcher: unchanged, render: unchanged},
		"render checkout persists the token":     {dispatcher: unchanged, render: replaceOnce("          persist-credentials: false\n", "          persist-credentials: true\n", t), wantErr: true},
		"render checkout picks another commit":   {dispatcher: unchanged, render: replaceOnce("          persist-credentials: false\n", "          persist-credentials: false\n          ref: main\n", t), wantErr: true},
		"render artifact is kept longer":         {dispatcher: unchanged, render: replaceOnce("          retention-days: 1\n", "          retention-days: 90\n", t), wantErr: true},
		"classifier checkout persists the token": {dispatcher: replaceOnce("          persist-credentials: false\n", "          persist-credentials: true\n", t), render: unchanged, wantErr: true},
		"failure images upload unconditionally":  {dispatcher: unchanged, render: replaceOnce("        if: failure()\n", "", t), wantErr: true},
		"push checkout picks another branch":     {dispatcher: replaceOnce("          ref: ${{ needs.resolve.outputs.branch }}\n", "          ref: main\n", t), render: unchanged, wantErr: true},
		"push job reads another run's artifact":  {dispatcher: replaceOnce("          run-id: ${{ needs.render.outputs.run_id }}\n", "          run-id: ${{ github.run_id }}\n", t), render: unchanged, wantErr: true},
		"push job reads its own run's artifacts": {dispatcher: replaceOnce("          run-id: ${{ needs.render.outputs.run_id }}\n          github-token: ${{ github.token }}\n", "", t), render: unchanged, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "visual-baselines.yml"), tc.dispatcher(dispatcher))
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "visual-baselines-render.yml"), tc.render(render))
			err := VerifyCIContainerDownloads(root)
			if tc.wantErr && err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a changed Visual baselines checkout or artifact")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("VerifyCIContainerDownloads: %v", err)
			}
		})
	}
}
