#!/usr/bin/env sh
# Tool-neutral interface for local agent sessions and worktrees.

set -eu

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
ROOT="${LOOMARR_REPO_ROOT:-$(CDPATH='' cd -- "$SCRIPT_DIR/.." && pwd -P)}"
ROOT="$(CDPATH='' cd -- "$ROOT" && pwd -P)"

common_dir() {
	git -C "$ROOT" rev-parse --path-format=absolute --git-common-dir
}

state_dir() {
	printf '%s/loomarr-agents' "$(common_dir)"
}

session_key() {
	printf '%s' "$ROOT" | cksum | awk '{print $1}'
}

session_file() {
	printf '%s/sessions/%s' "$(state_dir)" "$(session_key)"
}

field() {
	name="$1"
	file="$2"
	sed -n "s/^$name=//p" "$file" | head -1
}

valid_name() {
	case "$1" in
		''|*[!A-Za-z0-9._/-]*) return 1 ;;
	esac
}

normalise_claims() {
	printf '%s' "$1" | tr ',' '\n' | sed '/^$/d' | sort -u | paste -sd, -
}

claims_overlap() {
	left="$1"
	right="$2"
	[ -z "$left" ] || [ -z "$right" ] && return 1
	[ "$left" = '*' ] || [ "$right" = '*' ] && return 0
	old_ifs=$IFS
	IFS=,
	for a in $left; do
		for b in $right; do
			[ "$a" = "$b" ] && { IFS=$old_ifs; return 0; }
		done
	done
	IFS=$old_ifs
	return 1
}

acquire_lock() {
	STATE="$(state_dir)"
	mkdir -p "$STATE/sessions" "$STATE/baselines"
	LOCK="$STATE/lock"
	i=0
	until mkdir "$LOCK" 2>/dev/null; do
		i=$((i + 1))
		[ "$i" -ge 100 ] && { echo "agent: timed out waiting for registry lock" >&2; exit 1; }
		sleep 0.1
	done
	trap 'rmdir "$LOCK" 2>/dev/null || true' EXIT INT TERM
}

release_lock() {
	rmdir "$LOCK" 2>/dev/null || true
	trap - EXIT INT TERM
}

