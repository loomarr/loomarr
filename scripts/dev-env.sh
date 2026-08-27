#!/usr/bin/env sh
# Resolve one deterministic, isolated local-dev environment per git worktree.

set -eu

ROOT="${LOOMARR_REPO_ROOT:-$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd -P)}"
ROOT="$(CDPATH='' cd -- "$ROOT" && pwd -P)"

primary_worktree() {
	primary="$(git -C "$ROOT" worktree list --porcelain | sed -n 's/^worktree //p' | head -1)"
	[ -n "$primary" ] || return 1
	(CDPATH='' cd -- "$primary" && pwd -P)
}

slugify() {
	printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9_-' '-' | sed 's/^-//;s/-$//' | cut -c1-36
}

shell_quote() {
	printf "'"
	printf '%s' "$1" | sed "s/'/'\\\\''/g"
	printf "'"
}

valid_port() {
	case "$1" in
		''|*[!0-9]*) return 1 ;;
	esac
	[ "$1" -ge 1 ] && [ "$1" -le 65535 ]
}

valid_slot() {
	case "$1" in
		''|*[!0-9]*) return 1 ;;
	esac
	[ "$1" -ge 1 ] && [ "$1" -le 900 ]
}

preferred_slot() (
	checksum="$(printf '%s' "$1" | cksum | awk '{print $1}')"
	printf '%s\n' $((checksum % 900 + 1))
)

slot_for_worktree() (
	worktree="$1"
	slot_file="$worktree/.agent-data/dev-env-slot"
	if [ -f "$slot_file" ]; then
		slot="$(sed -n '1p' "$slot_file")"
		if valid_slot "$slot"; then
			printf '%s\n' "$slot"
			return
		fi
	fi
	preferred_slot "$worktree"
)

PRIMARY="$(primary_worktree)"
BRANCH="$(git -C "$ROOT" symbolic-ref --quiet --short HEAD 2>/dev/null || printf 'detached')"
BRANCH_SLUG="$(slugify "$BRANCH")"
[ -n "$BRANCH_SLUG" ] || BRANCH_SLUG=worktree
SLOT="$(slot_for_worktree "$ROOT")"

allocate_slot() {
	[ "$ROOT" != "$PRIMARY" ] || return 0
	slot_file="$ROOT/.agent-data/dev-env-slot"
	if [ -f "$slot_file" ] && valid_slot "$(sed -n '1p' "$slot_file")"; then
		return
	fi
	used_slots="$({
		git -C "$ROOT" worktree list --porcelain | sed -n 's/^worktree //p' | while IFS= read -r worktree; do
			[ "$worktree" = "$ROOT" ] || [ "$worktree" = "$PRIMARY" ] || slot_for_worktree "$worktree"
		done
	} | tr '\n' ' ')"
	candidate="$(preferred_slot "$ROOT")"
	attempt=0
	while [ "$attempt" -lt 900 ]; do
		case " $used_slots " in
			*" $candidate "*) candidate=$((candidate % 900 + 1)) ;;
			*) break ;;
		esac
		attempt=$((attempt + 1))
	done
	[ "$attempt" -lt 900 ] || { echo 'dev-env: no isolated runtime port slots remain' >&2; exit 1; }
	mkdir -p "$ROOT/.agent-data"
	tmp="$slot_file.tmp.$$"
	printf '%s\n' "$candidate" > "$tmp"
	mv "$tmp" "$slot_file"
}

if [ "$ROOT" = "$PRIMARY" ]; then
	INSTANCE=primary
	DEFAULT_BACKEND=8080
	DEFAULT_FRONTEND=5173
	DEFAULT_STORYBOOK=6006
	DEFAULT_TUNARR=8000
	DEFAULT_DATABASE=
	DEFAULT_FILLER=
	DEFAULT_PREPARED=
	DEFAULT_DEV_LOGIN=
else
	INSTANCE="${BRANCH_SLUG}-$(printf '%03d' "$SLOT")"
	DEFAULT_BACKEND=$((18000 + SLOT))
	DEFAULT_FRONTEND=$((15000 + SLOT))
	DEFAULT_STORYBOOK=$((16000 + SLOT))
	DEFAULT_TUNARR=$((19000 + SLOT))
	DEFAULT_DATABASE="sqlite://$ROOT/.agent-data/loomarr.db"
	DEFAULT_FILLER="$ROOT/.filler-drop"
	DEFAULT_PREPARED="$ROOT/.agent-data/prepared"
	DEFAULT_DEV_LOGIN=1
fi

