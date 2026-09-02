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

exported_port() {
	case "$1" in
		backend) name=LOOMARR_DEV_PORT ;;
		frontend) name=LOOMARR_FE_PORT ;;
		storybook) name=LOOMARR_STORYBOOK_PORT ;;
		tunarr) name=TUNARR_DEV_PORT ;;
		prometheus) name=PROMETHEUS_DEV_PORT ;;
		grafana) name=GRAFANA_DEV_PORT ;;
	esac
	printf '%s\n' "$candidate_exports" | sed -n "s/export $name='\([0-9][0-9]*\)'/\1/p"
}

port_was_explicit() {
	case "$1" in
		backend) [ "$explicit_backend" -eq 1 ] ;;
		frontend) [ "$explicit_frontend" -eq 1 ] ;;
		storybook) [ "$explicit_storybook" -eq 1 ] ;;
		tunarr) [ "$explicit_tunarr" -eq 1 ] ;;
		prometheus) [ "$explicit_prometheus" -eq 1 ] ;;
		grafana) [ "$explicit_grafana" -eq 1 ] ;;
	esac
}

acquire_lock() {
	STATE="$(state_dir)"
	mkdir -p "$STATE/sessions" "$STATE/baselines"
	LOCK="$STATE/lock"
	i=0
	until mkdir "$LOCK" 2>/dev/null; do
		owner_pid="$(field pid "$LOCK/owner" 2>/dev/null || true)"
		created="$(field created "$LOCK/owner" 2>/dev/null || true)"
		# A process can die in the few instructions between mkdir and writing owner. The
		# directory timestamp gives that ownerless lock the same bounded recovery path.
		case "$created" in
			''|*[!0-9]*) created="$(stat -c %Y "$LOCK" 2>/dev/null || stat -f %m "$LOCK" 2>/dev/null || true)" ;;
		esac
		now="$(date +%s)"
		case "$created" in
			''|*[!0-9]*) ;;
			*)
				# Registry mutations take milliseconds. Recover only an abandoned lock whose
				# process is gone and whose age proves this is not a writer between mkdir and
				# publishing its owner file.
				if [ $((now - created)) -ge 30 ]; then
					case "$owner_pid" in
						''|*[!0-9]*) abandoned=1 ;;
						*) kill -0 "$owner_pid" 2>/dev/null && abandoned=0 || abandoned=1 ;;
					esac
					if [ "$abandoned" -eq 1 ]; then
						rm -rf "$LOCK"
						continue
					fi
				fi
				;;
		esac
		i=$((i + 1))
		[ "$i" -ge 300 ] && {
			echo "agent: timed out waiting for registry lock (owner pid=${owner_pid:-unknown}, created=${created:-unknown})" >&2
			exit 1
		}
		sleep 0.1
	done
	{
		printf 'pid=%s\n' "$$"
		printf 'created=%s\n' "$(date +%s)"
	} > "$LOCK/owner"
	trap 'rm -f "$LOCK/owner"; rmdir "$LOCK" 2>/dev/null || true' EXIT INT TERM
}

release_lock() {
	rm -f "$LOCK/owner"
	rmdir "$LOCK" 2>/dev/null || true
	trap - EXIT INT TERM
}

