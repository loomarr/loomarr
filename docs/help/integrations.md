# Integrations

What to enter for each service. Every setting is an environment variable that's also
editable in **Settings** — an env-set value wins and shows as locked. Red checks link to
[Troubleshooting](troubleshooting) for the fix.

## Media server (Emby / Jellyfin)

Your title library, and optionally the source of user accounts.

- `LIBRARY_FLAVOR` — `emby` or `jellyfin`
- `LIBRARY_URL` — e.g. `http://emby:8096`
- `LIBRARY_TOKEN` — an **admin** API key

Loomarr only reads your library; it never writes to it (except the one-click Live TV
wiring below, which you trigger).

## Tunarr

Plays the channels. Loomarr decides what plays; Tunarr streams it.

- `TUNARR_URL` — e.g. `http://tunarr:8000`
- `TUNARR_API_KEY` — optional
- `TUNARR_TRANSCODE_CONFIG_ID` — optional; leave empty and Loomarr uses your instance's
  `Default`

Two one-click actions (wizard, or **Settings → Connections**), both re-runnable:

- **Wire Tunarr to your library** — so channels have real programs to play.
- **Connect Tunarr to the guide** — so channels appear in your TV guide.

## LLM (Ollama or hosted)

Turns your sentence into a lineup. Must support tool-calling.

- **Local Ollama** (default): `LLM_PROVIDER=ollama`, `LLM_URL=http://ollama:11434`. Don't
  pick a tag by hand — the AI settings picker ranks models that fit your GPU. Prefer a
  Q6_K quant.
- **Hosted** (OpenAI, Gemini, Groq, OpenRouter): `LLM_PROVIDER=openai`, `LLM_URL` = the
  base ending in `/v1`, `LLM_MODEL`, `LLM_API_KEY`.

Switching models takes effect immediately — no restart.

## TMDB

Required. Grounds suggestions and supplies ratings for titles you don't own yet. Set
`TMDB_API_KEY` — [get a key](https://www.themoviedb.org/settings/api).

## Requester — Seerr (or Sonarr/Radarr)

Optional. How Loomarr downloads missing titles. Without it, channels still play what you
already have.

- **Seerr**: `SEERR_URL`, `SEERR_API_KEY` (one integration for movies + shows)
- **Direct**: `SONARR_URL`/`SONARR_API_KEY`, `RADARR_URL`/`RADARR_API_KEY`

Nothing downloads without an admin approving a proposal.

## Sonarr / Radarr webhooks

So Loomarr knows the moment a download lands.

1. Set `WEBHOOK_SECRET`.
2. In each *arr app, add a webhook to
   `http://<loomarr>:8080/hooks/arr?token=<WEBHOOK_SECRET>`.
3. Click **Test** in the app — the wizard's Webhooks step turns green.

## Filler

Optional commercials. See the [Filler guide](filler).
