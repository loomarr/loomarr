#!/usr/bin/env bash
# Regression tests for the dev-server start path: free-orphan-port.sh, fe-sync.sh and the
# dev-be watchdog's failing-build case. Fake processes and command shims stand in for Air, Vite and
# pnpm, so it needs no network, build or install.

set -euo pipefail

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
REPO="$(CDPATH='' cd -- "$SCRIPT_DIR/../.." && pwd -P)"
# Canonical, because macOS reports process cwds under /private/var while mktemp says /var.
tmp="$(CDPATH='' cd -- "$(mktemp -d)" && pwd -P)"
# Every background process's pid, in a file: helpers run inside $(...), whose variables are lost.
track() { echo "$1" >>"$tmp/pids"; }
cleanup() {
	# shellcheck disable=SC2046 # One word per pid.
	[ ! -f "$tmp/pids" ] || kill $(cat "$tmp/pids") 2>/dev/null || true
	rm -rf "$tmp"
}
trap cleanup EXIT INT TERM

failures=0
fail() {
	printf 'FAIL: %s\n' "$*" >&2
	failures=$((failures + 1))
}
expect() { # OUTPUT PATTERN
	printf '%s\n' "$1" | grep -Eq -- "$2" || fail "expected /$2/ in: ${1:-<nothing>}"
}

# wait_until SECONDS COMMAND...: poll COMMAND every 0.2s until it succeeds.
wait_until() {
	local tries=$(($1 * 5))
	shift
	while [ "$tries" -gt 0 ]; do
		"$@" && return 0
		sleep 0.2
		tries=$((tries - 1))
	done
	return 1
}

# ---------------------------------------------------------------- free-orphan-port

if command -v python3 >/dev/null 2>&1 && { command -v lsof >/dev/null 2>&1 || command -v ss >/dev/null 2>&1; }; then
	free_port() { python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'; }
	listens() { python3 -c 'import socket,sys; s=socket.socket(); sys.exit(s.connect_ex(("127.0.0.1", int(sys.argv[1]))))' "$1"; }
	# listen DIR PORT: a listener whose cwd is DIR.
	listen() {
		(cd "$1" && { python3 -m http.server "$2" --bind 127.0.0.1 >/dev/null 2>&1 & echo $! >"$tmp/pid"; })
		track "$(cat "$tmp/pid")"
		wait_until 10 listens "$2" || fail "no listener came up on :$2"
		cat "$tmp/pid"
	}
	export WATCH_WORKTREE_ROOTS="$tmp/worktrees" WATCH_REAP_GRACE=2

	mkdir -p "$tmp/worktrees/gone/web" "$tmp/worktrees/live"
	port="$(free_port)"
	orphan="$(listen "$tmp/worktrees/gone/web" "$port")"
	rm -rf "$tmp/worktrees/gone"
	out="$("$SCRIPT_DIR/free-orphan-port.sh" "$port" 2>&1)" || fail 'free-orphan-port failed on an orphan'
	expect "$out" "stopped .*\\(pid $orphan\\) left listening on :$port by a removed worktree"
	wait_until 5 sh -c "! kill -0 $orphan 2>/dev/null" || fail 'the orphaned listener survived'

	port="$(free_port)"
	live="$(listen "$tmp/worktrees/live" "$port")"
	out="$("$SCRIPT_DIR/free-orphan-port.sh" "$port" 2>&1)" || fail 'free-orphan-port failed on a live listener'
	[ -z "$out" ] || fail "free-orphan-port reported a live listener: $out"
	kill -0 "$live" 2>/dev/null || fail 'free-orphan-port stopped a listener in a live worktree'

	"$SCRIPT_DIR/free-orphan-port.sh" "$(free_port)" || fail 'free-orphan-port failed on a free port'
	unset WATCH_WORKTREE_ROOTS
else
	echo 'dev-runtime-test: python3 or lsof/ss missing; skipping free-orphan-port'
fi

# ---------------------------------------------------------------- fe-sync

fe="$tmp/fe"
mkdir -p "$fe/scripts/dev" "$fe/web/node_modules" "$fe/web/packages/design-system" "$fe/api" "$tmp/shims"
cp "$SCRIPT_DIR/fe-sync.sh" "$fe/scripts/dev/"
printf 'lock: 1\n' >"$fe/web/pnpm-lock.yaml"
printf 'openapi: 3.1.0\n' >"$fe/api/openapi.yaml"
printf 'export {};\n' >"$fe/web/packages/design-system/web-theme.ts"
git -C "$fe" init -q && git -C "$fe" add web/packages/design-system
printf '#!/bin/sh\necho "$*" >>"%s"\n' "$tmp/pnpm-calls" >"$tmp/shims/pnpm"
chmod +x "$tmp/shims/pnpm"
sync_fe() { PATH="$tmp/shims:$PATH" "$fe/scripts/dev/fe-sync.sh" 2>&1; }
calls() { grep -c . "$tmp/pnpm-calls" 2>/dev/null || echo 0; }

out="$(sync_fe)"
expect "$out" 'installing and regenerating'
[ "$(calls)" = 2 ] || fail "a first sync should install and regenerate, got $(calls) pnpm calls"
expect "$(cat "$tmp/pnpm-calls")" '^install --frozen-lockfile$'
expect "$(cat "$tmp/pnpm-calls")" '^codegen$'
out="$(sync_fe)"
[ -z "$out" ] && [ "$(calls)" = 2 ] || fail "an unchanged checkout should not sync again: $out"
printf 'openapi: 3.1.1\n' >"$fe/api/openapi.yaml"
sync_fe >/dev/null
[ "$(calls)" = 4 ] || fail "a changed API spec should sync again, got $(calls) pnpm calls"
printf 'export const a = 1;\n' >"$fe/web/packages/design-system/web-theme.ts"
git -C "$fe" add web/packages/design-system
sync_fe >/dev/null
[ "$(calls)" = 6 ] || fail "changed design tokens should sync again, got $(calls) pnpm calls"
rm -rf "$fe/web/node_modules" && mkdir "$fe/web/node_modules"
sync_fe >/dev/null
[ "$(calls)" = 8 ] || fail "a fresh install should sync again, got $(calls) pnpm calls"

# ---------------------------------------------------------------- orca-doctor

if command -v jq >/dev/null 2>&1; then
	primary="$(git -C "$REPO" worktree list --porcelain | sed -n 's/^worktree //p' | head -1)"
	# fake_orca BASE_REF STARTUP SETUP_SCRIPT: an orca CLI that knows this repository with those settings.
	fake_orca() {
		cat >"$tmp/shims/orca" <<EOF
#!/bin/sh
case "\$1 \$2" in
	'repo list') printf '{"result":{"repos":[{"id":"r1","path":"%s"}]}}\n' '$primary' ;;
	'repo show') printf '{"result":{"repo":{"worktreeBaseRef":%s,"hookSettings":{"setupRunPolicy":"run-by-default","setupAgentStartupPolicy":"%s","scripts":{"setup":"%s","archive":""}}}}}\n' '$1' '$2' '$3' ;;
