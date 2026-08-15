#!/usr/bin/env sh

set -eu

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
TMP="$(mktemp -d)"
TMP="$(CDPATH='' cd -- "$TMP" && pwd -P)"
pids=
# shellcheck disable=SC2154 # pid is the loop variable inside the trap evaluated at exit.
trap 'for pid in $pids; do kill "$pid" 2>/dev/null || true; done; rm -rf "$TMP" "$TMP-wt" "$TMP-third"' EXIT INT TERM

step() {
	echo "agent-harness-test: $*"
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
secondary_exports="$(LOOMARR_REPO_ROOT="$TMP-wt" "$SCRIPT_DIR/dev-env.sh" export)"
printf '%s\n' "$secondary_exports" | grep -q "LOOMARR_AGENT_PREPARED_DIR=.*\.agent-data/prepared"
grep -q 'PLAYOUT_PREPARED_DIR=.*LOOMARR_AGENT_PREPARED_DIR' "$SCRIPT_DIR/../.air.toml"
if printf '%s\n' "$secondary" | grep -q 'http://localhost:8080'; then
	echo 'agent-harness-test: secondary worktree reused the primary backend port' >&2
	exit 1
fi

overridden="$(LOOMARR_REPO_ROOT="$TMP-wt" LOOMARR_DEV_PORT=23456 "$SCRIPT_DIR/dev-env.sh" show)"
printf '%s\n' "$overridden" | grep -q 'http://localhost:23456'

step 'claims and port conflicts'
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" start first openapi-client >/dev/null
LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" status | grep -q 'first.*openapi-client'
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
if LOOMARR_REPO_ROOT="$TMP" "$SCRIPT_DIR/agent.sh" status | grep -q abandoned; then
	echo 'agent-harness-test: prune retained an expired session' >&2
	exit 1
fi

# One clean-commit baseline is shared through the common Git directory.
step 'shared baseline cache'
mkdir -p "$TMP/fake-bin"
# shellcheck disable=SC2016 # BASELINE_LOG must expand when the generated fixture executes.
printf '%s\n' '#!/usr/bin/env sh' 'echo run >> "$BASELINE_LOG"' > "$TMP/fake-bin/make"
chmod +x "$TMP/fake-bin/make"
baseline_log="$TMP/baseline-runs"
PATH="$TMP/fake-bin:$PATH" BASELINE_LOG="$baseline_log" LOOMARR_REPO_ROOT="$TMP" \
	"$SCRIPT_DIR/agent.sh" baseline >/dev/null
PATH="$TMP/fake-bin:$PATH" BASELINE_LOG="$baseline_log" LOOMARR_REPO_ROOT="$TMP-wt" \
	"$SCRIPT_DIR/agent.sh" baseline >/dev/null
[ "$(wc -l < "$baseline_log" | tr -d ' ')" = 1 ]

# Process ownership is the worktree cwd, not the globally shared process name.
step 'process ownership'
cp "$(command -v sleep)" "$TMP/loomarr-dev"
cp "$(command -v sleep)" "$TMP-wt/loomarr-dev"
( cd "$TMP" && exec ./loomarr-dev 30 ) & first_pid=$!; pids="$pids $first_pid"
( cd "$TMP-wt" && exec ./loomarr-dev 30 ) & second_pid=$!; pids="$pids $second_pid"
sleep 0.1
# shellcheck disable=SC1091 # SCRIPT_DIR is resolved at runtime.
. "$SCRIPT_DIR/dev-processes.sh"
[ "$(repo_pids_by_comm loomarr-dev "$TMP")" = "$first_pid" ]
[ "$(repo_pids_by_comm loomarr-dev "$TMP-wt")" = "$second_pid" ]

# Worktree creation is product-neutral and does not copy credentials by default.
step 'worktree creation'
printf 'SECRET=fixture\n' > "$TMP/.env"
WORKTREE_PATH="$TMP-third" AGENT_WORKTREE_SKIP_BOOTSTRAP=1 LOOMARR_REPO_ROOT="$TMP" \
	"$SCRIPT_DIR/agent.sh" worktree third >/dev/null
[ -d "$TMP-third" ]
[ ! -e "$TMP-third/.env" ]

echo 'agent-harness-test: ok'