start_session() {
	task="${1:-}"
	claims="$(normalise_claims "${2:-}")"
	depends_on="${3:-${DEPENDS_ON:-}}"
	valid_name "$task" || { echo "agent-start: TASK is required and may contain letters, numbers, . _ / -" >&2; exit 2; }
	[ -z "$depends_on" ] || valid_name "$depends_on" || { echo "agent-start: invalid DEPENDS_ON: $depends_on" >&2; exit 2; }
	[ "$depends_on" != "$task" ] || { echo "agent-start: TASK cannot depend on itself" >&2; exit 2; }
	for claim in $(printf '%s' "$claims" | tr ',' ' '); do
		valid_name "$claim" || { echo "agent-start: invalid claim: $claim" >&2; exit 2; }
	done
	explicit_backend=0; [ "${LOOMARR_DEV_PORT+x}" = x ] && explicit_backend=1
	explicit_frontend=0; [ "${LOOMARR_FE_PORT+x}" = x ] && explicit_frontend=1
	explicit_storybook=0; [ "${LOOMARR_STORYBOOK_PORT+x}" = x ] && explicit_storybook=1
	explicit_tunarr=0; [ "${TUNARR_DEV_PORT+x}" = x ] && explicit_tunarr=1
	explicit_prometheus=0; [ "${PROMETHEUS_DEV_PORT+x}" = x ] && explicit_prometheus=1
	explicit_grafana=0; [ "${GRAFANA_DEV_PORT+x}" = x ] && explicit_grafana=1

	acquire_lock
	now="$(date +%s)"
	lease_hours="${AGENT_LEASE_HOURS:-4}"
	case "$lease_hours" in ''|*[!0-9]*) echo 'agent-start: AGENT_LEASE_HOURS must be an integer' >&2; release_lock; exit 2 ;; esac
	expires=$((now + lease_hours * 3600))
	mine="$(session_file)"
	initial_exports="$("$SCRIPT_DIR/dev-env.sh" export)"
	initial_slot="$(printf '%s\n' "$initial_exports" | sed -n "s/export LOOMARR_AGENT_SLOT='\([0-9][0-9]*\)'/\1/p")"
	dependency_branch=
	for file in "$STATE"/sessions/*; do
		[ -f "$file" ] || continue
		[ "$file" = "$mine" ] && continue
		other_expires="$(field expires "$file")"
		[ -n "$other_expires" ] && [ "$other_expires" -le "$now" ] && { rm -f "$file"; continue; }
		other_task="$(field task "$file")"
		if [ "$other_task" = "$task" ]; then
			echo "agent-start: task $task is already active in $(field worktree "$file")" >&2
			release_lock
			exit 1
		fi
		[ "$other_task" != "$depends_on" ] || dependency_branch="$(field branch "$file")"
		other_claims="$(field claims "$file")"
		if claims_overlap "$claims" "$other_claims"; then
			echo "agent-start: claim conflict with task $(field task "$file") in $(field worktree "$file"): $other_claims" >&2
			release_lock
			exit 1
		fi
	done
	if [ -n "$depends_on" ]; then
		if [ -z "$dependency_branch" ]; then
			echo "agent-start: dependency task $depends_on is not active" >&2
			release_lock
			exit 1
		fi
		if ! git -C "$ROOT" merge-base --is-ancestor "$dependency_branch" HEAD 2>/dev/null; then
			echo "agent-start: $task depends on $depends_on, but HEAD is not stacked on branch $dependency_branch; create it with BASE=$dependency_branch" >&2
			release_lock
			exit 1
		fi
	fi

	primary="$(git -C "$ROOT" worktree list --porcelain | sed -n 's/^worktree //p' | head -1)"
	primary="$(CDPATH='' cd -- "$primary" && pwd -P)"
	# Selection and the session-file rename both happen while the registry lock is held. Concurrent
	# starts therefore see either the old registry or the completed allocation, never the same free
	# candidate. Keep candidate exports inert until one wins so a rejected probe cannot become the
	# next probe's apparent explicit override.
	candidate="$initial_slot"
	attempt=0
	while :; do
		candidate_exports="$(LOOMARR_AGENT_SLOT_OVERRIDE="$candidate" "$SCRIPT_DIR/dev-env.sh" export)"
		conflict_name=
		conflict_port=
		conflict_task=
		conflict_explicit=0
		for file in "$STATE"/sessions/*; do
			[ -f "$file" ] || continue
			[ "$file" = "$mine" ] && continue
			for this_name in backend frontend storybook tunarr prometheus grafana; do
				this_port="$(exported_port "$this_name")"
				for other_name in backend frontend storybook tunarr prometheus grafana; do
					other_port="$(field "$other_name" "$file")"
					if [ -n "$other_port" ] && [ "$other_port" = "$this_port" ]; then
						conflict_name="$this_name"
						conflict_port="$this_port"
						conflict_task="$(field task "$file")"
						port_was_explicit "$this_name" && conflict_explicit=1
						break 3
					fi
				done
			done
		done
		if [ -z "$conflict_name" ]; then
			eval "$candidate_exports"
			break
		fi
		if [ "$conflict_explicit" -eq 1 ] || [ "$ROOT" = "$primary" ]; then
			echo "agent-start: $conflict_name port $conflict_port conflicts with task $conflict_task" >&2
			release_lock
			exit 1
		fi
		attempt=$((attempt + 1))
		if [ "$attempt" -ge 900 ]; then
			echo 'agent-start: no automatic worktree port tuple is available' >&2
			release_lock
			exit 1
		fi
		candidate=$((candidate % 900 + 1))
	done

	branch="$(git -C "$ROOT" symbolic-ref --quiet --short HEAD 2>/dev/null || printf detached)"
	tmp="$mine.tmp.$$"
	{
		printf 'task=%s\n' "$task"
		printf 'depends_on=%s\n' "$depends_on"
		printf 'dependency_branch=%s\n' "$dependency_branch"
		printf 'claims=%s\n' "$claims"
		printf 'worktree=%s\n' "$ROOT"
		printf 'branch=%s\n' "$branch"
		printf 'instance=%s\n' "$LOOMARR_INSTANCE"
		printf 'slot=%s\n' "$LOOMARR_AGENT_SLOT"
		printf 'backend=%s\n' "$LOOMARR_DEV_PORT"
		printf 'frontend=%s\n' "$LOOMARR_FE_PORT"
		printf 'storybook=%s\n' "$LOOMARR_STORYBOOK_PORT"
		printf 'tunarr=%s\n' "$TUNARR_DEV_PORT"
		printf 'prometheus=%s\n' "$PROMETHEUS_DEV_PORT"
		printf 'grafana=%s\n' "$GRAFANA_DEV_PORT"
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
	printf '%-24s %-20s %-22s %-20s %-9s %s\n' TASK DEPENDS-ON BRANCH CLAIMS LEASE WORKTREE
	printf '%-24s %-20s %-22s %-20s %-9s %s\n' '------------------------' '--------------------' '----------------------' '--------------------' '---------' '--------'
	found=0
	for file in "$dir"/*; do
		[ -f "$file" ] || continue
		found=1
		task="$(field task "$file")"
		expires="$(field expires "$file")"
		case "$expires" in
			''|*[!0-9]*) lease=unknown ;;
			*)
				if [ "$expires" -le "$now" ]; then
					lease=expired
				else
					lease="$(((expires - now + 59) / 60))m"
				fi
				;;
		esac
		printf '%-24s %-20s %-22s %-20s %-9s %s\n' "$task" "$(field depends_on "$file")" "$(field branch "$file")" "$(field claims "$file")" "$lease" "$(field worktree "$file")"
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
		echo 'agent-stop: after merge, run make agent-gc to audit retirement eligibility'
	else
		echo "agent-stop: this worktree has no registered session"
	fi
	release_lock
}

renew_session() {
	lease_hours="${AGENT_LEASE_HOURS:-4}"
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
		exec make -C "$ROOT" verify SCOPE=all
	fi
	state="$(state_dir)/baselines"
	mkdir -p "$state"
	toolchain="$(go version) $(rustc --version) $(uname -s)-$(uname -m)"
	start_head="$(git -C "$ROOT" rev-parse HEAD)"
	key="$(printf '%s\n%s' "$start_head" "$toolchain" | cksum | awk '{print $1}')"
	stamp="$state/$key.ok"
	lock="$state/$key.lock"
	if [ -f "$stamp" ]; then
		echo "agent-baseline: reusing green comprehensive verification for $(git -C "$ROOT" rev-parse --short HEAD)"
		return
	fi
	if mkdir "$lock" 2>/dev/null; then
		trap 'rmdir "$lock" 2>/dev/null || true' EXIT INT TERM
		echo "agent-baseline: no cached result; running make verify SCOPE=all"
		if make -C "$ROOT" verify SCOPE=all; then
			if [ "$(git -C "$ROOT" rev-parse HEAD)" != "$start_head" ] ||
				[ -n "$(git -C "$ROOT" status --porcelain --untracked-files=no)" ]; then
				echo "agent-baseline: worktree changed during comprehensive verification; refusing to cache mixed-tree evidence" >&2
				return 1
			fi
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
	docker_available=1
	echo 'Required toolchain:'
	tool_version Go go version || fail=1
	tool_version Rust rustc --version || fail=1
	tool_version Cargo cargo --version || fail=1
	tool_version Node node --version || fail=1
	tool_version pnpm pnpm --version || fail=1
	tool_version Make make --version || fail=1
	tool_version Docker docker --version || { fail=1; docker_available=0; }
	if [ "$docker_available" = 1 ]; then
		tool_version 'Docker Compose' docker compose version || fail=1
	fi
	tool_version shellcheck shellcheck --version || fail=1
	tool_version ffmpeg ffmpeg -version || fail=1
	tool_version ffprobe ffprobe -version || fail=1

	if command -v go >/dev/null 2>&1; then
		go_release="$(go version | sed -n 's/.* go\([0-9][0-9.]*\).*/\1/p')"
		go_major="${go_release%%.*}"
		go_rest="${go_release#*.}"
		go_minor="${go_rest%%.*}"
		case "$go_major:$go_minor" in
			1:27|1:2[89]|1:[3-9][0-9]|[2-9]:*) ;;
			*) echo "  ERROR: Go 1.27+ is required (found ${go_release:-unknown})"; fail=1 ;;
		esac
	fi
	if command -v node >/dev/null 2>&1; then
		node_major="$(node -p 'process.versions.node.split(".")[0]')"
		[ "$node_major" = 22 ] || { echo "  ERROR: Node 22 is required for CI parity (found $node_major)"; fail=1; }
	fi
	if command -v pnpm >/dev/null 2>&1; then
		[ "$(pnpm --version)" = 11.13.1 ] || { echo "  ERROR: pnpm 11.13.1 is required"; fail=1; }
	fi
	if command -v make >/dev/null 2>&1; then
		make_major="$(make --version | sed -n '1s/.* //p' | cut -d. -f1)"
		case "$make_major" in ''|*[!0-9]*|0|1|2|3) echo '  ERROR: GNU Make 4.x is required; macOS users should install Homebrew make and put its gnubin directory on PATH'; fail=1 ;; esac
	fi
	if command -v rustc >/dev/null 2>&1; then
		case "$(rustc --version)" in rustc\ 1.93.*) ;; *) echo "  ERROR: Rust 1.93.x is required (see rust-toolchain.toml)"; fail=1 ;; esac
	fi
	if [ "$docker_available" = 1 ] && ! docker info >/dev/null 2>&1; then
		case "$(uname -s)" in
			Darwin) echo '  ERROR: Docker Desktop is not running; run open -a Docker, wait for it to start, then rerun make doctor' ;;
			*) echo '  ERROR: the Docker daemon is unavailable; start it, then rerun make doctor' ;;
		esac
		fail=1
	fi

	echo
	"$SCRIPT_DIR/dev-env.sh" show
	echo
	worktree_count="$(git -C "$ROOT" worktree list --porcelain | grep -c '^worktree ')"
	echo "Registered worktrees: $worktree_count"
	if [ "$worktree_count" -gt 16 ]; then
		echo '  WARNING: 16 or more secondary worktrees; run make agent-gc'
	fi
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
		( cd "$ROOT" && DATABASE_URL="$LOOMARR_AGENT_DATABASE_URL" go run ./cmd/dev-bootstrap )
	elif [ -n "$LOOMARR_AGENT_DATABASE_URL" ]; then
		echo 'dev-bootstrap: skipped (AGENT_DEV_IDENTITY=0)'
	fi
	echo 'bootstrap: ready'
	"$SCRIPT_DIR/dev-env.sh" show
}

