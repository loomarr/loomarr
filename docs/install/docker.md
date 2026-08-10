# Docker install

## Before you start

Have your media server's URL and an admin API key ready. Everything else can be added later
from Settings.

> **There is no published image yet.** No `v*` tag has been cut, so `ghcr.io/mantonx/loomarr`
> does not exist and the compose file below **builds locally** from a clone. The first build
> takes a while — it downloads a speech model used for filler. Once a release is tagged this
> page will switch to a plain `docker pull`.

## Start it

```bash
git clone https://github.com/mantonx/loomarr && cd loomarr
docker compose -f docker/compose.yaml --profile sqlite up -d
```

Profiles, which combine:

| Profile | What it adds |
| --- | --- |
| `sqlite` | The default database. One file on the `/data` volume. Right for almost everyone. |
| `postgres` | A Postgres container instead. Choose it if you already run Postgres or expect many concurrent channels. |
| `ai` | A local Ollama alongside Loomarr. Omit it if you use a hosted provider or already run Ollama. |

So a typical first run with local AI:

```bash
docker compose -f docker/compose.yaml --profile sqlite --profile ai up -d
```

## Configure it

Every setting can be entered in the UI, so you can start with none. The three worth setting in
the environment up front:

```bash
SERVER_PUBLIC_URL=http://192.168.1.10:8080   # how your media server reaches Loomarr
LIBRARY_URL=http://192.168.1.10:8096         # your Emby/Jellyfin
LIBRARY_TOKEN=…                              # an admin API key
```

Two rules that cause most first-run problems:

- **`localhost` inside a container means the container.** Use service names (`http://emby:8096`)
  if they share a Docker network, or your host's LAN IP if they don't.
- **An environment variable wins and locks its field in Settings.** That is deliberate — it's
  how a GitOps setup pins values — but it means a field you can't edit in the UI is a field
  something is setting in your compose file.

The full list is generated from the source: **[configuration reference](../configuration.md)**.

## Data and backups

Everything lives on one named volume:

| Path | Contents |
| --- | --- |
| `/data/loomarr.db` | The database — accounts, channels, settings, secrets |
| `/data/filler/` | Your commercial and bumper clips |
| `/data/images/` | Cached artwork |

> ⚠ **If you write your own compose file, mount `/data`.** With the default
> `sqlite:///data/loomarr.db` and no mount, the database goes into the container's writable
> layer and is destroyed by the next `up --force-recreate` or image pull. The shipped compose
> file mounts it; this warning exists because for a long time it did not.

Back up SQLite with `GET /v1/backup`, which streams a consistent snapshot:

```bash
curl -sH "Authorization: Bearer $API_TOKEN" \
  http://localhost:8080/v1/backup > loomarr-$(date +%F).db
```

On Postgres use `pg_dump` — the image ships no Postgres client, so `/v1/backup` returns 501
there. **Backups contain secrets.** Store them accordingly.

## First boot

Open `http://<host>:8080`. A fresh install goes straight to the setup wizard — see
**[Quickstart](../help/quickstart.md)** for the walkthrough.

To check it's alive before opening a browser:

```bash
curl -fsS http://localhost:8080/v1/readyz && echo ready
```

`/v1/readyz` answers 503 with a reason until the database is open and migrated; `/v1/healthz`
answers as soon as the process serves. Both are unauthenticated on the LAN so orchestrators can
use them. The container's own healthcheck runs `loomarr healthcheck`, which probes the same
readiness endpoint.

## Filler clips

If you want commercials between programmes, point `FILLER_DIR` at a folder of clips. On the
default playout backend that's all that's needed. **On the Tunarr backend the same host folder
must be mounted into Tunarr at the same path**, because Tunarr scans it as a local media source.

## Troubleshooting

The wizard live-tests every connection and each red check links to its fix in
**[Troubleshooting](../help/troubleshooting.md)**. The two that catch people out:

- **Channels appear in the guide but won't play** — almost always `SERVER_PUBLIC_URL`. ffmpeg
  fetches the playlist as a separate process and has no notion of "the origin this came from",
  so there is no relative fallback and a wrong value fails only at tune time.
- **The container reports unhealthy** — check `docker logs`; readiness stays red while the
  database is unreachable, and the reason is in the `/v1/readyz` body.
