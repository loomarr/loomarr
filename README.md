# Loomarr

Turn a natural-language channel intent into a live, self-maintaining
[Tunarr](https://tunarr.com) channel: suggest a lineup (LLM, grounded against your library),
acquire what's missing (Seerr → Sonarr/Radarr), schedule and insert era-appropriate commercial
pods, push to Tunarr, and backfill as content lands.

> **Status:** under construction, built in verifiable phases (0–14). See
> [`PROGRESS.md`](PROGRESS.md) for the current phase and [`docs/design.md`](docs/design.md) for
> the full design (the single source of truth).

## Stack

Go 1.26 single binary · stdlib `net/http` + [Huma v2](https://huma.rocks) (code-first OpenAPI
3.1) · `database/sql` over `modernc.org/sqlite` **or** `pgx` (Postgres) · goose migrations ·
embedded Vite + React + TypeScript SPA (Phase 13). Distroless, non-root, cgo-free (~8 MB image).
Full rationale in [`docs/design.md`](docs/design.md) §14.

## Develop

Requires Go 1.26+, Node 20+, Docker (from Phase 4 for Postgres testcontainers).

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

## Layout

```
cmd/loomarr/         # main (Phase 1)
internal/config/     # §15 env config
internal/httpx/      # shared outbound HTTP client factory (§6 timeouts)
internal/api/        # inbound HTTP (§7; Huma mounts here in Phase 8)
internal/testkit/    # shared mocks + Phase-0 pinned fixtures (all tests use these)
internal/…           # provision, store, library, schedule, suggest, filler (later phases)
api/vendor/          # pinned external specs (Tunarr OpenAPI)
docs/                # design.md (truth), engineering/, product/ (in-app Help, Phase 13)
docker/              # deployment + dev compose, Dockerfile context
```

## License

MIT.
