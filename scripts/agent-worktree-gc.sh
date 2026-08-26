#!/usr/bin/env bash
# Audit every Loomarr worktree and optionally retire only exact, clean, merged PR heads.

set -euo pipefail

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
ROOT="${LOOMARR_REPO_ROOT:-$(CDPATH='' cd -- "$SCRIPT_DIR/.." && pwd -P)}"
ROOT="$(CDPATH='' cd -- "$ROOT" && pwd -P)"
apply="${APPLY:-0}"

# shellcheck disable=SC1091 # Resolved beside this script at runtime.
. "$SCRIPT_DIR/dev-processes.sh"

if [[ "$apply" != 0 && "$apply" != 1 ]]; then
	echo 'agent-gc: APPLY must be 0 or 1' >&2
	exit 2
fi

command -v gh >/dev/null 2>&1 || {
	echo 'agent-gc: gh is required to verify squash-merged PR heads' >&2
	exit 1
}

mapfile -t worktree_paths < <(git -C "$ROOT" worktree list --porcelain | sed -n 's/^worktree //p')
primary="${worktree_paths[0]}"
main_ref="${AGENT_GC_MAIN_REF:-origin/main}"
if [[ -z "${AGENT_GC_MAIN_REF:-}" ]]; then
	if ! git -C "$primary" fetch --quiet origin main; then
		echo 'agent-gc: could not refresh origin/main; no worktrees were removed' >&2
		exit 1
	fi
fi
git -C "$primary" rev-parse --verify --quiet "$main_ref^{commit}" >/dev/null || {
	echo "agent-gc: cannot verify main ref $main_ref; no worktrees were removed" >&2
	exit 1
}

pr_file="$(mktemp)"
trap 'rm -f "$pr_file"' EXIT INT TERM
if ! (
	cd "$ROOT"
	gh pr list --state all --limit 1000 \
		--json headRefName,headRefOid,state,mergeCommit,number,url \
		--jq '.[] | [.headRefName, .headRefOid, .state, (.mergeCommit.oid // ""), (.number | tostring), .url] | @tsv'
) > "$pr_file"; then
	echo 'agent-gc: could not read GitHub PR state; no worktrees were removed' >&2
	exit 1
fi

declare -A pr_state=()
declare -A pr_merge=()
declare -A pr_number=()
declare -A branch_seen=()
while IFS=$'\t' read -r branch head state merge_oid number _url; do
	[[ -n "$branch" && -n "$head" ]] || continue
	key="$branch|$head"
	branch_seen["$branch"]=1
	# `gh pr list` returns newest first. Preserve the newest exact branch/head match if a
	# branch name was reused across pull requests.
	if [[ -z "${pr_state[$key]:-}" ]]; then
		pr_state["$key"]="$state"
		pr_merge["$key"]="$merge_oid"
		pr_number["$key"]="$number"
	fi
done < "$pr_file"

