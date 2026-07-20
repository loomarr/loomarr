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
  say "starting loomarr on :$PORT (first run compiles, ~20s)"
  # nohup + a detached subshell so the server OUTLIVES this script: interrupting a run
  # must not take the stack down with it, or every ^C costs another cold start.
  # (No setsid — it is Linux-only and this is expected to run on the maintainer's Mac.)
  ( set -a && . "$WORK/env" && set +a && cd "$ROOT" \
      && nohup go run ./cmd/loomarr > "$WORK/app.log" 2>&1 < /dev/null & ) || true
  wait_for "localhost:$PORT/healthz" "loomarr" 180
}

case "${1:-run}" in
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
