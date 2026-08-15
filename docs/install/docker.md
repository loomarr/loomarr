# Docker install

You'll need your Emby or Jellyfin URL and an admin API key. Everything else can be added later
from Settings.

## Start it

```bash
git clone https://github.com/mantonx/loomarr && cd loomarr
docker compose -f docker/compose.yaml --profile sqlite up -d
```

There's no published image yet, so this builds from source. The first build takes a few minutes.

Profiles combine:

| Profile | Adds |
| --- | --- |
| `sqlite` | The default database — one file on the `/data` volume |
| `postgres` | A Postgres container instead |
| `ai` | A local Ollama. Omit it if you use a hosted provider or run Ollama already |

Open `http://<host>:8080` and follow the wizard — see [Quickstart](../help/quickstart.md).

## Settings

Everything can be entered in the UI, so you can start with none. Three are worth setting up
front:

```bash
SERVER_PUBLIC_URL=http://192.168.1.10:8080   # how your media server reaches Loomarr
LIBRARY_URL=http://192.168.1.10:8096         # your Emby/Jellyfin
LIBRARY_TOKEN=…                              # an admin API key
```

Two things that cause most first-run problems:

- **`localhost` inside a container means the container.** Use service names
  (`http://emby:8096`) if they share a Docker network, or your host's LAN IP if not.
- **An environment variable wins over the UI and locks that field.** If a setting won't edit,
  something in your compose file is setting it.

Full list: [configuration reference](../configuration.md).

## Data and backups

Everything lives on one volume:

| Path | Contents |
| --- | --- |
| `/data/loomarr.db` | Database — accounts, channels, settings, secrets |
| `/data/filler/` | Commercial and bumper clips |
| `/data/images/` | Cached artwork |
| `/data/prepared/` | Reusable prepared programme media for instant channel changes |

If you write your own compose file, **mount `/data`**. Without it the database goes into the
container's writable layer and is lost on the next `up --force-recreate` or image pull.

Back up SQLite:

```bash
curl -sH "Authorization: Bearer $API_TOKEN" \
  http://localhost:8080/v1/backup > loomarr-$(date +%F).db
```

On Postgres use `pg_dump` — the image ships no Postgres client, so `/v1/backup` returns 501
there. Backups contain secrets.

## Checking it's up

```bash
curl -fsS http://localhost:8080/v1/readyz && echo ready
```

`/v1/readyz` returns 503 with a reason until the database is open and migrated. `/v1/healthz`
answers as soon as the process is serving. The container's healthcheck runs
`loomarr healthcheck`, which probes the same endpoint.

## Filler clips

To play commercials between programmes, point `FILLER_DIR` at a folder of clips.

On the Tunarr backend, mount the same host folder into Tunarr at the same path so it can scan
them.

## When something's wrong

The wizard tests every connection and each failure links to its fix in
[Troubleshooting](../help/troubleshooting.md). The two most common:

- **Channels appear in the guide but won't play** — usually `SERVER_PUBLIC_URL`. ffmpeg fetches
  the playlist as a separate process, so a wrong value fails only at tune time.
- **The container reports unhealthy** — check `docker logs`. Readiness stays red while the
  database is unreachable, and the reason is in the `/v1/readyz` body.