worktree() {
	topic="${1:-}"
	task="${2:-$topic}"
	claims="${3:-}"
	depends_on="${4:-}"
	valid_name "$topic" || { echo 'agent-worktree: TOPIC is required' >&2; exit 2; }
	primary="$(git -C "$ROOT" worktree list --porcelain | sed -n 's/^worktree //p' | head -1)"
	worktree_limit="${AGENT_WORKTREE_LIMIT:-16}"
	case "$worktree_limit" in ''|*[!0-9]*) echo 'agent-worktree: AGENT_WORKTREE_LIMIT must be an integer' >&2; exit 2 ;; esac
	secondary_count="$(git -C "$ROOT" worktree list --porcelain | grep -c '^worktree ' || true)"
	secondary_count=$((secondary_count - 1))
	if [ "$secondary_count" -ge "$worktree_limit" ] && [ "${ALLOW_WORKTREE_BACKLOG:-0}" != 1 ]; then
		echo "agent-worktree: $secondary_count secondary worktrees reached the limit of $worktree_limit" >&2
		echo 'agent-worktree: run make agent-gc, then make agent-gc APPLY=1 after reviewing the audit' >&2
		echo 'agent-worktree: set ALLOW_WORKTREE_BACKLOG=1 only for an intentional exception' >&2
		exit 1
	fi
	slug="$(printf '%s' "$topic" | tr '/' '-' | tr -cs 'A-Za-z0-9._-' '-')"
	target="${WORKTREE_PATH:-$(dirname "$primary")/$(basename "$primary")-$slug}"
	[ ! -e "$target" ] || { echo "agent-worktree: target exists: $target" >&2; exit 1; }
	git -C "$primary" show-ref --verify --quiet "refs/heads/$topic" && { echo "agent-worktree: branch exists: $topic" >&2; exit 1; }

	# Branch off origin/main by default, NOT the primary worktree's HEAD. A new worktree is almost
	# always the start of a fresh PR, and branching off HEAD silently inherits whatever branch the
	# primary happens to be parked on — a stale or unrelated one — so the new work sits on the wrong
	# base and every rebase fights history it never meant to touch. Fetching first also picks up
	# merges that landed while you worked. `BASE=HEAD` (or BASE=<ref>) opts into deliberate stacking
	# on the current branch or an explicit ref. Same BASE convention as `make verify`.
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
	# Register before bootstrap. Once the branch exists, its seams must be visible before this
	# session generates or edits anything another agent could also own.
	if ! LOOMARR_REPO_ROOT="$target" "$SCRIPT_DIR/agent.sh" start "$task" "$claims" "$depends_on"; then
		echo "agent-worktree: registration failed; clean unregistered worktree retained at $target" >&2
		exit 1
	fi
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
	git -C "$ROOT" rev-parse --verify "$base^{commit}" >/dev/null 2>&1 || { echo "verify: unknown BASE=$base" >&2; exit 2; }
	changed="$( { git -C "$ROOT" diff --name-only "$base"...HEAD; git -C "$ROOT" diff --name-only; git -C "$ROOT" ls-files --others --exclude-standard; } | sort -u )"
	[ -n "$changed" ] || { echo 'verify: no changes'; return; }
	echo 'verify: affected local evidence selected by CI impact'
	printf '%s\n' "$changed"
	scope="$(printf '%s\n' "$changed" | "$SCRIPT_DIR/ci-impact.sh")"
	selected="$(printf '%s\n' "$scope" | sed -n 's/=true$//p' | paste -sd, -)"
	local_gates=
	protected_gates=
	while IFS='=' read -r gate enabled; do
		[ "$enabled" = true ] || continue
		case "$gate" in
			go|go_full|rust|web|clients|docs|agent|android|policy)
				if [ -n "$local_gates" ]; then local_gates="$local_gates,$gate"; else local_gates="$gate"; fi
				;;
			contracts|postgres|apple_mobile|apple_tv|expo_android_mobile|expo_android_tv|visual|e2e|tuner|image)
				if [ -n "$protected_gates" ]; then protected_gates="$protected_gates,$gate"; else protected_gates="$gate"; fi
				;;
			*)
				echo "verify: CI impact selected unsupported gate: $gate" >&2
				exit 2
				;;
		esac
	done <<EOF