BACKEND_PORT="${LOOMARR_DEV_PORT:-$DEFAULT_BACKEND}"
FRONTEND_PORT="${LOOMARR_FE_PORT:-$DEFAULT_FRONTEND}"
STORYBOOK_PORT="${LOOMARR_STORYBOOK_PORT:-$DEFAULT_STORYBOOK}"
TUNARR_PORT="${TUNARR_DEV_PORT:-$DEFAULT_TUNARR}"

if [ "$ROOT" = "$PRIMARY" ]; then
	DEFAULT_PUBLIC_URL=
else
	# Internal playout's parent ffmpeg re-opens Loomarr's playlist through SERVER_PUBLIC_URL.
	# A copied primary .env therefore must not send a secondary worktree back to :8080.
	DEFAULT_PUBLIC_URL="http://localhost:$BACKEND_PORT"
fi

for port in "$BACKEND_PORT" "$FRONTEND_PORT" "$STORYBOOK_PORT" "$TUNARR_PORT"; do
	valid_port "$port" || { echo "dev-env: invalid port: $port" >&2; exit 2; }
done

COMPOSE_SLUG="$(slugify "loomarr-$INSTANCE")"
ARTIFACT_DIR="$ROOT/.artifacts/$INSTANCE"
DATABASE_OVERRIDE="${LOOMARR_AGENT_DATABASE_URL:-$DEFAULT_DATABASE}"
FILLER_OVERRIDE="${LOOMARR_AGENT_FILLER_DIR:-$DEFAULT_FILLER}"
PREPARED_OVERRIDE="${LOOMARR_AGENT_PREPARED_DIR:-$DEFAULT_PREPARED}"
PUBLIC_URL_OVERRIDE="${LOOMARR_AGENT_PUBLIC_URL:-$DEFAULT_PUBLIC_URL}"
DEV_LOGIN_OVERRIDE="${LOOMARR_AGENT_DEV_LOGIN:-$DEFAULT_DEV_LOGIN}"

emit_export() {
	name="$1"
	value="$2"
	printf 'export %s=' "$name"
	shell_quote "$value"
	printf '\n'
}

case "${1:-show}" in
	allocate)
		allocate_slot
		;;
	export)
		emit_export LOOMARR_INSTANCE "$INSTANCE"
		emit_export LOOMARR_REPO_ROOT "$ROOT"
		emit_export LOOMARR_DEV_PORT "$BACKEND_PORT"
		emit_export LOOMARR_FE_PORT "$FRONTEND_PORT"
		emit_export LOOMARR_STORYBOOK_PORT "$STORYBOOK_PORT"
		emit_export TUNARR_DEV_PORT "$TUNARR_PORT"
		emit_export LISTEN_ADDR ":$BACKEND_PORT"
		emit_export LOOMARR_API "http://localhost:$BACKEND_PORT"
		emit_export COMPOSE_PROJECT_NAME "$COMPOSE_SLUG"
		emit_export LOOMARR_ARTIFACT_DIR "$ARTIFACT_DIR"
		emit_export LOOMARR_AGENT_DATABASE_URL "$DATABASE_OVERRIDE"
		emit_export LOOMARR_AGENT_FILLER_DIR "$FILLER_OVERRIDE"
		emit_export LOOMARR_AGENT_PREPARED_DIR "$PREPARED_OVERRIDE"
		emit_export LOOMARR_AGENT_PUBLIC_URL "$PUBLIC_URL_OVERRIDE"
		emit_export LOOMARR_AGENT_DEV_LOGIN "$DEV_LOGIN_OVERRIDE"
		emit_export FILLER_DROP_DIR "${FILLER_DROP_DIR:-$ROOT/.filler-drop}"
		;;
	show)
		if [ -n "$DEV_LOGIN_OVERRIDE" ]; then dev_login_label=automatic; else dev_login_label='<from .env>'; fi
		printf '%-22s %s\n' \
			'instance' "$INSTANCE" \
			'worktree' "$ROOT" \
			'backend' "http://localhost:$BACKEND_PORT" \
			'frontend' "http://localhost:$FRONTEND_PORT" \
			'storybook' "http://localhost:$STORYBOOK_PORT" \
			'tunarr' "http://localhost:$TUNARR_PORT" \
			'compose project' "$COMPOSE_SLUG" \
			'artifacts' "$ARTIFACT_DIR" \
			'database override' "${DATABASE_OVERRIDE:-<from .env>}" \
			'filler override' "${FILLER_OVERRIDE:-<from .env>}" \
			'prepared override' "${PREPARED_OVERRIDE:-<from .env>}" \
			'public URL override' "${PUBLIC_URL_OVERRIDE:-<from .env>}"
		printf '%-22s %s\n' 'dev login' "$dev_login_label"
		;;
	*)
		echo "usage: scripts/dev-env.sh [show|export|allocate]" >&2
		exit 2
		;;
esac
