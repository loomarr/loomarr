# Dev Tunarr setup

A persistent local Tunarr for developing the Programmer adapter (§9/§10) and the filler
pipeline against, without a throwaway spin-up each time.

> **You only need this when working on the Tunarr backend.** Since §9.1, `playout.backend`
> defaults to `internal` — Loomarr streams its own channels — so most development needs no
> Tunarr at all. `make dev` brings up external dependencies, not the app; the app itself runs
> from [`docs/dev/dev-loop.md`](../docs/dev/dev-loop.md).

## Start / stop

```bash
make dev                                             # portable (Mac or Linux, CPU transcode)
make dev-gpu                                          # Linux + NVIDIA (adds compose.dev.gpu.yaml)
docker compose -f docker/compose.dev.yaml logs -f tunarr
docker compose -f docker/compose.dev.yaml down       # stop (keeps the volume)
```

- Image pinned to **1.3.8** (`@sha256:88122a21…`) — matches `api/vendor/tunarr-openapi.json`.
  If you bump it, re-vendor the spec and update `docs/engineering/phase-0-findings.md`.
- Config persists in the named volume `tunarr-dev-config`. `restart: unless-stopped`.
- UI/API at http://localhost:8000 · version probe: `curl -s localhost:8000/api/version`.

### Mac (Apple Silicon) vs Linux

The base `compose.dev.yaml` is host-agnostic. The 1.3.8 image is **amd64-only**, so on Apple
Silicon it runs under emulation (`platform: linux/amd64` is pinned so the manifest resolves).
That's fine for developing the adapter/API against; **transcoding under emulation is slow**, so
for real playback smoke use the Linux GPU host. NVIDIA transcode is opt-in via `make dev-gpu`
(the `compose.dev.gpu.yaml` overlay) — never use it on a Mac (no GPU passthrough → `up` fails).

## Wire the Emby media source (§6: Tunarr streams the same library loomarr resolves against)

Tunarr authenticates as an **Emby user** (username + password → its own streaming token) via
`POST /api/emby/login` — this is a *different* auth relationship than loomarr's admin API key
(`X-Emby-Token`). Use a dedicated Emby account (e.g. a `tunarr` service user) where possible.

Credentials and the reachable media-server URL come from `.phase0.env` (git-ignored):
`EMBY_USER`, `EMBY_PW`, and `EMBY_URL` (for example, `http://emby-host:8096`).

**Two steps** — `login` only validates + returns a token; you then *create* the source with it:

```bash
set -a; . ./.phase0.env; set +a
: "${EMBY_URL:?set EMBY_URL to the media-server URL reachable from Tunarr}"

# 1. validate creds -> {accessToken, userId}
LOGIN=$(curl -s -X POST http://localhost:8000/api/emby/login \
  -H 'Content-Type: application/json' \
  -d "{\"url\":\"$EMBY_URL\",\"username\":\"$EMBY_USER\",\"password\":\"$EMBY_PW\"}")
TOKEN=$(echo "$LOGIN" | node -pe 'JSON.parse(require("fs").readFileSync(0)).accessToken')
USERID=$(echo "$LOGIN" | node -pe 'JSON.parse(require("fs").readFileSync(0)).userId')

# 2. create the media source (type "emby", pathReplacements required even if empty)
curl -s -X POST http://localhost:8000/api/media-sources -H 'Content-Type: application/json' \
  -d "{\"type\":\"emby\",\"name\":\"Fictional Emby\",\"uri\":\"$EMBY_URL\",\
\"accessToken\":\"$TOKEN\",\"userId\":\"$USERID\",\"username\":\"$EMBY_USER\",\"pathReplacements\":[]}"

# verify: should be {"healthy":true}
MSID=$(curl -s http://localhost:8000/api/media-sources | node -pe 'JSON.parse(require("fs").readFileSync(0))[0].id')
curl -s "http://localhost:8000/api/media-sources/$MSID/status"
# list libraries (fields: uuid, title, externalId, childType):
#   GET /api/emby/$MSID/user_libraries   -> Movies (childType:movie), "TV shows" (childType:show), …
```

**Note (Phase-10 relevant):** `POST /api/emby/login` does NOT persist a source — it only
returns `{accessToken,userId}`. Creating the source is a separate `POST /api/media-sources`
carrying that token. The Programmer adapter must do both.

`EMBY_URL` is explicit because the address must be reachable from the Tunarr container and is
installation-specific. Docker DNS names work for media servers on the same Compose network;
otherwise use a hostname or address routed from Docker on the host.

## Relation to the app compose (Phase 1)

This is dev *dependencies* only. Phase 1 adds the loomarr app + its `sqlite`/`postgres`/`ai`/
`filler` profiles (design §16); `make dev` will bring up the full stack. Keep this file focused
on external services we develop against.
