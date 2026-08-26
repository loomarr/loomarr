#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
tmp="$(mktemp -d)"
repo="$tmp/repo"
fake_bin="$tmp/bin"
fixture="$tmp/prs.tsv"
runtime_pid=
trap '[[ -z "$runtime_pid" ]] || kill "$runtime_pid" 2>/dev/null || true; rm -rf "$tmp"' EXIT INT TERM

git init -q -b main "$repo"
git -C "$repo" config user.email test@example.invalid
git -C "$repo" config user.name 'Worktree GC Test'
printf '.env\n' > "$repo/.gitignore"
printf 'base\n' > "$repo/base.txt"
git -C "$repo" add .gitignore base.txt
git -C "$repo" commit -qm base

add_worktree() {
	name="$1"
	path="$tmp/$name"
	git -C "$repo" worktree add -q -b "$name" "$path"
	printf '%s\n' "$name" > "$path/$name.txt"
	git -C "$path" add "$name.txt"
	git -C "$path" commit -qm "$name"
	printf '%s\n' "$path"
}

eligible="$(add_worktree eligible)"
dirty="$(add_worktree dirty)"
active="$(add_worktree active)"
open="$(add_worktree open)"
diverged="$(add_worktree diverged)"
no_pr="$(add_worktree no-pr)"
credential="$(add_worktree credential)"
locked="$(add_worktree locked)"
dependency="$(add_worktree dependency)"
git -C "$repo" worktree add -q -b dependent "$tmp/dependent" dependency
dependent="$tmp/dependent"
printf 'dependent\n' > "$dependent/dependent.txt"
git -C "$dependent" add dependent.txt
git -C "$dependent" commit -qm dependent
runtime="$(add_worktree runtime)"
cp "$(command -v sleep)" "$runtime/loomarr-dev"
git -C "$runtime" add loomarr-dev
git -C "$runtime" commit -qm 'runtime fixture'

printf 'dirty\n' >> "$dirty/dirty.txt"
printf 'secret\n' > "$credential/.env"
git -C "$repo" worktree lock "$locked"
diverged_pr_head="$(git -C "$diverged" rev-parse HEAD)"
printf 'later\n' >> "$diverged/diverged.txt"
git -C "$diverged" add diverged.txt
git -C "$diverged" commit -qm 'diverged later'

git -C "$repo" commit --allow-empty -qm 'squash merge fixture'
merge_oid="$(git -C "$repo" rev-parse HEAD)"

mkdir -p "$fake_bin"
cat > "$fake_bin/gh" <<'EOF'
#!/usr/bin/env sh
[ "${GH_GC_FAIL:-0}" = 1 ] && exit 1
cat "$GH_GC_FIXTURE"
EOF
chmod +x "$fake_bin/gh"

pr_row() {
	branch="$1"
	head="$2"
	state="$3"
	merge="$4"
	number="$5"
	printf '%s\t%s\t%s\t%s\t%s\thttps://example.invalid/pr/%s\n' \
		"$branch" "$head" "$state" "$merge" "$number" "$number"
}

{
	pr_row eligible "$(git -C "$eligible" rev-parse HEAD)" MERGED "$merge_oid" 1
	pr_row dirty "$(git -C "$dirty" rev-parse HEAD)" MERGED "$merge_oid" 2
	pr_row active "$(git -C "$active" rev-parse HEAD)" MERGED "$merge_oid" 3
	pr_row open "$(git -C "$open" rev-parse HEAD)" OPEN '' 4
	pr_row diverged "$diverged_pr_head" MERGED "$merge_oid" 5
	pr_row credential "$(git -C "$credential" rev-parse HEAD)" MERGED "$merge_oid" 6
	pr_row locked "$(git -C "$locked" rev-parse HEAD)" MERGED "$merge_oid" 7
	pr_row dependency "$(git -C "$dependency" rev-parse HEAD)" MERGED "$merge_oid" 8
	pr_row runtime "$(git -C "$runtime" rev-parse HEAD)" MERGED "$merge_oid" 9
} > "$fixture"

