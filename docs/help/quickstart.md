# Quickstart

Get a channel playing in about ten minutes.

## Before you start

You need **Emby or Jellyfin** running and reachable. That's the only connection the wizard
insists on.

Add these when you're ready — the wizard shows what each one unlocks:

- **TMDB API key** — [get one here](https://www.themoviedb.org/settings/api). Needed to suggest.
- **An LLM** — local Ollama or any OpenAI-compatible provider. Also needed to suggest.
- **Seerr**, or Sonarr and Radarr — to download what you're missing.
- **A filler folder** — for commercials between programmes.
- **Tunarr** — only if you want it to stream your channels instead of Loomarr.

## 1. Start it

```bash
docker compose -f docker/compose.yaml --profile sqlite up -d
```

Use `--profile postgres` for Postgres, and add `--profile ai` to run a local Ollama alongside it.

> Inside Docker, `localhost` means the container. Reach other services by name
> (`http://emby:8096`) or your host's LAN IP.
>
> Set `SERVER_PUBLIC_URL` to the address your media server can reach Loomarr on. Stream URLs are
> built from it, and a wrong value only shows up when a channel fails to play.

## 2. Run the wizard

Open `http://<host>:8080`. A fresh install goes straight to setup:

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
Postgres use `pg_dump`. Backups hold secrets.
