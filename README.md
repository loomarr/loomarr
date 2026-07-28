# Loomarr

[![CI](https://github.com/mantonx/loomarr/actions/workflows/ci.yml/badge.svg)](https://github.com/mantonx/loomarr/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)

Turn a natural-language channel intent into a live, self-maintaining
[Tunarr](https://tunarr.com) channel: suggest a lineup (LLM, grounded against your library),
acquire what's missing (Seerr → Sonarr/Radarr), schedule and insert era-appropriate commercial
pods, push to Tunarr, and backfill as content lands.

> **Status:** under construction, built in verifiable phases (0–14). See
> [`PROGRESS.md`](PROGRESS.md) for the current phase and [`docs/design.md`](docs/design.md) for
> the full design (the single source of truth).

## Stack

Go 1.26 single binary · stdlib `net/http` + [Huma v2](https://huma.rocks) (code-first OpenAPI
3.1, exported to [`api/openapi.yaml`](api/openapi.yaml) and consumed by the frontend via orval) ·
`database/sql` over `modernc.org/sqlite` **or** `pgx` (Postgres) · goose migrations · an
embedded Vite + React + TypeScript SPA (and the help docs) baked into the binary, so a single
image serves API + UI + docs offline. LLM via local **Ollama** or any OpenAI-compatible provider.
Distroless, non-root, cgo-free (~8 MB image). Full rationale in [`docs/design.md`](docs/design.md) §14.

## Develop

Requires Go 1.26+, Node 22.5+, `ffmpeg`/`ffprobe` on PATH, Docker (from Phase 4 for Postgres testcontainers).

```bash
make check      # fmt + vet + lint + unit tests  (the default gate)
make build      # static binary -> bin/loomarr
make dev        # dev dependencies (persistent Tunarr wired to Emby)
```

The full `make` contract (`test-pg`, `openapi`, `fe`, `e2e`, `seed`, …) is in the
[`Makefile`](Makefile); unimplemented targets fail loudly until their phase lands.

Copy [`.env.example`](.env.example) to `.env` and fill in (config reference: `docs/design.md` §15).

## Run (Docker)

```bash
docker compose -f docker/compose.yaml --profile sqlite   up -d   # SQLite backend
docker compose -f docker/compose.yaml --profile postgres up -d   # Postgres backend
```

Add `--profile ai` to either to run a local Ollama (the default `LLM_PROVIDER`); omit it
for a hosted or external LLM. Filler in-app clip download is the `loomarr:filler` image
tag, not a profile (see `docs/design.md` §16).

## Layout

```text
cmd/loomarr/         # main (Phase 1)
internal/config/     # §15 env config
internal/httpx/      # shared outbound HTTP client factory (§6 timeouts)
internal/api/        # inbound HTTP (§7; Huma mounts here in Phase 8)
internal/testkit/    # shared mocks + Phase-0 pinned fixtures (all tests use these)
internal/…           # provision, store, library, schedule, suggest, filler (later phases)
api/vendor/          # pinned external specs (Tunarr OpenAPI)
docs/                # design.md (truth), companion designs, help/ (in-app Help), integrations/
docker/              # deployment + dev compose, Dockerfile context
```

## Documentation

- **In-app Help** — the user-facing set ([`docs/help/`](docs/help/)) is embedded and served at
  `/v1/docs`; open **Help** in the app. Start with [Quickstart](docs/help/quickstart.md).
- **Design** — [`docs/design.md`](docs/design.md) is the single source of truth. Companion
  designs: [`programming-design.md`](docs/programming-design.md),
  [`config-design.md`](docs/config-design.md), [`frontend-design.md`](docs/frontend-design.md).

## Operations

- **Config** — everything is an env var ([`.env.example`](.env.example)); an env-set value locks
  its field in Settings. Full reference: `docs/design.md` §15.
- **Backup** — SQLite: `GET /v1/backup` streams a consistent snapshot, e.g. nightly:
  `curl -sH "Authorization: Bearer $API_TOKEN" localhost:8080/v1/backup > loomarr-$(date +%F).db`.
  Postgres: use `pg_dump` directly (the scratch image ships none, so `/v1/backup` returns 501).
  Backups contain secrets — keep them safe.
- **Upgrade** — migrations are forward-only, so the ritual is **back up, then pull.** Restore a
  SQLite install by replacing `/data/loomarr.db`.
- **Probes** — `/healthz` and `/readyz` are unauthenticated on the LAN for Docker
  healthchecks and orchestrators.
- **Metrics** — `/metrics` exposes Prometheus text (unauthenticated on the LAN). Currently:
  HTTP request rate/errors/latency (`loomarr_http_*`, labelled by method and matched route),
  the Go runtime + process collectors, the state gauges (`loomarr_titles{state}`,
  `loomarr_jobs{status}`, `loomarr_active_sessions`), the event counters
  (`loomarr_auth_logins_total{result}`, `loomarr_webhook_events_total{type}`), and the
  latency series — outbound client RED per dependency (`loomarr_outbound_requests_total`,
  `loomarr_outbound_request_duration_seconds`, labelled by `target`: tunarr/library/llm/…)
  and channel reconcile timing (`loomarr_channel_reconciles_total{result}`,
  `loomarr_channel_reconcile_duration_seconds`); and the domain counters —
  `loomarr_llm_tokens_total{kind}`, `loomarr_filler_pods_total{match_level}` (fallback-ladder
  depth), `loomarr_channel_slot_substitutions_total` (slot drift). This covers the §17 metric
  set; **cost** is intentionally left to a dashboard recording rule (tokens × your provider's
  posted rate) rather than a baked-in price table that would drift.

## Contributing

Contributions are welcome — see [`CONTRIBUTING.md`](CONTRIBUTING.md) for setup and
the conventions that keep the tree green (doc-first, one gate per change, generated
files never hand-edited). By participating you agree to the
[Code of Conduct](CODE_OF_CONDUCT.md). Security issues: [`SECURITY.md`](SECURITY.md).

## License

[MIT](LICENSE). Bundled and vendored components (notably the GPL `ffmpeg` in the
`loomarr:filler` image) are inventoried in
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
