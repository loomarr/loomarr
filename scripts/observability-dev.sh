#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
compose_file="$repo_root/docker/compose.observability.yaml"
action=${1:-up}

case "$action" in
  up|down) ;;
  *)
    echo 'usage: scripts/observability-dev.sh [up|down]' >&2
    exit 2
    ;;
esac

# Match make dev-be's configuration precedence: a worktree override wins over
# this checkout's .env, followed by a local SQLite default.
if [[ -f "$repo_root/.env" ]]; then
  set -a
  # shellcheck disable=SC1091 # The per-checkout environment is intentionally dynamic.
  source "$repo_root/.env"
  set +a
fi
eval "$(LOOMARR_REPO_ROOT="$repo_root" "$repo_root/scripts/dev-env.sh" export)"
database_url="${LOOMARR_AGENT_DATABASE_URL:-${DATABASE_URL:-sqlite://$repo_root/loomarr-dev.db}}"

compose() {
  LOOMARR_DEV_PORT="$LOOMARR_DEV_PORT" \
    PROMETHEUS_DEV_PORT="$PROMETHEUS_DEV_PORT" \
    GRAFANA_DEV_PORT="$GRAFANA_DEV_PORT" \
    docker compose \
      -p "$OBSERVABILITY_COMPOSE_PROJECT_NAME" \
      -f "$compose_file" \
      "$@"
}

if [[ "$action" == down ]]; then
  compose down
  exit 0
fi

if [[ "$database_url" == sqlite://* ]]; then
  database_path=${database_url#sqlite://}
  if [[ "$database_path" != /* ]]; then
    database_path="$repo_root/$database_path"
  fi
  if [[ ! -f "$database_path" ]]; then
    mkdir -p "$(dirname "$database_path")"
    echo "observability-dev: seeding $database_path through cmd/seed"
    (cd "$repo_root" && DATABASE_URL="$database_url" go run ./cmd/seed)
  fi
fi

echo "observability-dev: $OBSERVABILITY_COMPOSE_PROJECT_NAME"
compose up -d --wait

cat <<EOF
observability-dev: ready
  backend     http://localhost:$LOOMARR_DEV_PORT (run make dev-be separately)
  prometheus  http://localhost:$PROMETHEUS_DEV_PORT
  grafana     http://localhost:$GRAFANA_DEV_PORT (admin / loomarr)
  database    $database_url
EOF
