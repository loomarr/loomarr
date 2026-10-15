package releaseverify

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Visual baselines regenerates one pull request's committed Linux baselines on a runner and pushes
// only the changed PNGs to that pull request's branch, in two workflows. visual-baselines.yml runs
// from the default branch and runs none of the branch's code; visual-baselines-render.yml runs the
// branch's code read-only, dispatched at the branch so it stays in that branch's Actions cache
// scope. Their inline scripts are the whole of their authority, so each is pinned here verbatim;
// the workflow authority catalog registers the same text (with the YAML block's trailing newline)
// at its absolute step.

const visualBaselinesResolveCommand = `set -euo pipefail
if [ "$REF" != "refs/heads/$DEFAULT_BRANCH" ]; then
  echo "::error::dispatch Visual baselines from $DEFAULT_BRANCH, not $REF"
  exit 1
fi
if [[ ! $PR =~ ^[1-9][0-9]{0,6}$ ]]; then
  echo "::error::pr must be a pull request number, got '$PR'"
  exit 1
fi
gh pr view "$PR" --repo "$REPO" \
  --json state,isCrossRepository,headRefName,headRefOid,headRepository,headRepositoryOwner \
  > "$RUNNER_TEMP/pr.json"
state=$(jq -r .state "$RUNNER_TEMP/pr.json")
cross=$(jq -r .isCrossRepository "$RUNNER_TEMP/pr.json")
head_repo=$(jq -r '"\(.headRepositoryOwner.login)/\(.headRepository.name)"' "$RUNNER_TEMP/pr.json")
branch=$(jq -r .headRefName "$RUNNER_TEMP/pr.json")
sha=$(jq -r .headRefOid "$RUNNER_TEMP/pr.json")
if [ "$state" != OPEN ]; then
  echo "::error::#$PR is $state; only an open pull request gets regenerated baselines"
  exit 1
fi
queued=$(gh api graphql --jq .data.repository.pullRequest.isInMergeQueue \
  -f query="{repository(owner:\"${REPO%%/*}\",name:\"${REPO#*/}\"){pullRequest(number:$PR){isInMergeQueue}}}")
if [ "$queued" != false ]; then
  echo "::error::#$PR is in the merge queue, which locks its branch; take it out of the queue first"
  exit 1
fi
if [ "$cross" != false ] || [ "${head_repo,,}" != "${REPO,,}" ]; then
  echo "::error::#$PR comes from $head_repo, not $REPO; fork branches are refused"
  exit 1
fi
if [ "$branch" = "$DEFAULT_BRANCH" ] || ! git check-ref-format --branch "$branch" >/dev/null; then
  echo "::error::#$PR has head branch '$branch', which this workflow will not push to"
  exit 1
fi
if [[ ! $sha =~ ^[0-9a-f]{40}$ ]]; then
  echo "::error::#$PR has an unreadable head commit '$sha'"
  exit 1
fi
{ echo "branch=$branch"; echo "sha=$sha"; } >> "$GITHUB_OUTPUT"
echo "Regenerating baselines for #$PR: $branch at $sha" >> "$GITHUB_STEP_SUMMARY"`

