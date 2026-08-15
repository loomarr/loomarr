# Loomarr

[![CI](https://github.com/mantonx/loomarr/actions/workflows/ci.yml/badge.svg)](https://github.com/mantonx/loomarr/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)

**Describe a TV channel in a sentence. Loomarr builds it, plays it, and keeps it running.**

> *"90s Saturday morning cartoons for the kids"*

Loomarr picks a lineup from your library, requests anything missing, schedules it with ad
breaks, and streams it to your media server as Live TV.

```mermaid
graph LR
  I["<b>Your intent</b><br/><i>one sentence</i>"]
  P["<b>Proposal</b><br/>lineup + what's missing"]
  A{"<b>You approve</b>"}
  C["<b>Channel</b><br/>scheduled with ad breaks"]
  T["<b>Live TV</b><br/>in Emby / Jellyfin"]
  Q["<b>Acquire</b><br/>Seerr → Sonarr/Radarr"]

  I --> P
  P --> A
  C --> T
  A --> C
  A --> Q
  Q -.->|"fills in as titles arrive"| C

  classDef hi fill:#8a5a1a,stroke:#5c3b10,color:#fff
  classDef norm fill:#2b3b52,stroke:#1b2736,color:#dbe4ef
  classDef go fill:#1f6f4a,stroke:#134a31,color:#fff
  class A hi
  class T go
  class I,P,C,Q norm
```

Every pick is a real title from your library or TMDB — the model can't invent one. Nothing
downloads until an admin approves.

## Install

You need Emby or Jellyfin. Everything else is optional.

```bash
git clone https://github.com/mantonx/loomarr && cd loomarr
docker compose -f docker/compose.yaml --profile sqlite up -d
```

Open `http://<host>:8080` and follow the wizard.

There's no published image yet, so this builds from source and the first run takes a few
minutes.

→ [Install guide](docs/install/index.md) · [Docker](docs/install/docker.md) ·
[Hardware acceleration](docs/install/hardware.md) · [Upgrading](docs/install/upgrading.md) ·
[All settings](docs/configuration.md)

## How it plays

Loomarr streams your channels itself by default — your media server picks it up as Live TV,
with nothing else to install. **Tunarr** is a supported alternative if your hardware can't
transcode, or if you already run it. You choose in the wizard, and can override it per channel.

## Develop

Go 1.26+, the Rust toolchain pinned by `rust-toolchain.toml`, Node 22.x, `ffmpeg` and `ffprobe` on `PATH`, Docker for the Postgres and browser
test suites.

```bash
make doctor         # toolchain and local-state diagnostics
make bootstrap      # Rust worker + frontend dependencies + codegen
make dev-be         # isolated Go/Rust backend with live reload
make dev-fe         # isolated frontend pointed at that backend
```

→ [Developer guide](docs/dev/index.md) · [Setup](docs/dev/setup.md) ·
[Dev loop](docs/dev/dev-loop.md) · [Testing](docs/dev/testing.md) · [CI](docs/dev/ci.md) ·
[Commands](docs/dev/commands.md) · [Agent development](docs/dev/agents.md)

## Documentation

| For | Where |
| --- | --- |
| Installing | [`docs/install/`](docs/install/index.md) |
| Using it | [`docs/help/`](docs/help/quickstart.md) — also in the app under **Help** |
| Contributing | [`docs/dev/`](docs/dev/index.md), [`CONTRIBUTING.md`](CONTRIBUTING.md) |
| How it works | [`docs/design.md`](docs/design.md) |

The help pages are built into the binary, so they work offline. The docs site renders the same
files.

## Stack

One Go binary with the UI, help pages and API built in. Huma v2 for the API (OpenAPI 3.1,
consumed by the frontend via orval), `database/sql` over SQLite or Postgres, goose migrations,
an embedded Vite + React SPA, ffmpeg for playout, and Ollama or any OpenAI-compatible provider
for the LLM. Details in [`docs/design.md`](docs/design.md) §14.

## Operations

- **Probes** — `/v1/healthz` and `/v1/readyz`, unauthenticated on the LAN. `/healthz` and
  `/readyz` also work.
- **Metrics** — `/v1/metrics` in Prometheus format: HTTP rates and latency, Go runtime, state
  gauges, per-dependency outbound timing, and channel reconcile timing.
- **Backup** — `GET /v1/backup` for SQLite, `pg_dump` for Postgres. Backups contain secrets.

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) and the [Code of Conduct](CODE_OF_CONDUCT.md).
Security issues: [`SECURITY.md`](SECURITY.md).

## License

[MIT](LICENSE). Bundled components, including the GPL `ffmpeg`, are listed in
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
