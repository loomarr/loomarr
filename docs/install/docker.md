# Docker install

You'll need Docker Engine on Linux or Docker Desktop on macOS, plus your Emby or Jellyfin URL and
an admin API key. TMDB and an LLM are also required to create a channel from a sentence; requesters
and filler can be added later from Settings.

Until `v0.1.0-beta.1` appears in GitHub Releases, the command below is the release-candidate
contract rather than an available download.

## Start it

```bash
git clone --branch v0.1.0-beta.1 --depth 1 https://github.com/mantonx/loomarr && cd loomarr
cp .env.example .env
# Edit .env: SERVER_PUBLIC_URL must be a URL this host and your media server can reach.
LOOMARR_VERSION=0.1.0-beta.1 docker compose -f docker/compose.yaml --profile sqlite up -d
```

This pulls the pinned `ghcr.io/mantonx/loomarr:0.1.0-beta.1` image. Linux hosts use the native
amd64 or arm64 manifest. Docker Desktop does the same on Intel or Apple Silicon Macs.

Profiles combine:

| Profile | Adds |
| --- | --- |
| `sqlite` | The default database — one file on the `/data` volume |
| `postgres` | A Postgres container, selected with `docker/compose.postgres.yaml` |
| `ai` | A local Ollama. Omit it if you use a hosted provider or run Ollama already |

Open `SERVER_PUBLIC_URL` and follow the wizard — see [Quickstart](../help/quickstart.md). The
supported Compose topology publishes Traefik on host port 8080; Loomarr's port 8080 is private.
Set `LOOMARR_HTTP_PORT` in `.env` if the host must publish a different port, and include that port
in `SERVER_PUBLIC_URL`.

The default Traefik entrypoint is plain HTTP for the trusted-LAN deployment model in
[`SECURITY.md`](../../SECURITY.md). It is not an internet-facing TLS configuration. Keep the Docker
host trusted: Traefik's Docker socket bind is marked read-only, but the Docker API itself is a
host-control boundary.

For Postgres, use the explicit database override; a Compose profile can start Postgres but cannot
change Loomarr's SQLite default by itself:

```bash
LOOMARR_VERSION=0.1.0-beta.1 docker compose \
  -f docker/compose.yaml -f docker/compose.postgres.yaml \
  --profile postgres up -d
```

## Settings

Most application settings can be entered in the UI. Release Compose requires the canonical public
URL before it starts; the media-server values are also useful to set up front:

```bash
SERVER_PUBLIC_URL=http://192.168.1.10:8080   # how your media server reaches Traefik → Loomarr
LOOMARR_HTTP_PORT=8080                       # host port owned by Traefik
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

Runtime state defaults under `/data`, but an application backup is a **database snapshot**, not a
copy of that volume:

| Path | Contents |
| --- | --- |
| `/data/loomarr.db` | Database — accounts, channels, settings, secrets |
| `/data/filler/` | Commercial and bumper clips |
| `/data/images/` | Cached artwork |
| `/data/prepared/` | Reusable prepared programme media for instant channel changes |

The database backup contains accounts, channels, settings, and secrets. It does not contain filler
files, prepared media, cached artwork, or operator-uploaded images. Copy the `/data` volume as part
of host-level backup if those files matter; cached and prepared derivatives can be regenerated.

Prepared media is bounded by the hot-applied `PLAYOUT_PREPARED_BUDGET_GB` soft cap (512 GiB by
default). Keep enough free space for one programme beyond the cap because packaging commits before
the retention pass; recently played programmes remain protected even if that temporarily exceeds
the cap.

If you write your own compose file, **mount `/data`**. Without it the database goes into the
container's writable layer and is lost on the next `up --force-recreate` or image pull.

Back up SQLite:

```bash
curl -sH "Authorization: Bearer $API_TOKEN" \
  http://localhost:8080/v1/backup > loomarr-$(date +%F).db
```

On Postgres use `pg_dump` — the image ships no Postgres client, so `/v1/backup` returns 501
there. Backups contain secrets.

Scheduled backups default to `/data/backups`, which protects against a bad migration or application
change but not loss of the host disk or the whole volume. Copy them to another disk or host.

## Checking it's up

```bash
curl -fsS http://localhost:8080/v1/readyz && echo ready
```

The Loomarr HTTP listener starts only after the database opens and migrations finish. Traefik waits
for Loomarr's container healthcheck, then independently probes `/v1/readyz` before routing traffic.
If startup fails, inspect `docker compose logs loomarr traefik`; there is no readiness response from
a process that never started listening.

Traefik is a real load-balancing edge, but the first beta supports **one Loomarr replica**. Do not
run `docker compose up --scale loomarr=…` in production yet: recurring jobs, playout ownership, and
file-backed state still need the multi-replica investigation recorded in the beta-readiness plan.

## Filler clips

Filler works without extra Compose configuration: Loomarr stores clips under `/data/filler` on
the same persistent `loomarr-data` volume as the rest of `/data`.

To use a different host folder, add an explicit bind mount whose container target is an absolute
path, then set `FILLER_DIR` to that target. Saving `filler.dir` in Settings selects the desired
library but the running generation keeps its current library until Loomarr restarts; Loomarr does
not copy clips between the old and new folders.

On the Tunarr backend, mount the same host folder or named volume into Tunarr at the same absolute
container path so it can scan the clips. Internal playout reads Loomarr's own `/data/filler`
directly and needs no second mount.

## When something's wrong

The wizard tests every connection and each failure links to its fix in
[Troubleshooting](../help/troubleshooting.md). The two most common:

- **Channels appear in the guide but won't play** — usually `SERVER_PUBLIC_URL`. ffmpeg fetches
  the playlist as a separate process, so a wrong value fails only at tune time.
- **The URL is unavailable** — run `docker compose ps`, then check `docker compose logs loomarr
  traefik`. Database and migration failures happen before Loomarr's private listener starts;
  Traefik does not route to an unhealthy backend.