const visualBaselinesRenderDispatchCommand = `set -euo pipefail
inputs=(-f sha="$HEAD_SHA" -f request="$REQUEST")
if [ -n "$FORCE" ]; then
  inputs+=(-f force="$FORCE")
  echo "Forcing a rewrite of every baseline of the tests matching: $FORCE" >> "$GITHUB_STEP_SUMMARY"
fi
if ! gh workflow run visual-baselines-render.yml --repo "$REPO" --ref "$BRANCH" "${inputs[@]}"; then
  echo "::error::could not dispatch the render on $BRANCH; rebase it onto the default branch so it has visual-baselines-render.yml"
  exit 1
fi
run_id=""
for _ in $(seq 1 24); do
  run_id=$(gh run list --repo "$REPO" --workflow visual-baselines-render.yml --branch "$BRANCH" \
    --event workflow_dispatch --limit 20 --json databaseId,displayTitle \
    | jq -r --arg title "Render visual baselines $REQUEST" 'map(select(.displayTitle == $title)) | .[0].databaseId // ""')
  if [ -n "$run_id" ]; then
    break
  fi
  sleep 5
done
if [ -z "$run_id" ]; then
  echo "::error::the render was dispatched on $BRANCH but its run never appeared"
  exit 1
fi
echo "run_id=$run_id" >> "$GITHUB_OUTPUT"
echo "Render: $SERVER_URL/$REPO/actions/runs/$run_id" | tee -a "$GITHUB_STEP_SUMMARY"
until [ "$(gh api "repos/$REPO/actions/runs/$run_id" --jq .status)" = completed ]; do
  sleep 30
done
conclusion=$(gh api "repos/$REPO/actions/runs/$run_id" --jq .conclusion)
if [ "$conclusion" != success ]; then
  echo "::error::the render run finished as $conclusion: $SERVER_URL/$REPO/actions/runs/$run_id"
  exit 1
fi`

const visualBaselinesVerifyRenderCommand = `set -euo pipefail
if [[ ! $RENDER_RUN =~ ^[1-9][0-9]*$ ]]; then
  echo "::error::the render job reported no run id, got '$RENDER_RUN'"
  exit 1
fi
gh api "repos/$REPO/actions/runs/$RENDER_RUN" > "$RUNNER_TEMP/render.json"
if ! jq -e --arg repo "$REPO" --arg branch "$BRANCH" --arg sha "$HEAD_SHA" \
  --arg title "Render visual baselines $REQUEST" '
    .path == ".github/workflows/visual-baselines-render.yml"
    and .event == "workflow_dispatch"
    and .status == "completed" and .conclusion == "success"
    and .repository.full_name == $repo and .head_repository.full_name == $repo
    and .head_branch == $branch and .head_sha == $sha
    and .display_title == $title' "$RUNNER_TEMP/render.json" > /dev/null; then
  echo "::error::run $RENDER_RUN is not the successful render of $BRANCH at $HEAD_SHA that this run dispatched"
  exit 1
fi`

const visualBaselinesRenderGuardCommand = `set -euo pipefail
if [[ $REF != refs/heads/* ]] || [ "$REF" = "refs/heads/$DEFAULT_BRANCH" ]; then
  echo "::error::the render runs only on a pull request's branch, never $REF; dispatch Visual baselines instead"
  exit 1
fi
if [ "$SHA" != "$WANT_SHA" ]; then
  echo "::error::$REF is at $SHA, not the resolved $WANT_SHA; dispatch Visual baselines again"
  exit 1
fi`

const visualBaselinesCollectCommand = `set -euo pipefail
out="$RUNNER_TEMP/visual-baselines"
allowed='^web/apps/web/tests/visual/[A-Za-z0-9._-]+\.spec\.ts-snapshots/[A-Za-z0-9._-]+\.png$'
mkdir -p "$out/files"
git rev-parse HEAD > "$out/commit"
: > "$out/changed.txt"
git status --porcelain=v1 -z --untracked-files=all > "$RUNNER_TEMP/status"
unexpected=0
while IFS= read -r -d '' entry; do
  status=${entry:0:2}
  path=${entry:3}
  if [[ ($status == ' M' || $status == '??') && $path =~ $allowed ]]; then
    printf '%s\n' "$path" >> "$out/changed.txt"
    mkdir -p "$out/files/$(dirname "$path")"
    cp -- "$path" "$out/files/$path"
  else
    echo "::error::the update changed something other than a visual baseline: '$status' $path"
    unexpected=1
  fi
done < "$RUNNER_TEMP/status"
if [ "$unexpected" -ne 0 ]; then
  exit 1
fi
echo "$(wc -l < "$out/changed.txt") baselines changed" >> "$GITHUB_STEP_SUMMARY"`

