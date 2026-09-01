#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
prometheus_dir="$repo_root/observability/prometheus"
grafana_dir="$repo_root/observability/grafana"
manifest="$repo_root/observability/metrics-manifest.txt"
dashboard="$grafana_dir/dashboards/loomarr-overview.json"
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

jq -e '
  .uid == "loomarr-overview" and
  .editable == false and
  ([.templating.list[] | select(.name == "DS_PROMETHEUS" and .type == "datasource")] | length == 1) and
  ([.panels[].targets[].expr] | length > 0)
' "$dashboard" >/dev/null

{
  rg -o 'loomarr_[A-Za-z0-9_]+' "$dashboard" "$prometheus_dir" || true
} | sed -E 's/.*://' | sed -E 's/_(bucket|sum|count)$//' | sort -u >"$work_dir/referenced"
cut -d'|' -f1 "$manifest" | sed '/^#/d;/^$/d' | sort -u >"$work_dir/declared"
if ! comm -23 "$work_dir/referenced" "$work_dir/declared" >"$work_dir/unknown"; then
  exit 1
fi
if [[ -s "$work_dir/unknown" ]]; then
  echo "observability queries reference families absent from the manifest:" >&2
  cat "$work_dir/unknown" >&2
  exit 1
fi

docker run --rm --entrypoint /bin/promtool \
  -v "$prometheus_dir:/work:ro" -w /work \
  prom/prometheus:v3.14.0 check rules alerts.yml recording-rules.yml
docker run --rm --entrypoint /bin/promtool \
  -v "$prometheus_dir:/work:ro" -w /work \
  prom/prometheus:v3.14.0 test rules rules.test.yml

docker run --rm \
  -e PROMETHEUS_URL=http://prometheus:9090 \
  -e GF_SECURITY_ADMIN_PASSWORD=verify \
  -e GF_PLUGINS_PREINSTALL_DISABLED=true \
  -v "$grafana_dir/provisioning:/etc/grafana/provisioning:ro" \
  -v "$grafana_dir/dashboards:/var/lib/grafana/dashboards:ro" \
  --entrypoint sh grafana/grafana:13.2.0 -ec '
    grafana server --homepath=/usr/share/grafana >/tmp/grafana.log 2>&1 &
    server_pid=$!
    i=0
    while [ "$i" -lt 30 ]; do
      if wget -qO- http://127.0.0.1:3000/api/health >/dev/null 2>&1 &&
         wget -qO /tmp/dashboard.json --header="Authorization: Basic YWRtaW46dmVyaWZ5" http://127.0.0.1:3000/api/dashboards/uid/loomarr-overview &&
         grep -q "\"uid\":\"loomarr-overview\"" /tmp/dashboard.json; then
        kill "$server_pid"
        wait "$server_pid" || true
        exit 0
      fi
      if ! kill -0 "$server_pid" 2>/dev/null; then
        cat /tmp/grafana.log >&2
        exit 1
      fi
      i=$((i + 1))
      sleep 1
    done
    cat /tmp/grafana.log >&2
    kill "$server_pid" 2>/dev/null || true
    exit 1
  '
