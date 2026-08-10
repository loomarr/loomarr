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

## Playout — who streams your channels

Loomarr streams its own channels by default (`PLAYOUT_BACKEND=internal`). Nothing to install:
your media server picks Loomarr up as a Live TV tuner.

- `SERVER_PUBLIC_URL` — **set this.** The address your media server and clients reach Loomarr on.
  Every stream URL is built from it, and there is no boot-time warning if it's wrong; channels
  appear in the guide and fail at tune time.
- `PLAYOUT_QUALITY_TIER` — `efficient` (720p), `balanced` (1080p), or higher. Bandwidth against
  fidelity.
- `PLAYOUT_MAX_CHANNELS` — how many channels may stream at once (default 4). Raise it only as far
  as your encoder actually sustains.
- `PLAYOUT_ENCODER` — leave empty. Loomarr trial-encodes with every encoder ffmpeg reports and
  picks the best one that genuinely works; setting this overrides a measurement with a guess.

For hardware acceleration, pass your GPU through — `PLAYOUT_RENDER_DEVICE=/dev/dri` for Intel and
AMD, or the NVIDIA compose overlay for NVENC. Without it Loomarr encodes in software, which works
but limits how many channels play at once.

## Tunarr

**Optional.** The alternative playout backend, chosen on the wizard's Playout step. Pick it when
your hardware can't transcode, or when Tunarr already works for you. Loomarr still decides what
plays; Tunarr does the streaming.

- `TUNARR_URL` — e.g. `http://tunarr:8000` (Tunarr has no login; Loomarr talks to it directly)
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

**What leaves your network.** With local Ollama, nothing does. With a hosted provider, Loomarr
sends your intent text plus titles and metadata from your library as grounding candidates —
enough for the model to choose among real options rather than invent them. TMDB always receives
title searches, since that is how picks you don't already own get grounded.

Two optional filler features raise this and are **off by default** for that reason: vision
tagging sends video keyframes to the provider, and hosted transcription sends audio. A fully
local install sends nothing but TMDB lookups.

## TMDB

Required. Grounds suggestions and supplies ratings for titles you don't own yet. Set
`TMDB_API_KEY` — [get a key](https://www.themoviedb.org/settings/api).

## Requester — Seerr (or Sonarr/Radarr)

Optional. How Loomarr downloads missing titles. Without it, channels still play what you
already have.

- **Seerr**: `SEERR_URL`, `SEERR_API_KEY` (one integration for movies + shows)
- **Direct**: `SONARR_URL`/`SONARR_API_KEY`, `RADARR_URL`/`RADARR_API_KEY`

Nothing downloads without an admin approving a proposal.

## How Loomarr knows a download finished

Nothing to configure. Loomarr **polls** — a scheduled library scan, plus a download-queue
poll when you use the direct Sonarr/Radarr requester. A title moves *downloading* →
*available* once it appears in your media-server library.

Settings → Tasks shows each poll's last run and lets you **Run now** if you would rather
not wait.

> Earlier versions asked you to configure an inbound webhook in each *arr app. That was
> retired in favour of polling — there is no endpoint to point Sonarr at, and no secret to
> set. If you added one previously, you can delete it.

## Filler

Optional commercials. See the [Filler guide](filler).
