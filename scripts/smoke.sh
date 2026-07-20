#!/usr/bin/env bash
# The maintainer smoke (§21 Definition of Done, second half).
#
# Stands up a THROWAWAY install — its own database and its own Tunarr container — and
# drives it with Playwright against the operator's REAL media server, TMDB, and Ollama.
# The deliberate opposite of `make check`/`make e2e`, which mock every external service:
# it exists because the per-phase gates have been green while the seams between them were
# unwired (§21 phase 12.5), and because a bug that only appears with a real settings
# service will never show up in a mocked suite.
#
# Requester (Seerr) is deliberately OMITTED: approving would submit real requests to
# Sonarr/Radarr and start real downloads. The approval gate is still exercised end to end;
# nothing leaves the machine.
#
#   make smoke        run against the stack, starting it only if it isn't up
#   make smoke-reset  force a true FIRST RUN (wipes the database, recreates Tunarr)
#   make smoke-down   tear it all down
#
# The stack is left RUNNING between invocations on purpose: re-running the specs should
# take seconds, not the ~90s a cold start costs.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK=/tmp/loomarr-smoke
PORT=8090
TUNARR_PORT=8001
TUNARR_NAME=loomarr-smoke-tunarr
# The 1.3.8 image is amd64-only; Apple Silicon needs the platform pinned (docker/compose.dev.yaml).
TUNARR_IMAGE="chrisbenincasa/tunarr:1.3.8@sha256:88122a21c21e62c3786db786cb2725f2b94da538ea613d08febb67bf3d912e3e"

say()  { printf "\033[1m» %s\033[0m\n" "$1"; }
ok()   { printf "  \033[32m✓\033[0m %s\n" "$1"; }
wait_for() { # url label seconds
  local url=$1 label=$2 limit=${3:-120} waited=0
  while [ "$waited" -lt "$limit" ]; do
    if [ "$(curl -s -o /dev/null -w '%{http_code}' -m 2 "$url" || true)" = "200" ]; then
      ok "$label ready (${waited}s)"; return 0
    fi
    sleep 3; waited=$((waited + 3))
    [ $((waited % 15)) -eq 0 ] && printf "  … still waiting on %s (%ss)\n" "$label" "$waited"
  done
  echo "  ✗ $label did not come up in ${limit}s"; return 1
}

app_up()    { [ "$(curl -s -o /dev/null -w '%{http_code}' -m 2 "localhost:$PORT/healthz" || true)" = "200" ]; }
tunarr_up() { [ "$(curl -s -o /dev/null -w '%{http_code}' -m 2 "localhost:$TUNARR_PORT/api/channels" || true)" = "200" ]; }

stop_app() { lsof -ti:"$PORT" 2>/dev/null | xargs kill -9 2>/dev/null || true; }

down() {
  stop_app
  docker rm -f "$TUNARR_NAME" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}

write_env() {
  mkdir -p "$WORK"
  python3 - "$ROOT" "$WORK" "$PORT" "$TUNARR_PORT" <<'PY'
import pathlib, sys
root, work, port, tport = sys.argv[1:5]
out = []
for line in pathlib.Path(root, ".env").read_text().splitlines():
    if "=" not in line or line.strip().startswith("#"):
        continue
    k, v = line.split("=", 1)
    k = k.strip()
    # Omitted on purpose: a requester would turn an approval into a real download.
    if k in ("SEERR_URL", "SEERR_API_KEY", "TUNARR_TRANSCODE_CONFIG_ID"):
        continue
    if k == "DATABASE_URL": v = f"sqlite://{work}/smoke.db"
    if k == "TUNARR_URL":   v = f"http://localhost:{tport}"
    if k == "LISTEN_ADDR":  v = f":{port}"
    out.append(f"{k}={v}")
pathlib.Path(work, "env").write_text("\n".join(out) + "\n")
PY
}

start_app() {
  # BUILD, don't `go run`. `go run` does not exec the server — it compiles, then
  # supervises the real binary as a subprocess and stays alive itself. Backgrounded from
  # here it remained a CHILD of this script and held its stdout open, so `make smoke`
  # never returned even though Playwright had finished: piping the output (`make smoke |
  # tail`) hung forever with an empty buffer, which reads exactly like a stuck test run.
  # Building first removes the supervisor layer entirely — and it also means `stop_app`'s
  # kill-by-port actually kills the thing, instead of orphaning a `go run` parent.
  say "building the smoke binary"
  ( cd "$ROOT" && go build -o "$WORK/loomarr" ./cmd/loomarr )
  ok "built"

  say "starting loomarr on :$PORT"
  # nohup + a detached subshell so the server OUTLIVES this script: interrupting a run
  # must not take the stack down with it, or every ^C costs another cold start.
  # (No setsid — it is Linux-only and this is expected to run on the maintainer's Mac.)
  ( set -a && . "$WORK/env" && set +a \
      && nohup "$WORK/loomarr" > "$WORK/app.log" 2>&1 < /dev/null & ) || true
  wait_for "localhost:$PORT/healthz" "loomarr" 120
}

