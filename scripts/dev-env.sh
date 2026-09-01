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

registered_slot() {
	# Registration is authoritative once allocation moves a worktree off its legacy checksum slot.
	# Later dev commands must resolve that same tuple without extra shell state.
	common="$(git -C "$ROOT" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)" || return 1
	key="$(printf '%s' "$ROOT" | cksum | awk '{print $1}')"
	file="$common/loomarr-agents/sessions/$key"
	[ -f "$file" ] || return 1
	sed -n 's/^slot=//p' "$file" | head -1
}

valid_port() {
	case "$1" in
		''|*[!0-9]*) return 1 ;;
	esac
	[ "$1" -ge 1 ] && [ "$1" -le 65535 ]
}

PRIMARY="$(primary_worktree)"
BRANCH="$(git -C "$ROOT" symbolic-ref --quiet --short HEAD 2>/dev/null || printf 'detached')"
BRANCH_SLUG="$(slugify "$BRANCH")"
[ -n "$BRANCH_SLUG" ] || BRANCH_SLUG=worktree
CHECKSUM="$(printf '%s' "$ROOT" | cksum | awk '{print $1}')"
DEFAULT_SLOT=$((CHECKSUM % 900 + 1))
REGISTERED_SLOT="$(registered_slot || true)"
SLOT="${REGISTERED_SLOT:-${LOOMARR_AGENT_SLOT_OVERRIDE:-$DEFAULT_SLOT}}"
case "$SLOT" in
	''|*[!0-9]*) echo "dev-env: invalid agent port slot: $SLOT" >&2; exit 2 ;;
esac
if [ "$SLOT" -lt 1 ] || [ "$SLOT" -gt 900 ]; then
	echo "dev-env: invalid agent port slot: $SLOT" >&2
	exit 2
fi

if [ "$ROOT" = "$PRIMARY" ]; then
	INSTANCE=primary
	DEFAULT_BACKEND=8080
	DEFAULT_FRONTEND=5173
	DEFAULT_STORYBOOK=6006
	DEFAULT_TUNARR=8000
	DEFAULT_PROMETHEUS=9090
	DEFAULT_GRAFANA=3000
	DEFAULT_DATABASE=
	DEFAULT_FILLER=
	DEFAULT_PREPARED=
	DEFAULT_DIAGNOSTICS=
	DEFAULT_DEV_LOGIN=
else
	INSTANCE="${BRANCH_SLUG}-$(printf '%03d' "$SLOT")"
	DEFAULT_BACKEND=$((18000 + SLOT))
	DEFAULT_FRONTEND=$((15000 + SLOT))
	DEFAULT_STORYBOOK=$((16000 + SLOT))
	DEFAULT_TUNARR=$((19000 + SLOT))
	DEFAULT_PROMETHEUS=$((20000 + SLOT))
	DEFAULT_GRAFANA=$((21000 + SLOT))
	DEFAULT_DATABASE="sqlite://$ROOT/.agent-data/loomarr.db"
	DEFAULT_FILLER="$ROOT/.filler-drop"
	DEFAULT_PREPARED="$ROOT/.agent-data/prepared"
	DEFAULT_DIAGNOSTICS="$ROOT/.agent-data/diagnostics"
	DEFAULT_DEV_LOGIN=1
fi

BACKEND_PORT="${LOOMARR_DEV_PORT:-$DEFAULT_BACKEND}"
FRONTEND_PORT="${LOOMARR_FE_PORT:-$DEFAULT_FRONTEND}"
STORYBOOK_PORT="${LOOMARR_STORYBOOK_PORT:-$DEFAULT_STORYBOOK}"
TUNARR_PORT="${TUNARR_DEV_PORT:-$DEFAULT_TUNARR}"
PROMETHEUS_PORT="${PROMETHEUS_DEV_PORT:-$DEFAULT_PROMETHEUS}"
GRAFANA_PORT="${GRAFANA_DEV_PORT:-$DEFAULT_GRAFANA}"

if [ "$ROOT" = "$PRIMARY" ]; then
	DEFAULT_PUBLIC_URL=