$scope
EOF
	[ -n "$local_gates" ] || local_gates=none
	[ -n "$protected_gates" ] || protected_gates=none
	echo "verify: CI impact gates: $selected"
	echo "verify: local gates: $local_gates"
	echo "verify: protected gates: $protected_gates"
	if printf '%s\n' "$scope" | grep -qx 'go_full=true' && ! printf '%s\n' "$scope" | grep -qx 'go=true'; then
		echo 'verify: go_full selected without the executable Go gate' >&2
		exit 2
	fi

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
		if printf '%s\n' "$changed" | grep -qE '^observability/|^scripts/verify-observability\.sh$|^mk/check\.mk$'; then
			make -C "$ROOT" observability-verify
		fi
	fi
	if printf '%s\n' "$scope" | grep -qx 'agent=true'; then
		make -C "$ROOT" agent-harness-test
	fi
	if printf '%s\n' "$scope" | grep -qx 'policy=true'; then
		make -C "$ROOT" ci-lint release-verify
	fi
	if printf '%s\n' "$scope" | grep -qx 'go=true'; then
		packages="$(printf '%s\n' "$changed" | "$SCRIPT_DIR/go-impact.sh")"
		[ -n "$packages" ] || { echo 'verify: Go selected but package closure is empty' >&2; exit 1; }
		package_count="$(printf '%s\n' "$packages" | wc -l | tr -d ' ')"
		if printf '%s\n' "$scope" | grep -qx 'go_full=true'; then
			echo "verify: affected Go packages: $package_count (complete)"
		else
			echo "verify: affected Go packages: $package_count"
		fi
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
	if printf '%s\n' "$scope" | grep -qx 'clients=true'; then
		make -C "$ROOT" clients
	fi
	if printf '%s\n' "$scope" | grep -qx 'docs=true'; then
		make -C "$ROOT" docs-lint
	fi
	if printf '%s\n' "$scope" | grep -qx 'android=true'; then
		make -C "$ROOT" android
	fi
	echo "verify: completed local gates: $local_gates"
}

verify_all() {
	exec make -C "$ROOT" check-static test
}

case "${1:-}" in
	start) shift; start_session "${1:-}" "${2:-}" "${3:-}" ;;
	status) status_sessions ;;
	renew) renew_session ;;
	prune) prune_sessions ;;
	stop) stop_session ;;
	env) "$SCRIPT_DIR/dev-env.sh" show ;;
	baseline) baseline ;;
	doctor) doctor ;;
	bootstrap) bootstrap ;;
	worktree) shift; worktree "${1:-}" "${2:-}" "${3:-}" "${4:-}" ;;
	verify) verify_changed ;;
	verify-all) verify_all ;;
	gc) exec "$SCRIPT_DIR/agent-worktree-gc.sh" ;;
	*)
		echo 'usage: scripts/agent.sh {start TASK [CLAIMS] [DEPENDS_ON]|status|renew|prune|stop|env|baseline|doctor|bootstrap|worktree TOPIC [TASK] [CLAIMS] [DEPENDS_ON]|verify|verify-all|gc}' >&2
		exit 2
		;;
esac