# ---------------------------------------------------------------------------------
# The LIVE TV profile — a DISPOSABLE media server, on purpose.
#
# POST /v1/setup/livetv-connect is the one wiring action that writes to the MEDIA SERVER
# (it registers Tunarr as an M3U tuner + XMLTV guide source). Running it against the
# maintainer's real Emby would leave a tuner pointing at a Tunarr that gets torn down,
# and there is no product code path to undo it — internal/library/livetv.go only adds.
# So this profile stands up its own Jellyfin, wires THAT, and deletes the whole server
# afterwards. Reverting is free because nothing survives.
#
# It runs as a SEPARATE loomarr instance rather than repointing the main smoke stack:
# the main stack's LIBRARY_* must keep pointing at the real Emby, because that is where
# the library content the later steps ground against actually lives. This instance's
# library is empty, which is fine — the Live TV wiring never reads content.
JELLY_PORT=8097
JELLY_NAME=loomarr-smoke-jellyfin
JELLY_IMAGE="jellyfin/jellyfin:10.10.3"
LT_PORT=8091
LT_WORK=/tmp/loomarr-smoke-livetv
JELLY_USER=smoke
JELLY_PASS=smoke-password-123

jelly() { curl -s -m 10 -H 'Content-Type: application/json' "$@"; }

# Jellyfin boots into a first-run wizard and refuses API calls until it is completed.
# These are the wizard's own endpoints, in the order its UI calls them.
jellyfin_first_run() {
  say "completing Jellyfin's first-run wizard"
  jelly -X POST "localhost:$JELLY_PORT/Startup/Configuration" \
    -d '{"UICulture":"en-US","MetadataCountryCode":"US","PreferredMetadataLanguage":"en"}' >/dev/null
  jelly "localhost:$JELLY_PORT/Startup/User" >/dev/null
  jelly -X POST "localhost:$JELLY_PORT/Startup/User" \
    -d "{\"Name\":\"$JELLY_USER\",\"Password\":\"$JELLY_PASS\"}" >/dev/null
  jelly -X POST "localhost:$JELLY_PORT/Startup/RemoteAccess" \
    -d '{"EnableRemoteAccess":true,"EnableAutomaticPortMapping":false}' >/dev/null
  jelly -X POST "localhost:$JELLY_PORT/Startup/Complete" >/dev/null
  ok "wizard completed"
}

# The access token doubles as the admin API key for header auth (§6 flavor login).
jellyfin_token() {
  jelly -X POST "localhost:$JELLY_PORT/Users/AuthenticateByName" \
    -H 'Authorization: MediaBrowser Client="loomarr-smoke", Device="smoke", DeviceId="smoke", Version="1"' \
    -d "{\"Username\":\"$JELLY_USER\",\"Pw\":\"$JELLY_PASS\"}" \
    | sed -n 's/.*"AccessToken":"\([^"]*\)".*/\1/p'
}

livetv_down() {
  lsof -ti:"$LT_PORT" 2>/dev/null | xargs kill -9 2>/dev/null || true
  docker rm -f "$JELLY_NAME" >/dev/null 2>&1 || true
  rm -rf "$LT_WORK"
}

