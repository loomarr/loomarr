#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
compose_file="$repo_root/docker/compose.observability.yaml"

rendered=$(
  LOOMARR_DEV_PORT=18888 \
    PROMETHEUS_DEV_PORT=29090 \
    GRAFANA_DEV_PORT=23000 \
    docker compose \
      -p loomarr-observability-contract \
      -f "$compose_file" \
      config --format json
)

jq -e '
  (.services | keys) == ["grafana", "prometheus"] and
  .services.prometheus.image == "prom/prometheus:v3.14.0@sha256:5ce7540c3c00ef4ab0c9d2c995c6a5b9c421f44b4a115d97a2c7af3b1c21cbb0" and
  .services.grafana.image == "grafana/grafana:13.2.0@sha256:3fd54ae1214669f8355f065ec9f6445d5279a3d77095ab048ca045685272429b" and
  .services.prometheus.ports == [{"mode":"ingress","target":9090,"published":"29090","protocol":"tcp","host_ip":"127.0.0.1"}] and
  .services.grafana.ports == [{"mode":"ingress","target":3000,"published":"23000","protocol":"tcp","host_ip":"127.0.0.1"}] and
  .services.prometheus.extra_hosts == ["host.docker.internal=host-gateway"] and
  (.configs.prometheus_config.content | contains("host.docker.internal:18888")) and
  (.services.prometheus.volumes | any(.type == "bind" and (.source | endswith("/observability/prometheus")) and .read_only == true)) and
  (.services.prometheus.volumes | any(.type == "volume" and .target == "/prometheus" and .read_only != true)) and
  (.services.grafana.volumes | any(.type == "bind" and (.source | endswith("/observability/grafana/provisioning")) and .read_only == true)) and
  (.services.grafana.volumes | any(.type == "bind" and (.source | endswith("/observability/grafana/dashboards")) and .read_only == true)) and
  (.services.grafana.volumes | any(.type == "volume" and .target == "/var/lib/grafana" and .read_only != true))
' <<<"$rendered" >/dev/null

echo 'observability-dev-test: isolated Compose topology is wired'