const visualBaselinesCommitCommand = `set -euo pipefail
in="$RUNNER_TEMP/visual-baselines"
allowed='^web/apps/web/tests/visual/[A-Za-z0-9._-]+\.spec\.ts-snapshots/[A-Za-z0-9._-]+\.png$'
if [ "$(git rev-parse HEAD)" != "$HEAD_SHA" ] || [ "$(cat "$in/commit")" != "$HEAD_SHA" ]; then
  echo "::error::$BRANCH is no longer $HEAD_SHA, the commit that was rendered; dispatch again"
  exit 1
fi
if [ ! -s "$in/changed.txt" ]; then
  echo "No visual baseline changed for #$PR at $HEAD_SHA; nothing pushed." | tee -a "$GITHUB_STEP_SUMMARY"
  echo "pushed=false" >> "$GITHUB_OUTPUT"
  exit 0
fi
while IFS= read -r path; do
  if [[ ! $path =~ $allowed ]] || [ ! -f "$in/files/$path" ] || [ -L "$in/files/$path" ]; then
    echo "::error::refusing baseline '$path'"
    exit 1
  fi
  mode=$(git ls-files --stage -- "$path" | cut -d' ' -f1)
  if [ -n "$mode" ] && [ "$mode" != 100644 ]; then
    echo "::error::refusing to replace '$path', which the branch tracks as mode $mode"
    exit 1
  fi
  blob=$(git hash-object -w --no-filters -- "$in/files/$path")
  git update-index --add --cacheinfo "100644,$blob,$path"
done < "$in/changed.txt"
(cd "$in/files" && find . -type f -printf '%P\n') | sort > "$RUNNER_TEMP/artifact.txt"
sort "$in/changed.txt" > "$RUNNER_TEMP/manifest.txt"
if ! cmp -s "$RUNNER_TEMP/artifact.txt" "$RUNNER_TEMP/manifest.txt"; then
  echo "::error::the artifact holds files its manifest does not list"
  exit 1
fi
git diff --cached --name-only | sort > "$RUNNER_TEMP/staged.txt"
if [ -n "$(comm -23 "$RUNNER_TEMP/staged.txt" "$RUNNER_TEMP/manifest.txt")" ]; then
  echo "::error::the index has changes beyond the listed baselines"
  exit 1
fi
if [ ! -s "$RUNNER_TEMP/staged.txt" ]; then
  echo "No visual baseline changed for #$PR at $HEAD_SHA; nothing pushed." | tee -a "$GITHUB_STEP_SUMMARY"
  echo "pushed=false" >> "$GITHUB_OUTPUT"
  exit 0
fi
count=$(wc -l < "$RUNNER_TEMP/staged.txt")
printf '%s\n' \
  "test(visual): regenerate Linux baselines for #$PR" \
  "" \
  "Baselines only: $count PNGs rewritten by the Visual baselines workflow," \
  "run $RUN_URL (attempt $RUN_ATTEMPT), rendered from $HEAD_SHA by render run $RENDER_RUN." \
  > "$RUNNER_TEMP/commit-message.txt"
if [ -n "$FORCE" ]; then
  printf '%s\n' "" "Forced: every baseline of the tests matching this regex was rewritten:" "$FORCE" \
    >> "$RUNNER_TEMP/commit-message.txt"
fi
git -c user.name='github-actions[bot]' \
  -c user.email='41898282+github-actions[bot]@users.noreply.github.com' \
  commit --quiet --no-verify --file="$RUNNER_TEMP/commit-message.txt"
git push origin "HEAD:refs/heads/$BRANCH"
{ echo "pushed=true"; echo "sha=$(git rev-parse HEAD)"; } >> "$GITHUB_OUTPUT"
echo "Pushed $count baselines to $BRANCH as $(git rev-parse HEAD)." >> "$GITHUB_STEP_SUMMARY"`

