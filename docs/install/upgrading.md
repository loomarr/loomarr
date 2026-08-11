# Upgrading

Migrations only run forward, so: **back up, then pull.**

## Back up

SQLite — this streams a consistent snapshot while the app runs:

```bash
curl -sH "Authorization: Bearer $API_TOKEN" \
  http://localhost:8080/v1/backup > loomarr-$(date +%F).db
```

Postgres — use `pg_dump`. The image ships no Postgres client, so `/v1/backup` returns 501 there.

Backups contain secrets: session keys, API tokens, and every credential you entered.

## Pull

```bash
docker compose -f docker/compose.yaml --profile sqlite pull
docker compose -f docker/compose.yaml --profile sqlite up -d
```

Migrations run at boot. Watch the first start:

```bash
docker logs -f loomarr
curl -fsS http://localhost:8080/v1/readyz && echo ready
```

`/v1/readyz` stays 503 with a reason while migrations run.

## Rolling back

Restore the backup, then run the older image.

- **SQLite** — stop the container, replace `/data/loomarr.db`, start the older tag.
- **Postgres** — restore the dump into an empty database, then start the older tag.

Running an older binary against a newer database isn't supported. The new schema may contain
changes the old code can't read.

## What survives

Everything on the `/data` volume: database, artwork cache, filler clips. Generated secrets live
in the database, so tokens don't rotate on upgrade.

Check your compose file mounts `/data` — an upgrade is when you'd find out it doesn't, because
`pull` recreates the container.