start_session() {
	task="${1:-}"
	claims="$(normalise_claims "${2:-}")"
	valid_name "$task" || { echo "agent-start: TASK is required and may contain letters, numbers, . _ / -" >&2; exit 2; }
	for claim in $(printf '%s' "$claims" | tr ',' ' '); do
		valid_name "$claim" || { echo "agent-start: invalid claim: $claim" >&2; exit 2; }
	done

	acquire_lock
	now="$(date +%s)"
	lease_hours="${AGENT_LEASE_HOURS:-12}"
	case "$lease_hours" in ''|*[!0-9]*) echo 'agent-start: AGENT_LEASE_HOURS must be an integer' >&2; release_lock; exit 2 ;; esac
	expires=$((now + lease_hours * 3600))
	mine="$(session_file)"
	eval "$("$SCRIPT_DIR/dev-env.sh" export)"
	for file in "$STATE"/sessions/*; do
		[ -f "$file" ] || continue
		[ "$file" = "$mine" ] && continue
		other_expires="$(field expires "$file")"
		[ -n "$other_expires" ] && [ "$other_expires" -lt "$now" ] && { rm -f "$file"; continue; }
		other_claims="$(field claims "$file")"
		if claims_overlap "$claims" "$other_claims"; then
			echo "agent-start: claim conflict with task $(field task "$file") in $(field worktree "$file"): $other_claims" >&2
			release_lock
			exit 1
		fi
		for port_name in backend frontend storybook tunarr; do
			other_port="$(field "$port_name" "$file")"
			case "$port_name" in
				backend) this_port="$LOOMARR_DEV_PORT" ;;
				frontend) this_port="$LOOMARR_FE_PORT" ;;
				storybook) this_port="$LOOMARR_STORYBOOK_PORT" ;;
				tunarr) this_port="$TUNARR_DEV_PORT" ;;
			esac
			if [ -n "$other_port" ] && [ "$other_port" = "$this_port" ]; then
				echo "agent-start: $port_name port $this_port conflicts with task $(field task "$file")" >&2
				release_lock
				exit 1
			fi
		done
	done

	branch="$(git -C "$ROOT" symbolic-ref --quiet --short HEAD 2>/dev/null || printf detached)"
	tmp="$mine.tmp.$$"
	{
		printf 'task=%s\n' "$task"
		printf 'claims=%s\n' "$claims"
		printf 'worktree=%s\n' "$ROOT"
		printf 'branch=%s\n' "$branch"
		printf 'instance=%s\n' "$LOOMARR_INSTANCE"
		printf 'backend=%s\n' "$LOOMARR_DEV_PORT"
		printf 'frontend=%s\n' "$LOOMARR_FE_PORT"
		printf 'storybook=%s\n' "$LOOMARR_STORYBOOK_PORT"
		printf 'tunarr=%s\n' "$TUNARR_DEV_PORT"
		printf 'started=%s\n' "$now"
		printf 'expires=%s\n' "$expires"
	} > "$tmp"
	mv "$tmp" "$mine"
	release_lock
	echo "agent-start: registered $task ($branch)"
	[ -n "$claims" ] && echo "agent-start: claimed $claims"
	"$SCRIPT_DIR/dev-env.sh" show
}

status_sessions() {
	dir="$(state_dir)/sessions"
	now="$(date +%s)"
	printf '%-24s %-22s %-20s %s\n' TASK BRANCH CLAIMS WORKTREE
	printf '%-24s %-22s %-20s %s\n' '------------------------' '----------------------' '--------------------' '--------'
	found=0
	for file in "$dir"/*; do
		[ -f "$file" ] || continue
		found=1
		task="$(field task "$file")"
		expires="$(field expires "$file")"
		[ -n "$expires" ] && [ "$expires" -lt "$now" ] && task="$task [expired]"
		printf '%-24s %-22s %-20s %s\n' "$task" "$(field branch "$file")" "$(field claims "$file")" "$(field worktree "$file")"
	done
	[ "$found" -eq 1 ] || echo '(no registered sessions)'
}

stop_session() {
	acquire_lock
	file="$(session_file)"
	if [ -f "$file" ]; then
		task="$(field task "$file")"
		rm -f "$file"
		echo "agent-stop: released $task"
	else
		echo "agent-stop: this worktree has no registered session"
	fi
	release_lock
}

renew_session() {
	lease_hours="${AGENT_LEASE_HOURS:-12}"
	case "$lease_hours" in ''|*[!0-9]*) echo 'agent-renew: AGENT_LEASE_HOURS must be an integer' >&2; exit 2 ;; esac
	acquire_lock
	file="$(session_file)"
	if [ ! -f "$file" ]; then
		echo 'agent-renew: this worktree has no registered session' >&2
		release_lock
		exit 1
	fi
	expires=$(($(date +%s) + lease_hours * 3600))
	tmp="$file.tmp.$$"
	sed "s/^expires=.*/expires=$expires/" "$file" > "$tmp"
	mv "$tmp" "$file"
	task="$(field task "$file")"
	release_lock
	echo "agent-renew: renewed $task for $lease_hours hours"
}