const visualBaselinesCIDispatchCommand = `set -euo pipefail
gh workflow run ci.yml --repo "$REPO" --ref "$BRANCH" -f scope=full
run_url=""
for _ in $(seq 1 24); do
  run_url=$(gh run list --repo "$REPO" --workflow ci.yml --branch "$BRANCH" \
    --event workflow_dispatch --limit 20 --json headSha,url \
    | jq -r --arg sha "$HEAD_SHA" 'map(select(.headSha == $sha)) | .[0].url // ""')
  if [ -n "$run_url" ]; then
    break
  fi
  sleep 5
done
if [ -z "$run_url" ]; then
  echo "::error::CI was dispatched for $BRANCH but no run for $HEAD_SHA appeared"
  exit 1
fi
echo "CI for $HEAD_SHA: $run_url" | tee -a "$GITHUB_STEP_SUMMARY"`

const visualBaselinesUpdateStepName = "Regenerate the baselines in the pinned Playwright image"

// The dispatching run's id and attempt: the render run is named after it and the push job checks
// that name, so a render run some other dispatch started is never mistaken for this run's.
const visualBaselinesRequest = "${{ github.run_id }}-${{ github.run_attempt }}"

// visualBaselinesStep is the audited shape of one step. An action step names only its action;
// ci_container_workflow_actions.go pins its commit and inputs. A run step fixes its exact command,
// name, id, shell, and environment (nil when it has none).
type visualBaselinesStep struct {
	action string
	run    string
	name   string
	id     string
	shell  string
	env    map[string]string
}

type visualBaselinesJob struct {
	name        string
	keys        []string
	permissions map[string]string
	outputs     map[string]string
	condition   string
	steps       []visualBaselinesStep
}

// visualBaselinesInput is one string dispatch input. A required one has no default; an optional one
// defaults to empty, so a dispatch that omits it behaves as it did before the input existed.
type visualBaselinesInput struct {
	name     string
	optional bool
}

// visualBaselinesWorkflow is one workflow's audited shape: its top-level keys, its name and run
// name, its string inputs, its exact workflow-level maps, and its jobs in order.
type visualBaselinesWorkflow struct {
	label   string
	keys    []string
	name    string
	runName string
	inputs  []visualBaselinesInput
	maps    map[string]map[string]string
	jobs    []visualBaselinesJob
}

