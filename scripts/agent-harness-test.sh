#!/usr/bin/env sh

set -eu

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
TMP="$(mktemp -d)"
TMP="$(CDPATH='' cd -- "$TMP" && pwd -P)"
pids=
# shellcheck disable=SC2154 # pid is the loop variable inside the trap evaluated at exit.
trap 'for pid in $pids; do kill "$pid" 2>/dev/null || true; done; rm -rf "$TMP" "$TMP-wt" "$TMP-third" "$TMP-fourth" "$TMP-fifth"' EXIT INT TERM

step() {
	echo "agent-harness-test: $*"
}

wait_for_repo_pid() {
	expected_pid="$1"
	comm="$2"
	root="$3"
	got=
	attempt=0
	while [ "$attempt" -lt 40 ]; do
		got="$(repo_pids_by_comm "$comm" "$root")"
		[ "$got" = "$expected_pid" ] && return 0
		attempt=$((attempt + 1))
		sleep 0.05
	done
	echo "agent-harness-test: process ownership for $root = [$got], want [$expected_pid]" >&2
	return 1
}

step 'environment isolation'
git -C "$TMP" init -q
git -C "$TMP" config user.email test@example.invalid
git -C "$TMP" config user.name 'Harness Test'
printf 'fixture\n' > "$TMP/fixture"
git -C "$TMP" add fixture
git -C "$TMP" commit -qm fixture
git -C "$TMP" worktree add -q "$TMP-wt" -b secondary

primary="$(LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/dev-env.sh" show)"
printf '%s\n' "$primary" | grep -q 'http://localhost:8080'
printf '%s\n' "$primary" | grep -q 'http://localhost:5173'

secondary="$(LOOMARR_REPO_ROOT="$TMP-wt" "$SCRIPT_DIR/dev-env.sh" show)"
printf '%s\n' "$secondary" | grep -q 'database override.*\.agent-data/loomarr.db'
printf '%s\n' "$secondary" | grep -q 'prepared override.*\.agent-data/prepared'
printf '%s\n' "$secondary" | grep -q 'public URL override.*http://localhost:'
secondary_exports="$(LOOMARR_REPO_ROOT="$TMP-wt" "$SCRIPT_DIR/dev-env.sh" export)"
printf '%s\n' "$secondary_exports" | grep -q "LOOMARR_AGENT_PREPARED_DIR=.*\.agent-data/prepared"
printf '%s\n' "$secondary_exports" | grep -q "LOOMARR_AGENT_PUBLIC_URL=.*http://localhost:"
printf '%s\n' "$secondary_exports" | grep -q "LOOMARR_AGENT_DEV_LOGIN='1'"
if printf '%s\n' "$(LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/dev-env.sh" export)" | grep -q "LOOMARR_AGENT_DEV_LOGIN='1'"; then
	echo 'agent-harness-test: primary worktree enabled automatic dev login' >&2
	exit 1
fi
grep -q 'PLAYOUT_PREPARED_DIR=.*LOOMARR_AGENT_PREPARED_DIR' "$SCRIPT_DIR/../.air.toml"
grep -q 'SERVER_PUBLIC_URL=.*LOOMARR_AGENT_PUBLIC_URL' "$SCRIPT_DIR/../.air.toml"
grep -q 'LOOMARR_DEV_LOGIN=.*LOOMARR_AGENT_DEV_LOGIN' "$SCRIPT_DIR/../.air.toml"
if printf '%s\n' "$secondary" | grep -q 'http://localhost:8080'; then
	echo 'agent-harness-test: secondary worktree reused the primary backend port' >&2
	exit 1
fi

overridden="$(LOOMARR_REPO_ROOT="$TMP-wt" LOOMARR_DEV_PORT=23456 "$SCRIPT_DIR/dev-env.sh" show)"
printf '%s\n' "$overridden" | grep -q 'http://localhost:23456'

