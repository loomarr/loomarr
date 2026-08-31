# Quickstart

Get a channel playing in about ten minutes.

## Before you start

You need **Emby or Jellyfin** running and reachable. The wizard lets an instance start with only
that connection, but making the first channel from a sentence also requires **TMDB** and an
**LLM**. They are prerequisites for this quickstart, not optional polish.

Have these ready:

- **TMDB API key** — [get one here](https://www.themoviedb.org/settings/api).
- **An LLM** — local Ollama or any OpenAI-compatible provider.

Add these when you're ready — the wizard shows what each one unlocks:

- **Seerr**, or Sonarr and Radarr — to download what you're missing.
- **A filler folder** — for commercials between programmes.
- **Tunarr** — only if you want it to stream your channels instead of Loomarr.

## 1. Start it

Clone an exact version from GitHub Releases, copy `.env.example` to `.env`, and set
`SERVER_PUBLIC_URL`. This example uses `0.1.0-beta.8`; keep your chosen version pinned.

```bash
VERSION=0.1.0-beta.8
git clone --branch "v${VERSION}" --depth 1 https://github.com/loomarr/loomarr.git
cd loomarr
cp .env.example .env                     # set SERVER_PUBLIC_URL to this host's reachable URL
LOOMARR_VERSION="$VERSION" docker compose -f docker/compose.yaml --profile sqlite up -d
```

For Postgres, add `-f docker/compose.postgres.yaml --profile postgres`. Add `--profile ai` to
either database command to run a local Ollama alongside it.

The SQLite command does not export `DATABASE_URL`; Loomarr still defaults to `/data/loomarr.db`.
That omission is what keeps the in-app SQLite-to-Postgres migration able to persist its selection
and restart onto PostgreSQL. An explicit environment value remains authoritative and disables that
in-app switchover.

> Inside Docker, `localhost` means the container. Reach other services by name
> (`http://emby:8096`) or your host's LAN IP.
>
> Set `SERVER_PUBLIC_URL` to the address your media server can reach Loomarr on. Stream URLs are
> built from it, and a wrong value only shows up when a channel fails to play.

## 2. Run the wizard

Open the `SERVER_PUBLIC_URL` you set. Traefik listens on host port 8080 by default and routes to
Loomarr's private port 8080. A fresh install goes straight to setup:

1. **Admin** — create the account that owns this instance.
2. **Playout** — who streams your channels: **Loomarr** (default) or **Tunarr**. This changes
   the rest of the wizard, so it comes early.
3. **Connections** — Loomarr tests each one. Only the media server has to be green.
4. **Library** *(Tunarr only)* — one click to point Tunarr at your media server.
5. **Users** *(optional)* — pick who else can sign in.
6. **First channel**.

You can leave and come back. The wizard reads its position from the server.

## 3. Make a channel

On the **Suggest** page, describe one:

> 90s Saturday morning cartoons for the kids

Loomarr proposes a lineup from your library plus anything missing. Click **Approve** — that's
the step that starts downloads and creates the channel. It's live within a minute and fills in
as titles arrive.

## Upgrading

Migrations only run forward, so back up first. SQLite backups come from `GET /v1/backup`; for
Postgres create a custom archive with `umask 077; pg_dump --format=custom`. Backups hold secrets;
keep them mode `0600`. The [upgrade guide](../install/upgrading.md) has the restore procedure.
