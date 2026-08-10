# Quickstart

Get a channel playing in about ten minutes.

## Before you start

You need one thing running and reachable from Loomarr:

- **Emby or Jellyfin** — your library. This is the only connection the wizard insists on.

Everything else can wait, and the wizard will tell you what each one buys you:

- **TMDB API key** — [get one here](https://www.themoviedb.org/settings/api). Needed before
  Loomarr can suggest anything.
- **An LLM** — local Ollama (the default) or any OpenAI-compatible provider. Also needed for
  suggestions.
- **Seerr**, or Sonarr/Radarr directly — only if you want Loomarr to download what you're missing.
- **A filler folder** — only if you want commercials between programs.
- **Tunarr** — only if you want *it* to stream your channels instead of Loomarr. See step 2.

## 1. Start it

From a clone of the repository:

```bash
docker compose -f docker/compose.yaml --profile sqlite up -d
```

Use `--profile postgres` instead of `sqlite` for Postgres, and add `--profile ai` to either to
run a local Ollama alongside it. Omit `ai` if you're using a hosted provider or an Ollama you
already run.

The first start builds the image locally and takes a while — it downloads a speech model used
for filler. Subsequent starts are fast.

> Inside Docker, `localhost` means the container. Reach other services by name
> (`http://emby:8096`) or your host's LAN IP.
>
> **One setting has no default and matters:** `SERVER_PUBLIC_URL` must be the address your media
> server can reach Loomarr on. Every stream URL is built from it, and nothing warns you at boot
> if it's wrong — channels appear in the guide and then fail to play.

## 2. Run the wizard

Open `http://<host>:8080`. A fresh install goes straight to setup:

1. **Admin** — create the account that owns this instance. Runs once.
2. **Playout** — *who streams your channels?* **Loomarr** (default) or **Tunarr**. This answer
   changes the rest of the wizard, so it comes early. Choosing Tunarr reveals its connection
   form here and adds a Library step later; choosing Loomarr skips both.
3. **Connections** — Loomarr live-tests each one. **Only the media server has to be green**;
   Requester, TMDB and AI show their status but never block you. Red checks link to a fix.
4. **Library** *(Tunarr only)* — one click to point Tunarr at your media server and scan it, so
   channels have real programs to play.
5. **Users** *(optional, skippable)* — pick which media-server accounts can sign in.
6. **First channel** — described below.

You can leave and come back; the wizard reads its position from the server, so a refresh
loses nothing.

## 3. Make a channel

On the **Suggest** page, describe one:

> 90s Saturday morning cartoons for the kids

Loomarr proposes a lineup from your library, plus anything missing. Click **Approve** — that
is the only step that spends anything: it creates the channel and starts any downloads. It's
live within a minute and fills itself in as missing titles arrive.

That's it. Repeat for more channels.

## Upgrading

Migrations are one-way, so the ritual is **back up, then pull.** SQLite backups come from
`GET /v1/backup`; for Postgres, use `pg_dump`. Backups hold secrets — keep them safe.