step 'secondary dev identity'
mkdir -p "$TMP/fake-bin"
# shellcheck disable=SC2016 # BOOTSTRAP_LOG expands inside the generated fixture.
printf '%s\n' '#!/usr/bin/env sh' 'echo "$PWD|$*" >> "$BOOTSTRAP_LOG"' > "$TMP/fake-bin/go"
# shellcheck disable=SC2016 # BOOTSTRAP_LOG expands inside the generated fixture.
printf '%s\n' '#!/usr/bin/env sh' 'echo "cargo $*" >> "$BOOTSTRAP_LOG"' > "$TMP/fake-bin/cargo"
chmod +x "$TMP/fake-bin/go" "$TMP/fake-bin/cargo"
bootstrap_log="$TMP/bootstrap-runs"
PATH="$TMP/fake-bin:$PATH" BOOTSTRAP_LOG="$bootstrap_log" BOOTSTRAP_SKIP_FE=1 \
	LOOMARR_REPO_ROOT="$TMP-wt" "$SCRIPT_DIR/agent.sh" bootstrap >/dev/null
grep -q 'cargo build --locked -p loomarr-image' "$bootstrap_log"
grep -q 'run ./cmd/dev-bootstrap' "$bootstrap_log"
if ! grep -Fqx "$TMP-wt|run ./cmd/dev-bootstrap" "$bootstrap_log"; then
	echo 'agent-harness-test: dev-bootstrap did not run from the selected worktree' >&2
	exit 1
fi
: > "$bootstrap_log"
PATH="$TMP/fake-bin:$PATH" BOOTSTRAP_LOG="$bootstrap_log" BOOTSTRAP_SKIP_FE=1 AGENT_DEV_IDENTITY=0 \
	LOOMARR_REPO_ROOT="$TMP-wt" "$SCRIPT_DIR/agent.sh" bootstrap >/dev/null
grep -q 'cargo build --locked -p loomarr-image' "$bootstrap_log"
if grep -q 'run ./cmd/dev-bootstrap' "$bootstrap_log"; then
	echo 'agent-harness-test: disabled dev identity still ran dev-bootstrap' >&2
	exit 1
fi
rm -f "$TMP/fake-bin/go" "$TMP/fake-bin/cargo"

step 'claims and port conflicts'
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" start first openapi-client >/dev/null
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" status | grep 'first.*openapi-client' >/dev/null
if LOOMARR_REPO_ROOT="$TMP-wt" "$SCRIPT_DIR/agent.sh" start first visual-baselines >/dev/null 2>&1; then
	echo 'agent-harness-test: duplicate task was accepted' >&2
	exit 1
fi
if LOOMARR_REPO_ROOT="$TMP-wt" "$SCRIPT_DIR/agent.sh" start second visual-baselines missing-task >/dev/null 2>&1; then
	echo 'agent-harness-test: inactive dependency was accepted' >&2
	exit 1
fi
if LOOMARR_REPO_ROOT="$TMP-wt" "$SCRIPT_DIR/agent.sh" start second openapi-client >/dev/null 2>&1; then
	echo 'agent-harness-test: conflicting claim was accepted' >&2
	exit 1
fi
if LOOMARR_DEV_PORT=8080 LOOMARR_REPO_ROOT="$TMP-wt" "$SCRIPT_DIR/agent.sh" start second visual-baselines >/dev/null 2>&1; then
	echo 'agent-harness-test: conflicting runtime port was accepted' >&2
	exit 1
fi
LOOMARR_REPO_ROOT="$TMP-wt" "$SCRIPT_DIR/agent.sh" start second visual-baselines >/dev/null
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" stop >/dev/null
LOOMARR_REPO_ROOT="$TMP-wt" "$SCRIPT_DIR/agent.sh" stop >/dev/null

# A killed writer cannot leave every future agent waiting on an ownerless registry lock.
step 'abandoned registry lock recovery'
state_dir="$(git -C "$TMP" rev-parse --path-format=absolute --git-common-dir)/loomarr-agents"
mkdir -p "$state_dir/lock"
printf 'pid=999999\ncreated=1\n' > "$state_dir/lock/owner"
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" start after-stale-lock agent-contract >/dev/null
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" stop >/dev/null
mkdir -p "$state_dir/lock"
touch -t 197001010000 "$state_dir/lock"
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" start after-ownerless-lock agent-contract >/dev/null
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" stop >/dev/null