LOOMARR_REPO_ROOT="$active" "$SCRIPT_DIR/agent.sh" start active-task '' >/dev/null
LOOMARR_REPO_ROOT="$dependency" "$SCRIPT_DIR/agent.sh" start dependency-task '' >/dev/null
LOOMARR_REPO_ROOT="$dependent" "$SCRIPT_DIR/agent.sh" start dependent-task '' dependency-task >/dev/null
LOOMARR_REPO_ROOT="$dependency" "$SCRIPT_DIR/agent.sh" stop >/dev/null
(cd "$runtime" && exec ./loomarr-dev 60) &
runtime_pid=$!
sleep 0.05

if PATH="$fake_bin:$PATH" GH_GC_FAIL=1 GH_GC_FIXTURE="$fixture" AGENT_GC_MAIN_REF=main \
	LOOMARR_REPO_ROOT="$repo" "$SCRIPT_DIR/agent-worktree-gc.sh" >/dev/null 2>&1; then
	echo 'agent-worktree-gc-test: GitHub failure did not fail closed' >&2
	exit 1
fi
test -d "$eligible"

audit="$(PATH="$fake_bin:$PATH" GH_GC_FIXTURE="$fixture" AGENT_GC_MAIN_REF=main \
	LOOMARR_REPO_ROOT="$repo" "$SCRIPT_DIR/agent-worktree-gc.sh")"
printf '%s\n' "$audit" | grep -q $'^ELIGIBLE\teligible\t'
printf '%s\n' "$audit" | grep -q $'^PROTECTED\tdirty\tdirty\t'
printf '%s\n' "$audit" | grep -q $'^PROTECTED\tactive\tactive\t'
printf '%s\n' "$audit" | grep -q $'^PROTECTED\topen\topen-pr\t'
printf '%s\n' "$audit" | grep -q $'^PROTECTED\tdiverged\thead-diverged\t'
printf '%s\n' "$audit" | grep -q $'^PROTECTED\tno-pr\tno-pr\t'
printf '%s\n' "$audit" | grep -q $'^PROTECTED\tcredential\tlocal-credentials\t'
printf '%s\n' "$audit" | grep -q $'^PROTECTED\tlocked\tlocked\t'
printf '%s\n' "$audit" | grep -q $'^PROTECTED\tdependency\tactive-dependency\t'
printf '%s\n' "$audit" | grep -q $'^PROTECTED\tdependent\tactive\t'
printf '%s\n' "$audit" | grep -q $'^PROTECTED\truntime\trunning-process\t'
printf '%s\n' "$audit" | grep -q 'eligible=1 removed=0 protected=10'
test -d "$eligible"

kill "$runtime_pid"
wait "$runtime_pid" 2>/dev/null || true
runtime_pid=

applied="$(PATH="$fake_bin:$PATH" GH_GC_FIXTURE="$fixture" AGENT_GC_MAIN_REF=main APPLY=1 \
	LOOMARR_REPO_ROOT="$repo" "$SCRIPT_DIR/agent-worktree-gc.sh")"
printf '%s\n' "$applied" | grep -q $'^REMOVED\teligible\t'
printf '%s\n' "$applied" | grep -q $'^REMOVED\truntime\t'
printf '%s\n' "$applied" | grep -q 'eligible=2 removed=2 protected=9'
test ! -e "$eligible"
test ! -e "$runtime"
if git -C "$repo" show-ref --verify --quiet refs/heads/eligible; then
	echo 'agent-worktree-gc-test: eligible branch was retained' >&2
	exit 1
fi
test -d "$dirty"
test -d "$active"
test -d "$open"
test -d "$diverged"
test -d "$no_pr"
test -d "$credential"
test -d "$locked"
test -d "$dependency"
test -d "$dependent"

echo 'agent-worktree-gc-test: ok'