declare -A active_paths=()
declare -A dependency_branches=()
state_dir="$(git -C "$ROOT" rev-parse --path-format=absolute --git-common-dir)/loomarr-agents/sessions"
for session in "$state_dir"/*; do
	[[ -f "$session" ]] || continue
	path="$(sed -n 's/^worktree=//p' "$session")"
	[[ -n "$path" ]] && active_paths["$path"]=1
	dependency_branch="$(sed -n 's/^dependency_branch=//p' "$session")"
	[[ -n "$dependency_branch" ]] && dependency_branches["$dependency_branch"]=1
done

eligible=0
removed=0
protected=0
errors=0
secondary=0

report_protected() {
	local branch="$1" reason="$2" path="$3" detail="${4:--}"
	protected=$((protected + 1))
	printf 'PROTECTED\t%s\t%s\t%s\t%s\n' "$branch" "$reason" "$path" "$detail"
}

running_processes() {
	local path="$1" comm pids found=''
	for comm in air loomarr-dev node pnpm ffmpeg; do
		pids="$(repo_pids_by_comm "$comm" "$path")"
		if [[ -n "$pids" ]]; then
			found="${found:+$found,}$comm:$(printf '%s\n' "$pids" | paste -sd+ -)"
		fi
	done
	printf '%s' "$found"
}

process_worktree() {
	local path="$1" head="$2" branch_ref="$3" locked="$4"
	local branch key state merge_oid number status credentials processes

	if [[ "$path" == "$primary" ]]; then
		return 0
	fi
	secondary=$((secondary + 1))
	branch="${branch_ref#refs/heads/}"

	if [[ ! -d "$path" ]]; then
		report_protected "${branch:-detached}" missing "$path"
		return
	fi
	if [[ "$locked" == 1 ]]; then
		report_protected "${branch:-detached}" locked "$path"
		return
	fi
	if [[ -z "$branch_ref" ]]; then
		report_protected detached detached "$path"
		return
	fi
	if [[ -n "${active_paths[$path]:-}" ]]; then
		report_protected "$branch" active "$path"
		return
	fi
	if [[ -n "${dependency_branches[$branch]:-}" ]]; then
		report_protected "$branch" active-dependency "$path"
		return
	fi
	if [[ "$path" == "$ROOT" ]]; then
		report_protected "$branch" current-worktree "$path"
		return
	fi
	processes="$(running_processes "$path")"
	if [[ -n "$processes" ]]; then
		report_protected "$branch" running-process "$path" "$processes"
		return
	fi
	if ! status="$(git -C "$path" status --porcelain --untracked-files=all 2>/dev/null)"; then
		report_protected "$branch" unreadable "$path"
		return
	fi
	if [[ -n "$status" ]]; then
		report_protected "$branch" dirty "$path"
		return
	fi
	credentials="$(git -C "$path" ls-files --others --ignored --exclude-standard -- .env '.env.*' 2>/dev/null || true)"
	if [[ -n "$credentials" ]]; then
		report_protected "$branch" local-credentials "$path"
		return
	fi

	key="$branch|$head"
	state="${pr_state[$key]:-}"
	if [[ -z "$state" ]]; then
		if [[ -n "${branch_seen[$branch]:-}" ]]; then
			report_protected "$branch" head-diverged "$path"
		else
			report_protected "$branch" no-pr "$path"
		fi
		return
	fi
	number="${pr_number[$key]}"
	case "$state" in
		OPEN)
			report_protected "$branch" open-pr "$path" "#$number"
			return
			;;
		CLOSED)
			report_protected "$branch" closed-unmerged "$path" "#$number"
			return
			;;
		MERGED) ;;
		*)
			report_protected "$branch" "unknown-pr-state:$state" "$path" "#$number"
			return
			;;
	esac
	merge_oid="${pr_merge[$key]}"
	if [[ -z "$merge_oid" ]] || ! git -C "$primary" merge-base --is-ancestor "$merge_oid" "$main_ref" 2>/dev/null; then
		report_protected "$branch" merge-not-on-main "$path" "#$number"
		return
	fi

	eligible=$((eligible + 1))
	if [[ "$apply" == 0 ]]; then
		printf 'ELIGIBLE\t%s\tmerged-pr\t%s\t#%s\n' "$branch" "$path" "$number"
		return
	fi
	if ! git -C "$primary" worktree remove "$path"; then
		printf 'ERROR\t%s\tremove-failed\t%s\t#%s\n' "$branch" "$path" "$number" >&2
		errors=$((errors + 1))
		return
	fi
	if ! git -C "$primary" update-ref -d "refs/heads/$branch" "$head"; then
		printf 'ERROR\t%s\tbranch-moved\t%s\t#%s\n' "$branch" "$path" "$number" >&2
		errors=$((errors + 1))
		return
	fi
	removed=$((removed + 1))
	printf 'REMOVED\t%s\tmerged-pr\t%s\t#%s\n' "$branch" "$path" "$number"
}

path=
head=
branch_ref=
locked=0
while IFS= read -r line || [[ -n "$line" ]]; do
	if [[ -z "$line" ]]; then
		[[ -z "$path" ]] || process_worktree "$path" "$head" "$branch_ref" "$locked"
		path=
		head=
		branch_ref=
		locked=0
		continue
	fi
	case "$line" in
		worktree\ *) path="${line#worktree }" ;;
		HEAD\ *) head="${line#HEAD }" ;;
		branch\ *) branch_ref="${line#branch }" ;;
		locked*) locked=1 ;;
	esac
done < <(git -C "$ROOT" worktree list --porcelain; printf '\n')

printf 'SUMMARY\tsecondary=%d eligible=%d removed=%d protected=%d errors=%d mode=%s\n' \
	"$secondary" "$eligible" "$removed" "$protected" "$errors" "$([[ "$apply" == 1 ]] && printf apply || printf audit)"

[[ "$errors" == 0 ]]
