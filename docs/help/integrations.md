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

## Playout

Loomarr streams its own channels by default. Nothing to install — your media server picks it up
as a Live TV tuner.

- `SERVER_PUBLIC_URL` — set this. The address your media server reaches Loomarr on. Stream URLs
  are built from it, and a wrong value only shows up when a channel fails to play.
- `PLAYOUT_QUALITY_TIER` — `efficient` (720p), `balanced` (1080p), or higher.
- `PLAYOUT_MAX_CHANNELS` — optional live-transcode safety cap (`0`/empty uses measured capacity automatically).
- `PLAYOUT_ENCODER` — leave empty. Loomarr measures which encoders work at boot and picks one.

For hardware encoding, pass your GPU through: `PLAYOUT_RENDER_DEVICE=/dev/dri` for Intel and AMD,
or the NVIDIA compose overlay for NVENC.

## Tunarr

Optional — the alternative playout backend, chosen on the wizard's Playout step. Pick it if your
hardware can't transcode, or if you already run Tunarr. Loomarr still decides what plays.

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

OpenRouter can run every filler AI capability with that same key. In Filler settings, choose
**Hosted AI service** for language detection and transcription, set the transcription model
(the default is `openai/whisper-large-v3`), and enable vision with an image-capable model. The
chat, vision, and speech-to-text model ids remain separate because they accept different inputs;
the credential and provider are shared.

Switching models takes effect immediately — no restart.

**What leaves your network.** With local Ollama, nothing. With a hosted provider, Loomarr sends
your intent plus titles and metadata from your library, so the model picks among real options.
TMDB always receives title searches.

Two optional filler features send more and are off by default: vision tagging sends video
keyframes, and hosted transcription sends audio in sub-minute chunks. Hosted language detection
also sends a short audio sample when selected.

## TMDB

Required. Grounds suggestions and supplies ratings for titles you don't own yet. Set
`TMDB_API_KEY` — [get a key](https://www.themoviedb.org/settings/api).

## Notifications

**Settings → Notifications** has one provider list. SMTP uses the same setup as Slack, Discord,
webhook, and every other provider:

1. Click **Add provider**.
2. Choose SMTP, Slack, Discord, or another supported provider.
3. Enter that provider's settings.
4. Select which events it receives.
5. Save.
6. Optionally send a test.

Only fields for the chosen provider appear. Loomarr decides which fields are sensitive, encrypts
their values in the database, and never returns them to the browser. Editing a provider shows only
whether each sensitive field is configured; leave it unchanged to preserve it or explicitly clear
it before saving.

Account invitations and password recovery remain mandatory security messages even though SMTP is
configured through this same provider list; product-event selections do not suppress those account
messages. A successful **Test** means Loomarr queued a provider handoff, not that a remote service or
device displayed it. Each provider row shows its last accepted handoff, a safe failure category, and
queued or failed counts.

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