// visualBaselinesDispatcher never runs the branch's code. Only commit may write contents; render
// and ci may dispatch a workflow and nothing else; commit reads the render run's artifact.
func visualBaselinesDispatcher() visualBaselinesWorkflow {
	return visualBaselinesWorkflow{
		label:  "Visual baselines workflow",
		keys:   []string{"name", "on", "permissions", "concurrency", "jobs"},
		name:   "Visual baselines",
		inputs: []visualBaselinesInput{{name: "pr"}, {name: "force", optional: true}},
		maps: map[string]map[string]string{
			"permissions": {"contents": "read"},
			"concurrency": {"group": "visual-baselines-${{ inputs.pr }}", "cancel-in-progress": "false"},
		},
		jobs: []visualBaselinesJob{
			{
				name:        "resolve",
				keys:        []string{"name", "runs-on", "timeout-minutes", "permissions", "outputs", "steps"},
				permissions: map[string]string{"contents": "read", "pull-requests": "read"},
				outputs: map[string]string{
					"branch": "${{ steps.pr.outputs.branch }}",
					"sha":    "${{ steps.pr.outputs.sha }}",
				},
				steps: []visualBaselinesStep{{
					run: visualBaselinesResolveCommand, name: "Resolve and vet the pull request", id: "pr", shell: "bash",
					env: map[string]string{
						"GH_TOKEN":       "${{ github.token }}",
						"PR":             "${{ inputs.pr }}",
						"REPO":           "${{ github.repository }}",
						"REF":            "${{ github.ref }}",
						"DEFAULT_BRANCH": "${{ github.event.repository.default_branch }}",
					},
				}},
			},
			{
				name:        "render",
				keys:        []string{"name", "needs", "runs-on", "timeout-minutes", "permissions", "outputs", "steps"},
				permissions: map[string]string{"actions": "write", "contents": "read"},
				outputs:     map[string]string{"run_id": "${{ steps.render.outputs.run_id }}"},
				steps: []visualBaselinesStep{{
					run: visualBaselinesRenderDispatchCommand, name: "Dispatch the render and wait for it", id: "render", shell: "bash",
					env: map[string]string{
						"GH_TOKEN":   "${{ github.token }}",
						"REPO":       "${{ github.repository }}",
						"BRANCH":     "${{ needs.resolve.outputs.branch }}",
						"HEAD_SHA":   "${{ needs.resolve.outputs.sha }}",
						"REQUEST":    visualBaselinesRequest,
						"SERVER_URL": "${{ github.server_url }}",
						"FORCE":      "${{ inputs.force }}",
					},
				}},
			},
			{
				name:        "commit",
				keys:        []string{"name", "needs", "runs-on", "timeout-minutes", "permissions", "outputs", "steps"},
				permissions: map[string]string{"actions": "read", "contents": "write"},
				outputs: map[string]string{
					"pushed": "${{ steps.commit.outputs.pushed }}",
					"sha":    "${{ steps.commit.outputs.sha }}",
				},
				steps: []visualBaselinesStep{
					{action: "actions/checkout"},
					{
						run: visualBaselinesVerifyRenderCommand, name: "Verify the render run", shell: "bash",
						env: map[string]string{
							"GH_TOKEN":   "${{ github.token }}",
							"REPO":       "${{ github.repository }}",
							"BRANCH":     "${{ needs.resolve.outputs.branch }}",
							"HEAD_SHA":   "${{ needs.resolve.outputs.sha }}",
							"RENDER_RUN": "${{ needs.render.outputs.run_id }}",
							"REQUEST":    visualBaselinesRequest,
						},
					},
					{action: "actions/download-artifact"},
					{
						run: visualBaselinesCommitCommand, name: "Commit and push only the changed baselines", id: "commit", shell: "bash",
						env: map[string]string{
							"PR":          "${{ inputs.pr }}",
							"BRANCH":      "${{ needs.resolve.outputs.branch }}",
							"HEAD_SHA":    "${{ needs.resolve.outputs.sha }}",
							"RUN_URL":     "${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}",
							"RUN_ATTEMPT": "${{ github.run_attempt }}",
							"RENDER_RUN":  "${{ needs.render.outputs.run_id }}",
							"FORCE":       "${{ inputs.force }}",
						},
					},
				},
			},
			{
				name:        "ci",
				keys:        []string{"name", "needs", "if", "runs-on", "timeout-minutes", "permissions", "steps"},
				permissions: map[string]string{"actions": "write", "contents": "read"},
				condition:   "needs.commit.outputs.pushed == 'true'",
				steps: []visualBaselinesStep{{
					run: visualBaselinesCIDispatchCommand, name: "Dispatch CI for the pushed commit", shell: "bash",
					env: map[string]string{
						"GH_TOKEN": "${{ github.token }}",
						"REPO":     "${{ github.repository }}",
						"BRANCH":   "${{ needs.resolve.outputs.branch }}",
						"HEAD_SHA": "${{ needs.commit.outputs.sha }}",
					},
				}},
			},
		},
	}
}

