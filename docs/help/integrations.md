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

### Provider settings

| Provider | What to enter |
| --- | --- |
| SMTP | Submission host, port, TLS policy, sender address/name, and optional username/password. `None` is only for a trusted local relay. |
| Webhook | HTTPS endpoint plus optional bearer token and HMAC signing secret. Loomarr sends a versioned JSON event and an `X-Loomarr-Event-ID`. |
| Slack | Incoming webhook URL from Slack. |
| Discord | Incoming webhook URL from Discord. |
| ntfy | Server URL and topic; optionally a username and password/token. |
| Gotify | Server URL and application token. |
| Apprise | Apprise API URL and either a stored configuration key or a stateless destination URL; optionally an API token. |
| Pushover | Application token and user/group key; optionally a device. |
| Telegram Bot | Bot token and chat ID; optionally a topic/thread ID. |
| Mattermost | Incoming webhook URL. |
| Matrix | Homeserver URL, room ID, and access token. |
| MQTT | `mqtt://` or `mqtts://` broker URL, base topic, QoS 0/1, retain choice, and optional client ID or username/password. For private CAs or mutual TLS, add the PEM CA and paired client certificate/key. |
| Browser Push | No keys to paste. Choose events, then click **Enable this browser**. The browser permission prompt appears only then. |

### Provider notes

- **Slack, Discord, and Mattermost:** create an incoming webhook in the destination channel, paste
  its URL into Loomarr, save, and send a test. To rotate it, create a replacement webhook, update
  the provider, test it, then revoke the old webhook. Loomarr needs no bot, OAuth, or inbound access.
- **ntfy:** use a hard-to-guess topic with authentication, especially on `ntfy.sh`; public topics can
  be read by anyone who knows their name. For self-hosting, create the user/token in ntfy and enter
  the same credentials here. Routine events use priority 3 and a `tv` tag; degraded/gave-up events
  use priority 4 and a `warning` tag.
- **Gotify:** create an application in Gotify and paste that application's token. Tests should open
  Loomarr when clicked. Rotate or revoke access by replacing or deleting the Gotify application
  token, then update and retest the provider.
- **Apprise:** run Apprise API separately; Loomarr does not install or operate it. A minimal setup is
  an Apprise configuration such as `loomarr.yml` with a service URL under `urls:`, saved in Apprise
  under a key such as `household`; enter the Apprise base URL and that key in Loomarr. Stateless mode
  instead accepts one Apprise destination URL as a write-only value. Apprise and every downstream
  service it targets are inside your trust boundary: they receive the notification content and
  Loomarr confirms only the API handoff, not every fan-out result.
- **Pushover:** create a Pushover application, then enter its application token and your user or
  delivery-group key. Pushover requires an account and may require a one-time client-platform
  purchase; consult Pushover for current terms. Loomarr never uses emergency priority.
- **Telegram:** create a bot with BotFather, add it to the direct chat or group, obtain the numeric
  chat ID, and optionally enter a forum topic/thread ID. The bot must be allowed to post. Rotate the
  token through BotFather, update the provider, and send another test.
- **Matrix:** invite a dedicated bot/user to the room and use that account's access token. Loomarr
  sends ordinary `m.room.message` text; end-to-end encrypted rooms are not supported. Rotate the
  account token, update the provider, and test before revoking the old token.
- **MQTT:** leave Client ID blank to have Loomarr derive a stable value from the provider, or enter
  the identity required by the broker. Loomarr publishes below `<base topic>/<event type>`. Messages
  are not retained by default; enabling retain can replay an old alert as though it were current.
  `mqtts://` uses TLS 1.2 or newer and always verifies the broker certificate and hostname. A PEM CA
  certificate adds trust for a private broker; a client certificate and key must be supplied together
  when the broker requires mutual TLS. Loomarr stores all three as encrypted, write-only fields and
  does not offer a certificate-verification bypass. Home Assistant can subscribe without giving
  Loomarr any Home Assistant credential:

  ```yaml
  automation:
    - alias: Loomarr degraded channel
      triggers:
        - trigger: mqtt
          topic: home/loomarr/channel_degraded
      actions:
        - action: notify.mobile_app_phone
          data:
            title: "{{ trigger.payload_json.subject }}"
            message: "{{ trigger.payload_json.summary }}"
  ```

Browser Push belongs to the signed-in person and to that browser subscription. Members see only
their own Browser Push providers; administrators also see the installation providers they manage.
Loomarr stores the Push endpoint and browser keys encrypted and shows only the device label. A
locked-screen preview says only that a Loomarr notification is available. Deleting the provider
unsubscribes the current browser when possible; an expired subscription is disabled automatically.

The retired `event.webhook_url` setting was an inbound callback from Sonarr/Radarr, not an outbound
notification target, so it is not migrated into this provider list. Outbound Webhook providers are
created here and use the durable notification queue. Existing SMTP settings are imported once into
an SMTP provider with their secrets encrypted; review its event selections and send a test after an
upgrade.

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
