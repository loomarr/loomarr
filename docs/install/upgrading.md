# Upgrading

**Migrations are forward-only.** There is no downgrade path, so the ritual is always:

> **Back up, then pull.**

## Back up

SQLite — `GET /v1/backup` streams a consistent snapshot while the app runs:

```bash
curl -sH "Authorization: Bearer $API_TOKEN" \
  http://localhost:8080/v1/backup > loomarr-$(date +%F).db
```

Postgres — use `pg_dump`. The image ships no Postgres client, so `/v1/backup` returns 501 on
that backend rather than pretending.

**Backups contain secrets** — session keys, API tokens, and every credential you entered.
Treat them like the database they are.

## Pull

```bash
docker compose -f docker/compose.yaml --profile sqlite pull
docker compose -f docker/compose.yaml --profile sqlite up -d
```

Migrations run automatically at boot (`AUTO_MIGRATE=true`). Watch the first start:

```bash
docker logs -f loomarr
curl -fsS http://localhost:8080/v1/readyz && echo ready
```

`/v1/readyz` stays 503 with a reason while migrations are running or the store is unreachable,
so it is the honest signal that an upgrade landed.

## Rolling back

Restore the backup, then run the older image.

- **SQLite** — stop the container, replace `/data/loomarr.db` with your backup, start the older
  tag.
- **Postgres** — restore the dump into an empty database, then start the older tag.

Running an older binary against a *newer* database is not supported. Forward-only means the new
schema may contain changes the old code cannot read, and it will not warn you politely.

## What survives

Everything on the `/data` volume: the database, artwork cache, and filler clips. Generated
secrets persist in the database, so tokens do not rotate on upgrade.

> ⚠ **Check your compose file mounts `/data`.** If it doesn't, an upgrade is exactly when you
> find out — `pull` recreates the container and the database goes with it. The shipped compose
> file mounts it correctly.