// visualBaselinesRender runs the branch's code, so it can only read, and its first step refuses
// the default branch's cache scope and any commit but the resolved one before anything is fetched.
func visualBaselinesRender() visualBaselinesWorkflow {
	return visualBaselinesWorkflow{
		label:   "Visual baselines render workflow",
		keys:    []string{"name", "run-name", "on", "permissions", "env", "jobs"},
		name:    "Visual baselines render",
		runName: "Render visual baselines ${{ inputs.request }}",
		inputs:  []visualBaselinesInput{{name: "sha"}, {name: "request"}, {name: "force", optional: true}},
		maps: map[string]map[string]string{
			"permissions": {"contents": "read"},
			"env":         {"NODE_VERSION": "22"},
		},
		jobs: []visualBaselinesJob{{
			name:        "render",
			keys:        []string{"name", "runs-on", "timeout-minutes", "permissions", "steps"},
			permissions: map[string]string{"contents": "read"},
			steps: []visualBaselinesStep{
				{
					run: visualBaselinesRenderGuardCommand, name: "Refuse the default branch", shell: "bash",
					env: map[string]string{
						"REF":            "${{ github.ref }}",
						"DEFAULT_BRANCH": "${{ github.event.repository.default_branch }}",
						"SHA":            "${{ github.sha }}",
						"WANT_SHA":       "${{ inputs.sha }}",
					},
				},
				{action: "actions/checkout"},
				{action: "actions/setup-node"},
				{run: "corepack enable"},
				{action: "actions/cache/restore"},
				{run: "make fe-install"},
				{run: "make fe-codegen"},
				{
					run: "make fe-visual-update", name: visualBaselinesUpdateStepName,
					env: map[string]string{"LOOMARR_PLAYWRIGHT_FORCE": "${{ inputs.force }}"},
				},
				{run: visualBaselinesCollectCommand, name: "Collect the rewritten baselines", shell: "bash"},
				{action: "actions/upload-artifact"},
			},
		}},
	}
}

// VerifyVisualBaselinesWorkflow keeps the baseline push manual, serialized per pull request, and
// behind least-privilege jobs that run none of the branch's code. Every step is pinned, every step
// is unconditional and fails closed, and no expression is interpolated into a shell.
func VerifyVisualBaselinesWorkflow(path string) error {
	return verifyVisualBaselinesWorkflow(path, visualBaselinesDispatcher())
}

// VerifyVisualBaselinesRenderWorkflow keeps the half that runs the branch's code read-only and out
// of the default branch's cache scope, with the same pinned, unconditional, env-only steps.
func VerifyVisualBaselinesRenderWorkflow(path string) error {
	return verifyVisualBaselinesWorkflow(path, visualBaselinesRender())
}

func verifyVisualBaselinesWorkflow(path string, want visualBaselinesWorkflow) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	root, err := parseYAML(data)
	if err != nil {
		return err
	}
	if err := verifyUses(root); err != nil {
		return err
	}
	if err := verifyOnlyKeys(root, want.label, want.keys...); err != nil {
		return err
	}
	if scalarValue(root, "name") != want.name {
		return fmt.Errorf("%s name must be %s", want.label, want.name)
	}
	if got := scalarValue(root, "run-name"); got != want.runName {
		return fmt.Errorf("%s run-name is %q, want %q", want.label, got, want.runName)
	}
	if err := verifyVisualBaselinesTrigger(root, want); err != nil {
		return err
	}
	for key, values := range want.maps {
		node, err := requiredMap(root, key)
		if err != nil {
			return err
		}
		if err := verifyExactScalarMap(node, want.label+" "+key, values); err != nil {
			return err
		}
	}
	jobs, err := requiredMap(root, "jobs")
	if err != nil {
		return err
	}
	if len(jobs.Content) != len(want.jobs)*2 {
		return fmt.Errorf("%s must contain exactly the %d audited jobs", want.label, len(want.jobs))
	}
	for index, job := range want.jobs {
		if jobs.Content[index*2].Value != job.name {
			return fmt.Errorf("%s job %d must be %s", want.label, index+1, job.name)
		}
		if err := verifyVisualBaselinesJob(want.label+" job "+job.name, jobs.Content[index*2+1], job); err != nil {
			return err
		}
	}
	return nil
}

