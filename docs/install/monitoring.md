# Monitoring Loomarr

Loomarr exposes Prometheus metrics at `/v1/metrics`; the permanent `/metrics` alias returns the
same scrape. Both endpoints are intentionally unauthenticated because container runtimes and
Prometheus do not hold a Loomarr session.

Keep the endpoint on a trusted LAN or private container network. The supported Traefik listener
routes it with the rest of Loomarr and is not an internet-facing security boundary. If an external
Prometheus must cross an untrusted network, add TLS and authentication at an operator-managed edge
rather than exposing the default Compose listener.

## Check the scrape

From the Docker host:

```bash
curl -fsS http://localhost:8080/v1/metrics | head
```

A healthy response starts with Prometheus `HELP` and `TYPE` records. Loomarr also exports Go and
process collectors, so an otherwise idle instance still has data.

## Configure Prometheus

When Prometheus shares the Compose network, scrape Loomarr's private service port directly:

```yaml
scrape_configs:
  - job_name: loomarr
    metrics_path: /v1/metrics
    static_configs:
      - targets:
          - loomarr:8080
```

When it runs elsewhere on the trusted LAN, use the address that reaches Traefik instead, such as
`192.168.1.10:8080`. Do not add a session cookie or Loomarr API token; the metrics route does not
consume either one.

Each Loomarr label has a bounded operational meaning. Usernames, emails, Titles, Channel ids, media
ids, request ids, URLs, paths, prompts, errors, and secrets are never labels. HTTP `route` is the
matched route template rather than the requested path. This keeps the number of Prometheus series
bounded as a Library and household grow.

## Use the supplied Grafana dashboard

The repository's `observability/grafana/loomarr-overview.json` is one portable operational
overview. It uses a Prometheus datasource variable, so it does not assume a datasource UID. The
companion provider example in `observability/grafana/provisioning/` provisions the dashboard from
disk with a stable UID and disables UI updates.

Copy those files into an existing Grafana deployment and select the Prometheus datasource when the
dashboard opens. File provisioning is authoritative: replacing the JSON during an upgrade
overwrites local UI edits. Copy the dashboard under a different UID if you intentionally want a
locally maintained variant.

Optional recording and alert-rule examples live in `observability/prometheus/`. They do not install
Alertmanager, choose notification destinations, or impose a retention policy. Review thresholds
against the capacity and traffic of your installation before loading them.

Loomarr does not add Prometheus, Grafana, Alertmanager, credentials, storage, or monitoring ports to
its default Compose topology. These artifacts integrate with an observability stack you already
operate.
