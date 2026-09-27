# Monitoring Loomarr

Loomarr exposes Prometheus metrics at `/v1/metrics`; the permanent `/metrics` alias returns the
same scrape. **Both require a scrape token**, sent as `Authorization: Bearer <token>`. Prometheus
holds no Loomarr session, so the token is its own credential, separate from `API_TOKEN` and from
every user login: a session, a member or admin sign-in, or the API token does not unlock it.

Without a token configured the endpoints are **refused** (`403`) rather than served openly, and
Loomarr logs a one-line warning at start-up. A missing or wrong token gets `401`.

> **Upgrading?** Earlier releases served `/metrics` with no credential. An existing Prometheus job
> goes `down` until you set the token below **and** add it to that job's `scrape_config`. The
> path is unchanged.

Keep the listener on a trusted LAN or private container network regardless: the token stops
anonymous readers, but the supported Traefik listener is plain HTTP, so the token travels in the
clear. If Prometheus must cross an untrusted network, add TLS at an operator-managed edge.

## Set the scrape token

Generate a long random value and give it to Loomarr with either variable (set only one; setting
both stops start-up):

```bash
openssl rand -hex 32 > metrics-token && chmod 600 metrics-token
```

- `LOOMARR_METRICS_TOKEN` — the value itself (for example in `.env`).
- `LOOMARR_METRICS_TOKEN_FILE` — a path to a file containing it, the Docker-secrets idiom.
  The path is read **inside the container**, so mount the file there. A trailing newline is
  ignored. An unreadable file stops start-up instead of leaving `/metrics` unprotected.

The token is never written to the log or echoed in an error.

## Check the scrape

From the Docker host (substitute your token):

```bash
curl -fsS -H "Authorization: Bearer $(cat metrics-token)" http://localhost:8080/v1/metrics | head
```

`curl` without the header should return `401`, and `403` if no token is configured yet.

A healthy response starts with Prometheus `HELP` and `TYPE` records. Loomarr also exports Go and
process collectors, so an otherwise idle instance still has data.

## Run the local seeded stack

The repository includes an opt-in development stack for inspecting real metrics without changing
the default or release Compose deployments. Start it before the backend so a missing worktree
SQLite database is populated through Loomarr's real seed command:

```bash
make observability-dev
make dev-be
```

Keep `make dev-be` running in its own terminal. The first command starts only Prometheus and
Grafana; Loomarr continues to run directly on the host with live reload. The launcher prints this
worktree's URLs. In the primary worktree they default to:

- Loomarr: `http://localhost:8080`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (`admin` / `loomarr`)

Sibling worktrees receive deterministic, non-overlapping ports, Compose project names, SQLite
paths, and monitoring volumes. Existing SQLite databases are never reseeded. A configured
PostgreSQL database is scraped as-is; the launcher does not write fixtures to it.

Stop only the monitoring containers with `make observability-dev-down`. Their Prometheus and
Grafana volumes are preserved for the next session, and the seeded Loomarr database is left in
place. The stack binds its UI ports to loopback and is for local development only.

To verify the complete path independently, run `make observability-dev-test`. It creates a
temporary SQLite fixture through `cmd/seed`, starts the real Loomarr binary and both pinned
containers, and then uses HTTP to prove that Prometheus scraped a seeded `loomarr_titles` value and
Grafana loaded dashboard UID `loomarr-overview`. The test removes its temporary containers,
volumes, database, and backend when it exits.

## Configure Prometheus

When Prometheus shares the Compose network, scrape Loomarr's private service port directly:

```yaml
scrape_configs:
  - job_name: loomarr
    metrics_path: /v1/metrics
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/loomarr-metrics-token
    static_configs:
      - targets:
          - loomarr:8080
```

Mount the same token file into the Prometheus container at that path (`credentials_file` is the
current name; older Prometheus releases call it `bearer_token_file`). When Prometheus runs
elsewhere on the trusted LAN, use the address that reaches Traefik instead, such as
`192.168.1.10:8080`. Do not add a session cookie or Loomarr API token; the metrics route accepts
only the scrape token.

Each Loomarr label has a bounded operational meaning. Usernames, emails, Titles, Channel ids, media
ids, request ids, URLs, paths, prompts, errors, and secrets are never labels. HTTP `route` is the
matched route template rather than the requested path. This keeps the number of Prometheus series
bounded as a Library and household grow.

## Use the supplied Grafana dashboard

The repository's `observability/grafana/dashboards/loomarr-overview.json` is one portable operational
overview. It uses a Prometheus datasource variable, so it does not assume a datasource UID. The
companion files in `observability/grafana/provisioning/` provision the dashboard and a Prometheus
datasource from disk. Set `PROMETHEUS_URL` in Grafana's environment to the URL Grafana can use to
reach Prometheus. The dashboard has a stable UID and disables UI updates.

Copy those files into an existing Grafana deployment and select the Prometheus datasource when the
dashboard opens. File provisioning is authoritative: replacing the JSON during an upgrade
overwrites local UI edits. Copy the dashboard under a different UID if you intentionally want a
locally maintained variant.

Optional recording and alert-rule examples live in `observability/prometheus/`. They do not install
Alertmanager, choose notification destinations, or impose a retention policy. Review thresholds
against the capacity and traffic of your installation before loading them.

Run `make observability-verify` after changing the manifest, dashboard, provisioning, or rules. It
uses pinned Prometheus and Grafana containers to check rule behavior and prove Grafana can load the
dashboard by its UID.

Loomarr does not add Prometheus, Grafana, Alertmanager, credentials, storage, or monitoring ports to
its default Compose topology. The local stack is an explicit development overlay; the remaining
artifacts integrate with an observability stack you already operate.