// Dispatch only: pull_request, merge_group, and push would run either workflow without anyone
// asking, and pull_request_target would hand a fork the write token.
func verifyVisualBaselinesTrigger(root *yaml.Node, want visualBaselinesWorkflow) error {
	on, err := requiredMap(root, "on")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(on, want.label+" trigger", "workflow_dispatch"); err != nil {
		return err
	}
	dispatch, err := requiredMap(on, "workflow_dispatch")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(dispatch, want.label+" dispatch", "inputs"); err != nil {
		return err
	}
	inputs, err := requiredMap(dispatch, "inputs")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(want.inputs))
	for _, input := range want.inputs {
		names = append(names, input.name)
	}
	if err := verifyOnlyKeys(inputs, want.label+" inputs", names...); err != nil {
		return err
	}
	for _, spec := range want.inputs {
		input, err := requiredMap(inputs, spec.name)
		if err != nil {
			return err
		}
		label := want.label + " " + spec.name + " input"
		if !spec.optional {
			if err := verifyOnlyKeys(input, label, "description", "required", "type"); err != nil {
				return err
			}
			if scalarValue(input, "type") != "string" || scalarValue(input, "required") != "true" {
				return fmt.Errorf("%s input %s must be a required string", want.label, spec.name)
			}
			continue
		}
		if err := verifyOnlyKeys(input, label, "description", "required", "default", "type"); err != nil {
			return err
		}
		if _, ok := mappingValue(input, "default"); !ok || scalarValue(input, "default") != "" ||
			scalarValue(input, "type") != "string" || scalarValue(input, "required") != "false" {
			return fmt.Errorf("%s input %s must be an optional string that defaults to empty", want.label, spec.name)
		}
	}
	return nil
}

func verifyVisualBaselinesJob(label string, job *yaml.Node, want visualBaselinesJob) error {
	if job.Kind != yaml.MappingNode {
		return fmt.Errorf("%s must be a mapping", label)
	}
	if err := verifyOnlyKeys(job, label, want.keys...); err != nil {
		return err
	}
	for _, key := range want.keys {
		if _, ok := mappingValue(job, key); !ok {
			return fmt.Errorf("%s must declare %s", label, key)
		}
	}
	if got := scalarValue(job, "if"); got != want.condition {
		return fmt.Errorf("%s condition is %q, want %q", label, got, want.condition)
	}
	permissions, err := requiredMap(job, "permissions")
	if err != nil {
		return err
	}
	if err := verifyExactScalarMap(permissions, label+" permissions", want.permissions); err != nil {
		return err
	}
	if want.outputs != nil {
		outputs, err := requiredMap(job, "outputs")
		if err != nil {
			return err
		}
		if err := verifyExactScalarMap(outputs, label+" outputs", want.outputs); err != nil {
			return err
		}
	}
	steps, err := requiredSequence(job, "steps")
	if err != nil {
		return err
	}
	if len(steps.Content) != len(want.steps) {
		return fmt.Errorf("%s must contain exactly %d audited steps, found %d", label, len(want.steps), len(steps.Content))
	}
	for index, step := range steps.Content {
		if err := verifyVisualBaselinesStep(fmt.Sprintf("%s step %d", label, index+1), step, want.steps[index]); err != nil {
			return err
		}
	}
	return nil
}

func verifyVisualBaselinesStep(label string, step *yaml.Node, want visualBaselinesStep) error {
	if step.Kind != yaml.MappingNode {
		return fmt.Errorf("%s must be a mapping", label)
	}
	if want.action != "" {
		if err := verifyOnlyKeys(step, label, "name", "uses", "with"); err != nil {
			return err
		}
		if actionName(step) != want.action {
			return fmt.Errorf("%s must use %s", label, want.action)
		}
		return nil
	}
	if err := verifyOnlyKeys(step, label, "name", "id", "shell", "env", "run"); err != nil {
		return err
	}
	run := scalarValue(step, "run")
	if strings.Contains(run, "${{") {
		return fmt.Errorf("%s interpolates an expression into its shell; pass it through env", label)
	}
	if strings.TrimSpace(run) != want.run {
		return fmt.Errorf("%s is not its audited command", label)
	}
	for key, value := range map[string]string{"name": want.name, "id": want.id, "shell": want.shell} {
		if got := scalarValue(step, key); got != value {
			return fmt.Errorf("%s %s is %q, want %q", label, key, got, value)
		}
	}
	env, ok := mappingValue(step, "env")
	if want.env == nil {
		if ok {
			return fmt.Errorf("%s must not declare an environment", label)
		}
		return nil
	}
	if !ok {
		return fmt.Errorf("%s must declare its audited environment", label)
	}
	return verifyExactScalarMap(env, label+" environment", want.env)
}