else
	# Internal playout's parent ffmpeg re-opens Loomarr's playlist through SERVER_PUBLIC_URL.
	# A copied primary .env therefore must not send a secondary worktree back to :8080.
	DEFAULT_PUBLIC_URL="http://localhost:$BACKEND_PORT"
fi

for port in "$BACKEND_PORT" "$FRONTEND_PORT" "$STORYBOOK_PORT" "$TUNARR_PORT" "$PROMETHEUS_PORT" "$GRAFANA_PORT"; do
	valid_port "$port" || { echo "dev-env: invalid port: $port" >&2; exit 2; }
done

COMPOSE_SLUG="$(slugify "loomarr-$INSTANCE")"
OBSERVABILITY_COMPOSE_SLUG="${OBSERVABILITY_COMPOSE_PROJECT_NAME:-$COMPOSE_SLUG-observability}"
ARTIFACT_DIR="$ROOT/.artifacts/$INSTANCE"
DATABASE_OVERRIDE="${LOOMARR_AGENT_DATABASE_URL:-$DEFAULT_DATABASE}"
FILLER_OVERRIDE="${LOOMARR_AGENT_FILLER_DIR:-$DEFAULT_FILLER}"
PREPARED_OVERRIDE="${LOOMARR_AGENT_PREPARED_DIR:-$DEFAULT_PREPARED}"
DIAGNOSTICS_OVERRIDE="${LOOMARR_AGENT_DIAGNOSTICS_DIR:-$DEFAULT_DIAGNOSTICS}"
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
	export)
		emit_export LOOMARR_INSTANCE "$INSTANCE"
		emit_export LOOMARR_AGENT_SLOT "$SLOT"
		emit_export LOOMARR_REPO_ROOT "$ROOT"
		emit_export LOOMARR_DEV_PORT "$BACKEND_PORT"
		emit_export LOOMARR_FE_PORT "$FRONTEND_PORT"
		emit_export LOOMARR_STORYBOOK_PORT "$STORYBOOK_PORT"
		emit_export TUNARR_DEV_PORT "$TUNARR_PORT"
		emit_export PROMETHEUS_DEV_PORT "$PROMETHEUS_PORT"
		emit_export GRAFANA_DEV_PORT "$GRAFANA_PORT"
		emit_export LISTEN_ADDR ":$BACKEND_PORT"
		emit_export LOOMARR_API "http://localhost:$BACKEND_PORT"
		emit_export COMPOSE_PROJECT_NAME "$COMPOSE_SLUG"
		emit_export OBSERVABILITY_COMPOSE_PROJECT_NAME "$OBSERVABILITY_COMPOSE_SLUG"
		emit_export LOOMARR_ARTIFACT_DIR "$ARTIFACT_DIR"
		emit_export LOOMARR_AGENT_DATABASE_URL "$DATABASE_OVERRIDE"
		emit_export LOOMARR_AGENT_FILLER_DIR "$FILLER_OVERRIDE"
		emit_export LOOMARR_AGENT_PREPARED_DIR "$PREPARED_OVERRIDE"
		emit_export LOOMARR_AGENT_DIAGNOSTICS_DIR "$DIAGNOSTICS_OVERRIDE"
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
			'prometheus' "http://localhost:$PROMETHEUS_PORT" \
			'grafana' "http://localhost:$GRAFANA_PORT" \
			'compose project' "$COMPOSE_SLUG" \
			'observability project' "$OBSERVABILITY_COMPOSE_SLUG" \
			'artifacts' "$ARTIFACT_DIR" \
			'database override' "${DATABASE_OVERRIDE:-<from .env>}" \
			'filler override' "${FILLER_OVERRIDE:-<from .env>}" \
			'prepared override' "${PREPARED_OVERRIDE:-<from .env>}" \
			'diagnostics override' "${DIAGNOSTICS_OVERRIDE:-<from .env>}" \
			'public URL override' "${PUBLIC_URL_OVERRIDE:-<from .env>}"
		printf '%-22s %s\n' 'dev login' "$dev_login_label"
		;;
	*)
		echo "usage: scripts/dev-env.sh [show|export]" >&2
		exit 2
		;;
esac