# Leases can be extended explicitly, and abandoned entries can be pruned without touching worktrees.
step 'lease renewal and pruning'
AGENT_LEASE_HOURS=1 LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" start renewable agent-contract >/dev/null
session_id="$(printf '%s' "$TMP" | cksum | awk '{print $1}')"
session_path="$(git -C "$TMP" rev-parse --path-format=absolute --git-common-dir)/loomarr-agents/sessions/$session_id"
old_expiry="$(sed -n 's/^expires=//p' "$session_path")"
AGENT_LEASE_HOURS=2 LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" renew >/dev/null
new_expiry="$(sed -n 's/^expires=//p' "$session_path")"
[ "$new_expiry" -gt "$old_expiry" ]
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" stop >/dev/null

AGENT_LEASE_HOURS=0 LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" start abandoned agent-contract >/dev/null
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" prune | grep -q 'removed 1 expired session'
if LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" status | grep abandoned >/dev/null; then
	echo 'agent-harness-test: prune retained an expired session' >&2
	exit 1
fi

# One clean-commit baseline is shared through the common Git directory.
step 'shared baseline cache'
# shellcheck disable=SC2016 # BASELINE_LOG must expand when the generated fixture executes.
printf '%s\n' '#!/usr/bin/env sh' 'echo run >> "$BASELINE_LOG"' > "$TMP/fake-bin/make"
chmod +x "$TMP/fake-bin/make"
baseline_log="$TMP/baseline-runs"
PATH="$TMP/fake-bin:$PATH" BASELINE_LOG="$baseline_log" LOOMARR_REPO_ROOT="$TMP" \
	"$SCRIPT_DIR/agent.sh" baseline >/dev/null
PATH="$TMP/fake-bin:$PATH" BASELINE_LOG="$baseline_log" LOOMARR_REPO_ROOT="$TMP-wt" \
	"$SCRIPT_DIR/agent.sh" baseline >/dev/null
[ "$(wc -l < "$baseline_log" | tr -d ' ')" = 1 ]

# A gate that overlaps an edit cannot publish a green cache entry for the clean commit it started
# from. This catches agents beginning implementation while their required baseline is still running.
step 'baseline rejects concurrent edits'
git -C "$TMP" commit --allow-empty -qm 'new baseline key'
# shellcheck disable=SC2016 # LOOMARR_REPO_ROOT expands inside the generated fixture.
printf '%s\n' '#!/usr/bin/env sh' 'printf "changed during baseline\n" >> "$LOOMARR_REPO_ROOT/fixture"' > "$TMP/fake-bin/make"
chmod +x "$TMP/fake-bin/make"
if PATH="$TMP/fake-bin:$PATH" LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" baseline >/dev/null 2>&1; then
	echo 'agent-harness-test: baseline cached a worktree that changed during the gate' >&2
	exit 1
fi
git -C "$TMP" restore fixture

# Focused verification uses the shared classifier and the reverse-dependency closure rather than
# testing only the directory that happened to contain the edited file. Fake only the expensive
# command execution; the real classifiers still resolve Loomarr's actual package graph.
step 'dependency-aware focused verification'
git -C "$TMP" add -A
git -C "$TMP" commit -qm 'focused verification fixture'
mkdir -p "$TMP/internal/suggest"
printf 'package suggest\n' > "$TMP/internal/suggest/probe.go"
verify_log="$TMP/verify-runs"
real_go="$(command -v go)"
verify_bin="$TMP-wt/verify-bin"
mkdir -p "$verify_bin"
# shellcheck disable=SC2016 # REAL_GO and VERIFY_LOG expand when the generated fixture executes.
printf '%s\n' '#!/usr/bin/env sh' 'if [ "$1" = test ]; then echo "go $*" >> "$VERIFY_LOG"; exit 0; fi' 'exec "$REAL_GO" "$@"' > "$verify_bin/go"
# shellcheck disable=SC2016 # VERIFY_LOG expands when the generated fixture executes.
printf '%s\n' '#!/usr/bin/env sh' 'echo "make $*" >> "$VERIFY_LOG"' > "$verify_bin/make"
chmod +x "$verify_bin/go" "$verify_bin/make"
verify_output="$(PATH="$verify_bin:$PATH" REAL_GO="$real_go" VERIFY_LOG="$verify_log" \
	BASE=HEAD LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" verify)"
