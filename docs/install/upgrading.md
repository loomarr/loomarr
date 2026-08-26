# Upgrading

Migrations only run forward, so: **back up, then pull.**

## Back up

The simplest way is the admin UI: **Settings → System → Backup**, which downloads a snapshot with
your signed-in session — no token needed. To script it instead, use the endpoint below.

`/v1/backup` is admin-only. The `API_TOKEN` it needs is **not** the internal one Loomarr
auto-generates; reveal a usable value under **Settings → Secrets** (eye toggle + copy), or set your
own `API_TOKEN=<something-long>` in `.env` and restart. Export it as `$API_TOKEN` before the curl —
without it the request is `401`.

SQLite — this streams a consistent snapshot while the app runs:

```bash
backup="loomarr-$(date +%F).db"
umask 077
curl -fsS -H "Authorization: Bearer $API_TOKEN" \
  http://localhost:8080/v1/backup > "$backup.partial" &&
  chmod 600 "$backup.partial" &&
  mv "$backup.partial" "$backup"
```

Postgres — create the custom archive `pg_restore` expects. Loomarr's image ships no Postgres client
and the supported Compose stack keeps the database off the host network, so run the client inside
the Postgres service and stream the archive to the trusted host; `/v1/backup` returns 501 there.

```bash
compose=(docker compose -f docker/compose.yaml -f docker/compose.postgres.yaml --profile postgres)
archive="loomarr-$(date +%F).dump"
umask 077
"${compose[@]}" exec -T postgres sh -ceu \
  'pg_dump --format=custom --username="$POSTGRES_USER" --dbname="$POSTGRES_DB"' \
  > "$archive.partial" &&
  chmod 600 "$archive.partial" &&
  mv "$archive.partial" "$archive"
```

Backups contain secrets: session keys, API tokens, and every credential you entered.

The application backup is database-only. Copy `/data` separately if you need operator-uploaded
images or filler files; cached artwork and prepared media can be regenerated. Keep at least one
backup off the Loomarr data volume and, ideally, off the host.

## Pull

```bash
NEXT_VERSION=0.1.0-beta.8 # replace with the exact release you intend to run
LOOMARR_VERSION="$NEXT_VERSION" docker compose -f docker/compose.yaml --profile sqlite pull
LOOMARR_VERSION="$NEXT_VERSION" docker compose -f docker/compose.yaml --profile sqlite up -d
```

Use the Postgres override files from the install guide for a Postgres deployment.

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
- **Postgres** — use the same Compose files as the running stack, stop Loomarr, replace only the
  Loomarr database, set `LOOMARR_VERSION` to the older version, and start with the Postgres
  override. The replacement database is owned by the configured Postgres user:

  ```bash
  compose=(docker compose -f docker/compose.yaml -f docker/compose.postgres.yaml --profile postgres)
  "${compose[@]}" stop loomarr
  "${compose[@]}" exec -T postgres sh -ceu '
    dropdb --username="$POSTGRES_USER" --force "$POSTGRES_DB"
    createdb --username="$POSTGRES_USER" --owner="$POSTGRES_USER" "$POSTGRES_DB"
    pg_restore --username="$POSTGRES_USER" --dbname="$POSTGRES_DB" --exit-on-error
  ' < loomarr-YYYY-MM-DD.dump
  "${compose[@]}" up -d loomarr traefik
  ```

  Confirm readiness, sign-in, and a channel before discarding the newer dump.

Running an older binary against a newer database isn't supported. The new schema may contain
changes the old code can't read.

## What survives

Everything on the `/data` volume: database, artwork cache, filler clips. Generated secrets live
in the database, so tokens don't rotate on upgrade.

Check your compose file mounts `/data` — an upgrade is when you'd find out it doesn't, because
`pull` recreates the container.