esac
EOF
		chmod +x "$tmp/shims/orca"
	}
	fake_orca null start-immediately 'make setup'
	out="$(ORCA_CLI_COMMAND="$tmp/shims/orca" "$SCRIPT_DIR/orca-doctor.sh")" || fail 'orca-doctor failed'
	expect "$out" 'set-base-ref --repo r1 --ref origin/main'
	expect "$out" 'setupAgentStartupPolicy=start-immediately'
	expect "$out" 'a local setup or archive script replaces orca.yaml'
	fake_orca '"origin/main"' wait-for-setup ''
	out="$(ORCA_CLI_COMMAND="$tmp/shims/orca" "$SCRIPT_DIR/orca-doctor.sh")" || fail 'orca-doctor failed'
	expect "$out" 'settings match'
	printf '%s\n' "$out" | grep -q WARNING && fail "matching settings were warned about: $out"
	out="$(ORCA_CLI_COMMAND="$tmp/missing-orca" "$SCRIPT_DIR/orca-doctor.sh")" || fail 'orca-doctor failed without orca'
	[ -z "$out" ] || fail "orca-doctor printed without orca: $out"
else
	echo 'dev-runtime-test: jq missing; skipping orca-doctor'
fi

# ---------------------------------------------------------------- dev-be watchdog

# A fake worktree whose Air and dev binary are sleeps under their real names, so the watchdog's
# by-name, by-cwd ownership checks find them.
wd="$tmp/wd"
mkdir -p "$wd/tmp" "$wd/internal" "$wd/scripts" "$tmp/names"
cp "$REPO/scripts/dev-processes.sh" "$wd/scripts/"
# Symlinks, not copies: macOS kills a copied system binary, and a process takes its name from the link.
ln -s "$(command -v sleep)" "$tmp/names/air"
ln -s "$(command -v sleep)" "$tmp/names/loomarr-dev"
start_fake() { # NAME
	(cd "$wd" && { "$tmp/names/$1" 300 >/dev/null 2>&1 & echo $! >"$tmp/pid"; })
	track "$(cat "$tmp/pid")"
	cat "$tmp/pid"
}
watchdog() { # LOG: run the watchdog against the fake worktree in the background
	LOOMARR_REPO_ROOT="$wd" DEV_BE_WATCHDOG_POLL=1 DEV_BE_WATCHDOG_STRIKES=1 \
		sh "$REPO/scripts/dev-be-watchdog.sh" >"$1" 2>&1 </dev/null &
	track $!
	echo $!
}
touch -t 202001010000 "$wd/tmp/loomarr-dev"
touch "$wd/internal/main.go" # a source newer than the binary: the binary is stale
air="$(start_fake air)"

# The last build failed: its log is newer than the binary. Report it and leave the binary running.
binary="$(start_fake loomarr-dev)"
touch "$wd/tmp/build-errors.log"
dog="$(watchdog "$tmp/failing.log")"
wait_until 10 grep -q 'the build is failing' "$tmp/failing.log" || fail "a failing build was not reported: $(cat "$tmp/failing.log")"
sleep 3
kill -0 "$binary" 2>/dev/null || fail 'the watchdog restarted the binary while the build was failing'
[ "$(grep -c 'the build is failing' "$tmp/failing.log")" = 1 ] || fail "one failure should be reported once: $(cat "$tmp/failing.log")"
kill "$dog" 2>/dev/null || true

# No failed build: a stale binary means Air stopped rebuilding. Nudge, then restart the binary.
rm -f "$wd/tmp/build-errors.log"
dog="$(watchdog "$tmp/wedged.log")"
wait_until 15 sh -c "! kill -0 $binary 2>/dev/null" || fail "a wedged Air's binary was not restarted: $(cat "$tmp/wedged.log")"
expect "$(cat "$tmp/wedged.log")" 'nudging Air'
kill "$dog" "$air" 2>/dev/null || true

if [ "$failures" -gt 0 ]; then
	printf 'dev-runtime-test: %s failure(s)\n' "$failures" >&2
	exit 1
fi
echo 'dev-runtime-test: ok'