printf '%s\n' "$verify_output" | grep -q 'selected gates: contracts,go,postgres,image'
printf '%s\n' "$verify_output" | grep -q 'affected Go packages:'
grep -q 'make -C .* fmt tags-verify' "$verify_log"
grep -q 'go test -race .*internal/app.*internal/suggest' "$verify_log"
rm -rf "$TMP/internal"

# Process ownership is the worktree cwd, not the globally shared process name.
step 'process ownership'
mkdir "$TMP-gone"
cp "$(command -v sleep)" "$TMP-gone/loomarr-dev"
( cd "$TMP-gone" && exec ./loomarr-dev 30 ) & stale_pid=$!; pids="$pids $stale_pid"
sleep 0.05
rm -rf "$TMP-gone"
cp "$(command -v sleep)" "$TMP/loomarr-dev"
cp "$(command -v sleep)" "$TMP-wt/loomarr-dev"
( cd "$TMP" && exec ./loomarr-dev 30 ) & first_pid=$!; pids="$pids $first_pid"
( cd "$TMP-wt" && exec ./loomarr-dev 30 ) & second_pid=$!; pids="$pids $second_pid"
# shellcheck disable=SC1091 # SCRIPT_DIR is resolved at runtime.
. "$SCRIPT_DIR/dev-processes.sh"
wait_for_repo_pid "$first_pid" loomarr-dev "$TMP"
wait_for_repo_pid "$second_pid" loomarr-dev "$TMP-wt"

# Worktree creation is product-neutral and does not copy credentials by default.
step 'claimed worktree creation and dependency visibility'
printf 'SECRET=fixture\n' > "$TMP/.env"
BASE=HEAD WORKTREE_PATH="$TMP-third" AGENT_WORKTREE_SKIP_BOOTSTRAP=1 LOOMARR_REPO_ROOT="$TMP" \
	"$SCRIPT_DIR/agent.sh" worktree third third-task agent-contract >/dev/null
[ -d "$TMP-third" ]
[ ! -e "$TMP-third/.env" ]
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" status | grep 'third-task.*agent-contract' >/dev/null

BASE=third WORKTREE_PATH="$TMP-fourth" AGENT_WORKTREE_SKIP_BOOTSTRAP=1 LOOMARR_REPO_ROOT="$TMP" \
	"$SCRIPT_DIR/agent.sh" worktree fourth fourth-task '' third-task >/dev/null
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" status | grep 'fourth-task.*third-task' >/dev/null

if AGENT_WORKTREE_LIMIT=3 BASE=HEAD WORKTREE_PATH="$TMP-fifth" AGENT_WORKTREE_SKIP_BOOTSTRAP=1 \
	LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" worktree fifth fifth-task '' >/dev/null 2>&1; then
	echo 'agent-harness-test: worktree backlog limit was not enforced' >&2
	exit 1
fi
ALLOW_WORKTREE_BACKLOG=1 AGENT_WORKTREE_LIMIT=3 BASE=HEAD WORKTREE_PATH="$TMP-fifth" \
	AGENT_WORKTREE_SKIP_BOOTSTRAP=1 LOOMARR_REPO_ROOT="$TMP" \
	"$SCRIPT_DIR/agent.sh" worktree fifth fifth-task '' >/dev/null

"$SCRIPT_DIR/agent-worktree-gc-test.sh" >/dev/null

echo 'agent-harness-test: ok'
