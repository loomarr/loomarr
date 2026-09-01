#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
launcher="$repo_root/scripts/observability-dev.sh"

if [[ ! -x "$launcher" ]]; then
  echo 'observability-dev-runtime-test: scripts/observability-dev.sh is missing or not executable' >&2
  exit 1
fi

for command in curl docker go jq; do
  command -v "$command" >/dev/null || {
    echo "observability-dev-runtime-test: $command is required" >&2
    exit 1
  }
done

tmp=$(mktemp -d "${TMPDIR:-/tmp}/loomarr-observability.XXXXXX")
slot=$(( $$ % 500 ))
backend_port="${OBSERVABILITY_TEST_BACKEND_PORT:-$((30000 + slot))}"
prometheus_port="${OBSERVABILITY_TEST_PROMETHEUS_PORT:-$((31000 + slot))}"
grafana_port="${OBSERVABILITY_TEST_GRAFANA_PORT:-$((32000 + slot))}"
project="loomarr-observability-test-$$"
database_url="sqlite://$tmp/loomarr.db"
server_pid=

stack() {
  LOOMARR_DEV_PORT="$backend_port" \
    PROMETHEUS_DEV_PORT="$prometheus_port" \
    GRAFANA_DEV_PORT="$grafana_port" \
    OBSERVABILITY_COMPOSE_PROJECT_NAME="$project" \
    LOOMARR_AGENT_DATABASE_URL="$database_url" \
    "$launcher" "$@"
}

cleanup() {
  LOOMARR_DEV_PORT="$backend_port" \
    PROMETHEUS_DEV_PORT="$prometheus_port" \
    GRAFANA_DEV_PORT="$grafana_port" \
    docker compose \
      -p "$project" \
      -f "$repo_root/docker/compose.observability.yaml" \
      down --volumes >/dev/null 2>&1 || true
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT INT TERM

wait_for_http() {
  local url=$1
  local attempts=${2:-120}
  local response
  for ((i = 1; i <= attempts; i++)); do
    if response=$(curl --fail --silent --show-error --max-time 2 "$url" 2>/dev/null); then
      printf '%s' "$response"
      return 0
    fi
    sleep 0.5
  done
  echo "observability-dev-runtime-test: timed out waiting for $url" >&2
  return 1
}

wait_for_json() {
  local url=$1
  local filter=$2
  local credentials=${3:-}
  local response
  for ((i = 1; i <= 120; i++)); do
    if [[ -n "$credentials" ]]; then
      response=$(curl --fail --silent --max-time 2 --user "$credentials" "$url" 2>/dev/null) || true
    else
      response=$(curl --fail --silent --max-time 2 "$url" 2>/dev/null) || true
    fi
    if [[ -n "$response" ]] && jq -e "$filter" <<<"$response" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "observability-dev-runtime-test: condition did not become true at $url" >&2
  [[ -z "$response" ]] || jq . <<<"$response" >&2 || printf '%s\n' "$response" >&2
  return 1
}

echo 'observability-dev-runtime-test: building the real backend'
make -C "$repo_root" rust-dev-build >/dev/null
go build -o "$tmp/loomarr" "$repo_root/cmd/loomarr"

echo 'observability-dev-runtime-test: creating the seed fixture through cmd/seed'
DATABASE_URL="$database_url" go run "$repo_root/cmd/seed"

DATABASE_URL="$database_url" \
  AUTO_MIGRATE=true \
  LISTEN_ADDR=":$backend_port" \
  SERVER_PUBLIC_URL="http://127.0.0.1:$backend_port" \
  LOOMARR_IMAGE_WORKER="$repo_root/target/debug/loomarr-image" \
  LOOMARR_DEV_LOGIN=1 \
  FILLER_DROP_DIR="$tmp/filler" \
  PLAYOUT_PREPARED_DIR="$tmp/prepared" \
  DIAGNOSTICS_DIR="$tmp/diagnostics" \
  "$tmp/loomarr" >"$tmp/loomarr.log" 2>&1 &
server_pid=$!

wait_for_http "http://127.0.0.1:$backend_port/v1/metrics" >/dev/null

echo 'observability-dev-runtime-test: starting Prometheus and Grafana'
stack up >/dev/null

wait_for_json \
  "http://127.0.0.1:$prometheus_port/api/v1/targets" \
  '.data.activeTargets | any(.labels.job == "loomarr" and .health == "up")'
wait_for_json \
  "http://127.0.0.1:$prometheus_port/api/v1/query?query=sum%28loomarr_titles%29" \
  '.status == "success" and (.data.result | length == 1) and (.data.result[0].value[1] | tonumber > 0)'
wait_for_json \
  "http://127.0.0.1:$grafana_port/api/dashboards/uid/loomarr-overview" \
  '.dashboard.uid == "loomarr-overview"' \
  'admin:loomarr'

echo 'observability-dev-runtime-test: seeded metric and provisioned dashboard verified over HTTP'
