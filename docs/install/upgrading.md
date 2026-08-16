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

The application backup is database-only. Copy `/data` separately if you need operator-uploaded
images or filler files; cached artwork and prepared media can be regenerated. Keep at least one
backup off the Loomarr data volume and, ideally, off the host.

## Pull

```bash
LOOMARR_VERSION=0.1.0-beta.1 docker compose -f docker/compose.yaml --profile sqlite pull
LOOMARR_VERSION=0.1.0-beta.1 docker compose -f docker/compose.yaml --profile sqlite up -d
```

Replace `0.1.0-beta.1` with the exact version you intend to run. Use the Postgres override files
from the install guide for a Postgres deployment.

Migrations run at boot. Watch the first start:

```bash
docker logs -f loomarr
curl -fsS http://localhost:8080/v1/readyz && echo ready
```

The app does not listen until migrations finish. A failed migration appears in the container log;
once the listener is up, `/v1/readyz` reports runtime readiness.

## Rolling back

Restore the backup, then run the older image.

- **SQLite** — stop the container, replace `/data/loomarr.db`, restore ownership to uid/gid 65532,
  set `LOOMARR_VERSION` to the older version, and start it. Confirm `/v1/readyz`, then sign in and
  inspect a channel before discarding the newer backup.
- **Postgres** — stop Loomarr, restore the dump into an empty database, set `LOOMARR_VERSION` to the
  older version, and start with the Postgres override. Confirm readiness and a channel before
  discarding the newer dump.

Running an older binary against a newer database isn't supported. The new schema may contain
changes the old code can't read.

## What survives

Everything on the `/data` volume: database, artwork cache, filler clips. Generated secrets live
in the database, so tokens don't rotate on upgrade.

Check your compose file mounts `/data` — an upgrade is when you'd find out it doesn't, because
`pull` recreates the container.
