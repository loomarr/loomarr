# Quickstart

Get a channel playing in about ten minutes.

## Before you start

You'll need these running and reachable from Loomarr:

- **Emby or Jellyfin** — your library
- **Tunarr** — plays the channels
- **TMDB API key** — [get one here](https://www.themoviedb.org/settings/api)
- **An LLM** — local Ollama (default) or a hosted provider

Optional, add later: Seerr (to download missing titles), a filler folder (for commercials).

## 1. Start it

Copy `.env.example` to `.env`, fill in the `(req)` values, then:

```bash
docker compose --profile sqlite --profile ai up -d
```

Use `--profile postgres` instead of `sqlite` for Postgres.

> Inside Docker, `localhost` means the container. Reach other services by name
> (`http://emby:8096`) or your host's LAN IP.

## 2. Run the wizard

Open `http://<host>:8080`. A fresh install goes straight to setup:

1. **Create your admin** — username + password. This account owns the instance.
2. **Connect services** — Loomarr tests each one. Media server and Tunarr must be green;
   the rest can wait. Red checks link to a fix.
3. **Pick a model** — Loomarr ranks the Ollama models that fit your GPU. Pick one.
4. **Wire Tunarr** — two one-click steps: point Tunarr at your library, and add it to your
   TV guide. Both are safe to re-run.
5. **Import users** (optional) — pick who else can sign in.

## 3. Make a channel

On the **Suggest** page, describe one:

> 90s Saturday morning cartoons for the kids

Loomarr proposes a lineup from your library plus anything missing. Click **Approve** — it
creates the channel and takes you to it. It's live in Tunarr within a minute, and fills
itself in as missing titles arrive.

That's it. Repeat for more channels.

## Upgrading

Migrations are one-way, so: **back up, then pull.** SQLite backups come from
`GET /v1/backup`; for Postgres, use `pg_dump`. Backups hold secrets — keep them safe.