prune_sessions() {
	acquire_lock
	now="$(date +%s)"
	count=0
	for file in "$STATE"/sessions/*; do
		[ -f "$file" ] || continue
		expires="$(field expires "$file")"
		case "$expires" in ''|*[!0-9]*) continue ;; esac
		if [ "$expires" -le "$now" ]; then
			rm -f "$file"
			count=$((count + 1))
		fi
	done
	release_lock
	echo "agent-prune: removed $count expired session(s)"
}

baseline() {
	if [ -n "$(git -C "$ROOT" status --porcelain --untracked-files=no)" ]; then
		echo "agent-baseline: tracked files are dirty; running the gate without caching"
		exec make -C "$ROOT" check
	fi
	state="$(state_dir)/baselines"
	mkdir -p "$state"
	toolchain="$(go version) $(rustc --version) $(uname -s)-$(uname -m)"
	key="$(printf '%s\n%s' "$(git -C "$ROOT" rev-parse HEAD)" "$toolchain" | cksum | awk '{print $1}')"
	stamp="$state/$key.ok"
	lock="$state/$key.lock"
	if [ -f "$stamp" ]; then
		echo "agent-baseline: reusing green make check for $(git -C "$ROOT" rev-parse --short HEAD)"
		return
	fi
	if mkdir "$lock" 2>/dev/null; then
		trap 'rmdir "$lock" 2>/dev/null || true' EXIT INT TERM
		echo "agent-baseline: no cached result; running make check"
		if make -C "$ROOT" check; then
			printf '%s\n' "$toolchain" > "$stamp"
			rmdir "$lock"
			trap - EXIT INT TERM
			return
		fi
		return 1
	fi
	echo "agent-baseline: another worktree is proving this commit; waiting"
	while [ -d "$lock" ]; do sleep 2; done
	[ -f "$stamp" ] || { echo "agent-baseline: the shared gate failed" >&2; exit 1; }
	echo "agent-baseline: shared gate is green"
}

tool_version() {
	label="$1"
	shift
	if command -v "$1" >/dev/null 2>&1; then
		printf '  %-14s %s\n' "$label" "$("$@" 2>&1 | head -1)"
	else
		printf '  %-14s MISSING\n' "$label"
		return 1
	fi
}

doctor() {
	fail=0
	echo 'Required toolchain:'
	tool_version Go go version || fail=1
	tool_version Rust rustc --version || fail=1
	tool_version Cargo cargo --version || fail=1
	tool_version Node node --version || fail=1
	tool_version pnpm pnpm --version || fail=1
	tool_version Docker docker --version || fail=1
	tool_version shellcheck shellcheck --version || fail=1
	tool_version ffmpeg ffmpeg -version || true

	if command -v node >/dev/null 2>&1; then
		node_major="$(node -p 'process.versions.node.split(".")[0]')"
		[ "$node_major" = 22 ] || { echo "  ERROR: Node 22 is required for CI parity (found $node_major)"; fail=1; }
	fi
	if command -v pnpm >/dev/null 2>&1; then
		[ "$(pnpm --version)" = 11.13.1 ] || { echo "  ERROR: pnpm 11.13.1 is required"; fail=1; }
	fi
	if command -v rustc >/dev/null 2>&1; then
		case "$(rustc --version)" in rustc\ 1.93.*) ;; *) echo "  ERROR: Rust 1.93.x is required (see rust-toolchain.toml)"; fail=1 ;; esac
	fi

	echo
	"$SCRIPT_DIR/dev-env.sh" show
	echo
	echo "Registered worktrees: $(git -C "$ROOT" worktree list --porcelain | grep -c '^worktree ')"
	git -C "$ROOT" worktree list --porcelain | awk '
		/^worktree / { path=$2 }
		/^branch refs\/heads\/main$/ && NR > 2 { print "  WARNING: main is parked at " path }
	'
	[ -d "$HOME/.cache/go-build" ] && echo "Go build cache: $(du -sh "$HOME/.cache/go-build" 2>/dev/null | awk '{print $1}')"
	[ -d "$ROOT/.filler-smoke" ] && echo "Ignored smoke artifacts: $(du -sh "$ROOT/.filler-smoke" 2>/dev/null | awk '{print $1}')"
	root_artifacts="$(git -C "$ROOT" ls-files --others --exclude-standard -- '*.png' '*.jpg' | awk 'index($0,"/")==0' | wc -l | tr -d ' ')"
	[ "$root_artifacts" -eq 0 ] || echo "  WARNING: $root_artifacts image artifacts are in the repo root; use $ROOT/.artifacts/"
	return "$fail"
}

bootstrap() {
	command -v cargo >/dev/null 2>&1 || { echo 'bootstrap: cargo is required' >&2; exit 1; }
	( cd "$ROOT" && LOOMARR_RELEASE=dev cargo build --locked -p loomarr-image )
	if [ "${BOOTSTRAP_SKIP_FE:-0}" != 1 ]; then
		command -v pnpm >/dev/null 2>&1 || { echo 'bootstrap: pnpm is required' >&2; exit 1; }
		( cd "$ROOT/web" && pnpm install --frozen-lockfile && pnpm codegen )
	fi
	mkdir -p "$ROOT/.artifacts" "$ROOT/.agent-data"
	eval "$("$SCRIPT_DIR/dev-env.sh" export)"
	if [ -n "$LOOMARR_AGENT_DATABASE_URL" ] && [ "${AGENT_DEV_IDENTITY:-1}" = 1 ]; then
		DATABASE_URL="$LOOMARR_AGENT_DATABASE_URL" go run ./cmd/dev-bootstrap
	elif [ -n "$LOOMARR_AGENT_DATABASE_URL" ]; then
		echo 'dev-bootstrap: skipped (AGENT_DEV_IDENTITY=0)'
	fi
	echo 'bootstrap: ready'
	"$SCRIPT_DIR/dev-env.sh" show
}

worktree() {
	topic="${1:-}"
	valid_name "$topic" || { echo 'agent-worktree: TOPIC is required' >&2; exit 2; }
	primary="$(git -C "$ROOT" worktree list --porcelain | sed -n 's/^worktree //p' | head -1)"
	slug="$(printf '%s' "$topic" | tr '/' '-' | tr -cs 'A-Za-z0-9._-' '-')"
	target="${WORKTREE_PATH:-$(dirname "$primary")/$(basename "$primary")-$slug}"
	[ ! -e "$target" ] || { echo "agent-worktree: target exists: $target" >&2; exit 1; }
	git -C "$primary" show-ref --verify --quiet "refs/heads/$topic" && { echo "agent-worktree: branch exists: $topic" >&2; exit 1; }

	# Branch off origin/main by default, NOT the primary worktree's HEAD. A new worktree is almost
	# always the start of a fresh PR, and branching off HEAD silently inherits whatever branch the
	# primary happens to be parked on — a stale or unrelated one — so the new work sits on the wrong
	# base and every rebase fights history it never meant to touch. Fetching first also picks up
	# merges that landed while you worked. `BASE=HEAD` (or BASE=<ref>) opts into deliberate stacking
	# on the current branch or an explicit ref. Same BASE convention as `make agent-verify`.
	#
	# The origin/main default degrades gracefully: an offline dev, a fresh clone mid-fetch, or a repo
	# whose remote is named differently (or a hermetic test repo with no remote) all fall back to HEAD
	# rather than failing — HEAD is the only base guaranteed to exist. An EXPLICIT, unresolvable BASE
	# is still a hard error, because the caller named a specific ref and silently ignoring it would be
	# the surprising thing.
	base="${BASE:-origin/main}"
	if [ -z "${BASE:-}" ]; then
		# Best-effort refresh of the default base only; never fetch on an explicit BASE.
		git -C "$ROOT" fetch --quiet origin main 2>/dev/null || true
		if ! git -C "$ROOT" rev-parse --verify --quiet "$base^{commit}" >/dev/null; then
			echo "agent-worktree: origin/main not available; falling back to HEAD" >&2
			base=HEAD
		fi
	elif ! git -C "$ROOT" rev-parse --verify --quiet "$base^{commit}" >/dev/null; then
		echo "agent-worktree: unknown BASE=$base (want a ref like origin/main or HEAD)" >&2
		exit 2
	fi
	echo "agent-worktree: branching $topic off $base ($(git -C "$ROOT" rev-parse --short "$base"))" >&2
	git -C "$ROOT" worktree add "$target" -b "$topic" "$base"
	if [ "${COPY_ENV:-0}" = 1 ] && [ -f "$primary/.env" ]; then
		cp "$primary/.env" "$target/.env"
		chmod 600 "$target/.env"
	fi
	if [ "${AGENT_WORKTREE_SKIP_BOOTSTRAP:-0}" != 1 ]; then
		BOOTSTRAP_SKIP_FE="${BOOTSTRAP_SKIP_FE:-0}" LOOMARR_REPO_ROOT="$target" "$target/scripts/agent.sh" bootstrap
	fi
	echo "agent-worktree: created $target"
}

verify_changed() {
	base="${BASE:-origin/main}"
	git -C "$ROOT" rev-parse --verify "$base^{commit}" >/dev/null 2>&1 || { echo "agent-verify: unknown BASE=$base" >&2; exit 2; }
	changed="$( { git -C "$ROOT" diff --name-only "$base"...HEAD; git -C "$ROOT" diff --name-only; git -C "$ROOT" ls-files --others --exclude-standard; } | sort -u )"
	[ -n "$changed" ] || { echo 'agent-verify: no changes'; return; }
	echo 'agent-verify: focused checks only; make check remains the final gate'
	printf '%s\n' "$changed"
	scope="$(printf '%s\n' "$changed" | "$SCRIPT_DIR/ci-impact.sh")"
	selected="$(printf '%s\n' "$scope" | sed -n 's/=true$//p' | paste -sd, -)"
	echo "agent-verify: selected gates: $selected"

	if printf '%s\n' "$scope" | grep -qx 'contracts=true'; then
		if printf '%s\n' "$changed" | grep -qE '\.go$|^go\.(mod|sum)$'; then
			make -C "$ROOT" fmt tags-verify
		fi
		if printf '%s\n' "$changed" | grep -qE '^scripts/.*\.sh$'; then
			make -C "$ROOT" shellcheck
		fi
		if printf '%s\n' "$changed" | grep -q '^docs/help/'; then
			make -C "$ROOT" retired-verify
		fi
	fi
	if printf '%s\n' "$scope" | grep -qx 'agent=true'; then
		make -C "$ROOT" agent-harness-test
	fi
	if printf '%s\n' "$scope" | grep -qx 'go=true'; then
		packages="$(printf '%s\n' "$changed" | "$SCRIPT_DIR/go-impact.sh")"
		[ -n "$packages" ] || { echo 'agent-verify: Go selected but package closure is empty' >&2; exit 1; }
		echo "agent-verify: affected Go packages: $(printf '%s\n' "$packages" | wc -l | tr -d ' ')"
		traced="$(printf '%s\n' "$packages" | "$SCRIPT_DIR/go-race-policy.sh" --race)"
		untraced="$(printf '%s\n' "$packages" | "$SCRIPT_DIR/go-race-policy.sh" --no-race)"
		if [ -n "$traced" ]; then
			# shellcheck disable=SC2086 # newline-delimited package list intentionally becomes argv.
			( cd "$ROOT" && go test -race -timeout 25m $traced )
		fi
		if [ -n "$untraced" ]; then
			# shellcheck disable=SC2086 # newline-delimited package list intentionally becomes argv.
			( cd "$ROOT" && go test -timeout 25m $untraced )
		fi
	fi
	if printf '%s\n' "$scope" | grep -qx 'rust=true'; then
		make -C "$ROOT" rust-check
	fi
	if printf '%s\n' "$scope" | grep -qx 'web=true'; then
		( cd "$ROOT/web" && pnpm codegen && pnpm lint && pnpm -r --parallel typecheck )
	fi
	if printf '%s\n' "$scope" | grep -qx 'docs=true'; then
		make -C "$ROOT" docs-lint
	fi
	if printf '%s\n' "$scope" | grep -qx 'android=true'; then
		make -C "$ROOT" android
	fi
}

case "${1:-}" in
	start) shift; start_session "${1:-}" "${2:-}" ;;
	status) status_sessions ;;
	renew) renew_session ;;
	prune) prune_sessions ;;
	stop) stop_session ;;
	env) "$SCRIPT_DIR/dev-env.sh" show ;;
	baseline) baseline ;;
	doctor) doctor ;;
	bootstrap) bootstrap ;;
	worktree) shift; worktree "${1:-}" ;;
	verify) verify_changed ;;
	*)
		echo 'usage: scripts/agent.sh {start TASK [CLAIMS]|status|renew|prune|stop|env|baseline|doctor|bootstrap|worktree TOPIC|verify}' >&2
		exit 2
		;;
esac