livetv_smoke() {
  # Always from scratch: a half-configured media server is worse than none, and the
  # whole point of this profile is that nothing persists.
  say "tearing down any previous Live TV profile"
  livetv_down
  mkdir -p "$LT_WORK/jellyfin/config" "$LT_WORK/jellyfin/cache"

  # Tunarr must be up — it is what gets registered as the tuner.
  tunarr_up || { echo "start the main smoke stack first (make smoke)"; return 1; }

  say "starting a throwaway Jellyfin on :$JELLY_PORT"
  docker run -d --name "$JELLY_NAME" \
    -p "$JELLY_PORT":8096 \
    -v "$LT_WORK/jellyfin/config":/config -v "$LT_WORK/jellyfin/cache":/cache \
    "$JELLY_IMAGE" >/dev/null
  wait_for "localhost:$JELLY_PORT/System/Info/Public" "jellyfin" 180

  jellyfin_first_run
  local token; token=$(jellyfin_token)
  [ -n "$token" ] || { echo "  ✗ could not authenticate against the throwaway Jellyfin"; return 1; }
  ok "authenticated"

  # TUNARR_URL must be reachable from BOTH sides, so it is the host's LAN address rather
  # than localhost: §6 derives the M3U/XMLTV URLs from this one setting, and those URLs
  # are handed to Jellyfin — which is in a container, where "localhost" is itself. There
  # is no separate public-URL knob to reach for (§15), and inventing one to paper over a
  # test-environment detail would be the wrong fix.
  local host_ip; host_ip=$(ipconfig getifaddr en0 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}')
  [ -n "$host_ip" ] || { echo "  ✗ could not determine a host IP the container can reach"; return 1; }
  ok "host address for Tunarr URLs: $host_ip"

  say "writing the Live TV env (own DB, own Jellyfin, own port)"
  grep -v -E '^(LIBRARY_FLAVOR|LIBRARY_URL|LIBRARY_TOKEN|DATABASE_URL|LISTEN_ADDR|TUNARR_URL|SEERR_URL|SEERR_API_KEY)=' \
    "$WORK/env" > "$LT_WORK/env"
  {
    echo "LIBRARY_FLAVOR=jellyfin"
    echo "LIBRARY_URL=http://localhost:$JELLY_PORT"
    echo "LIBRARY_TOKEN=$token"
    echo "DATABASE_URL=sqlite://$LT_WORK/livetv.db"
    echo "LISTEN_ADDR=:$LT_PORT"
    echo "TUNARR_URL=http://$host_ip:$TUNARR_PORT"
  } >> "$LT_WORK/env"

  say "starting loomarr on :$LT_PORT"
  ( cd "$ROOT" && go build -o "$LT_WORK/loomarr" ./cmd/loomarr )
  ( set -a && . "$LT_WORK/env" && set +a \
      && nohup "$LT_WORK/loomarr" > "$LT_WORK/app.log" 2>&1 < /dev/null & ) || true
  wait_for "localhost:$LT_PORT/healthz" "loomarr" 120

  say "driving the Live TV wiring with Playwright"
  local rc=0
  ( cd "$ROOT/web/apps/web" \
      && SMOKE_BASE_URL="http://127.0.0.1:$LT_PORT" \
         SMOKE_JELLYFIN_URL="http://127.0.0.1:$JELLY_PORT" \
         SMOKE_JELLYFIN_TOKEN="$token" \
         npx playwright test --config=playwright.smoke.config.ts livetv ) || rc=$?

  # The revert IS the teardown: the media server we wrote to ceases to exist.
  say "destroying the throwaway Jellyfin (this is the revert)"
  livetv_down
  ok "gone — your real media server was never touched"
  return $rc
}

case "${1:-run}" in
  livetv) livetv_smoke; exit $? ;;
  down)
    say "tearing down the smoke stack"; down
    echo "done — your dev stack (tunarr-dev, data/loomarr.db) was never touched."
    exit 0 ;;
  reset)
    say "forcing a FIRST RUN (wiping the smoke database + Tunarr)"; down ;;
  run) shift || true ;;
esac
[ "${1:-}" = "reset" ] && shift || true

[ -f "$ROOT/.env" ] || { echo "no .env — the smoke needs your real LIBRARY_URL/TMDB_API_KEY/LLM_URL"; exit 1; }

if tunarr_up; then
  ok "throwaway Tunarr already up on :$TUNARR_PORT"
else
  say "starting a throwaway Tunarr on :$TUNARR_PORT (image is emulated, ~30s)"
  docker rm -f "$TUNARR_NAME" >/dev/null 2>&1 || true
  # A bind mount under $WORK, NOT a named volume: `docker volume rm` races the container
  # teardown and fails silently, which left a half-deleted Meilisearch database behind and
  # made the "fresh" instance refuse to boot. This ties Tunarr's state to the one
  # directory the reset already wipes — a single lifecycle instead of two.
  mkdir -p "$WORK/tunarr"
  docker run -d --name "$TUNARR_NAME" --platform linux/amd64 \
    -p "$TUNARR_PORT":8000 -v "$WORK/tunarr":/config "$TUNARR_IMAGE" >/dev/null
  wait_for "localhost:$TUNARR_PORT/api/channels" "tunarr" 180
fi

[ -f "$WORK/env" ] || { say "writing the smoke env (own DB, own Tunarr, NO requester)"; write_env; ok "env written"; }

if app_up; then
  ok "loomarr already up on :$PORT"
else
  start_app
fi

say "driving it with Playwright"
cd "$ROOT/web/apps/web"
SMOKE_BASE_URL="http://127.0.0.1:$PORT" npx playwright test --config=playwright.smoke.config.ts "$@"
