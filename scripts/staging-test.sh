#!/usr/bin/env bash
# Contract for scripts/staging.sh, against a fake curl: no network, no cloud account.

set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd -P)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/bin"

# The fake answers GET with $WORK/instances.json, and logs each method, path and body.
cat >"$WORK/bin/curl" <<'FAKE'
#!/usr/bin/env bash
method=GET url='' data=''
while [ "$#" -gt 0 ]; do
	case "$1" in
	-X) method="$2"; shift ;;
	--data) data="${2#@}"; shift ;;
	-K|-H|--max-time|--retry) shift ;;
	http*) url="$1" ;;
	esac
	shift
done
cat >"$FAKE_DIR/auth.last"
printf '%s %s\n' "$method" "${url#https://api.example.test/v2}" >>"$FAKE_DIR/requests"
if [ -n "$data" ]; then cp "$data" "$FAKE_DIR/body.json"; fi
if [ -e "$FAKE_DIR/api-down" ]; then
	printf '{"error":"Invalid API token.","status":401}\n'
	exit 22
fi
case "$method" in
GET) cat "$FAKE_DIR/instances.json" ;;
POST) printf '{"instance":{"id":"new-server"}}\n' ;;
esac
FAKE
chmod +x "$WORK/bin/curl"

export PATH="$WORK/bin:$PATH" FAKE_DIR="$WORK" VULTR_API_URL=https://api.example.test/v2
export VULTR_API_KEY=test-key TS_AUTHKEY=test-tailnet-key
COMMIT=0123456789abcdef0123456789abcdef01234567
IMAGE=ghcr.io/loomarr/loomarr-staging:$COMMIT
NOW="$(date +%s)"
PAST=$((NOW - 60))
FUTURE=$((NOW + 3600))

fail() {
	printf 'staging-test: %s\n' "$1" >&2
	exit 1
}

reset() {
	: >"$WORK/requests"
	rm -f "$WORK/body.json"
	printf '%s\n' "$1" >"$WORK/instances.json"
}

deleted() {
	grep '^DELETE ' "$WORK/requests" | sed 's/^DELETE .instances.//' | sort | tr '\n' ' ' || true
}

expect_refused() {
	local why="$1"
	shift
	if env "$@" "$ROOT/scripts/staging.sh" up >/dev/null 2>&1; then
		fail "up accepted $why"
	fi
	if grep -q '^POST ' "$WORK/requests"; then
		fail "up created a server despite $why"
	fi
}

# A server of ours, an unrelated server carrying only one of our two marks, and one with neither.
ours() { printf '{"id":"%s","label":"loomarr-staging-%s","tags":["loomarr-staging","expires-%s"]}' "$1" "$1" "$2"; }
MAIL='{"id":"mail","label":"mail-server","tags":[]}'
TAG_ONLY='{"id":"tag-only","label":"other","tags":["loomarr-staging","expires-1"]}'
LABEL_ONLY='{"id":"label-only","label":"loomarr-staging-x","tags":["expires-1"]}'

# Sweep destroys only what is ours and expired.
reset "{\"instances\":[$MAIL,$TAG_ONLY,$LABEL_ONLY,$(ours expired "$PAST"),$(ours live "$FUTURE")]}"
"$ROOT/scripts/staging.sh" sweep >/dev/null
[ "$(deleted)" = "expired " ] || fail "sweep deleted: $(deleted)"

# An unreadable expiry resolves toward destruction, so a mistake cannot bill forever.
reset '{"instances":[{"id":"no-expiry","label":"loomarr-staging-y","tags":["loomarr-staging"]}]}'
"$ROOT/scripts/staging.sh" sweep >/dev/null
[ "$(deleted)" = "no-expiry " ] || fail "sweep kept a server with no expiry: $(deleted)"

# The demo site survives the sweep only by the exact tag, even beside a stale expiry.
KEPT='{"id":"demo","label":"loomarr-staging-demo","tags":["loomarr-staging","keep","expires-1"]}'
NEAR_KEEP='{"id":"near","label":"loomarr-staging-near","tags":["loomarr-staging","keeper"]}'
reset "{\"instances\":[$KEPT,$NEAR_KEEP]}"
"$ROOT/scripts/staging.sh" sweep >/dev/null
[ "$(deleted)" = "near " ] || fail "sweep with a kept server deleted: $(deleted)"

# Down destroys every server of ours, and nothing else.
reset "{\"instances\":[$MAIL,$TAG_ONLY,$LABEL_ONLY,$KEPT,$(ours a "$FUTURE"),$(ours b "$PAST")]}"
"$ROOT/scripts/staging.sh" down >/dev/null
[ "$(deleted)" = "a b demo " ] || fail "down deleted: $(deleted)"

# Keep is the demo site: tagged to stay, with no expiry at all.
reset '{"instances":[]}'
VERSION=1.0.0 STAGING_HOURS=keep "$ROOT/scripts/staging.sh" up >/dev/null
jq -e '(.tags | index("keep")) and ([.tags[] | select(startswith("expires-"))] | length == 0)' \
	"$WORK/body.json" >/dev/null || fail 'keep did not make a kept server'

# Up from a commit creates one tagged server with a bounded lifetime.
reset '{"instances":[]}'
STAGING_IMAGE="$IMAGE" STAGING_COMMIT="$COMMIT" STAGING_HOURS=3 "$ROOT/scripts/staging.sh" up >/dev/null
[ -f "$WORK/body.json" ] || fail 'up sent no create request'
jq -e '.plan == "vc2-4c-8gb" and .backups == "disabled"' "$WORK/body.json" >/dev/null || fail 'up used the wrong plan'
jq -e '.label | startswith("loomarr-staging-")' "$WORK/body.json" >/dev/null || fail 'up used the wrong label'
jq -e '.tags | index("loomarr-staging")' "$WORK/body.json" >/dev/null || fail 'up did not tag the server'
expires="$(jq -r '.tags[] | select(startswith("expires-")) | ltrimstr("expires-")' "$WORK/body.json")"
if [ "$expires" -lt $((NOW + 3 * 3600)) ] || [ "$expires" -gt $((NOW + 3 * 3600 + 60)) ]; then
	fail "up set expiry $expires for 3 hours from $NOW"
fi
boot="$(jq -r .user_data "$WORK/body.json" | base64 -d)"
[[ "$boot" == *"$COMMIT"* ]] || fail 'the boot script does not name the commit'
[[ "$boot" == *"$IMAGE"* ]] || fail 'the boot script does not name the image'
if grep -q test-key "$WORK/body.json"; then fail 'the cloud API key reached the server'; fi
grep -q 'test-key' "$WORK/auth.last" || fail 'the key did not go through curl config'

# Up from a release needs no commit image.
reset '{"instances":[]}'
VERSION=0.2.0-beta.9-rc.11 "$ROOT/scripts/staging.sh" up >/dev/null
[[ "$(jq -r .user_data "$WORK/body.json" | base64 -d)" == *0.2.0-beta.9-rc.11* ]] || fail 'the boot script does not name the release'
[[ "$(jq -r .user_data "$WORK/body.json" | base64 -d)" == *STAGING_DATABASE=sqlite* ]] || fail 'the boot script does not default to SQLite'

# A private server never receives the tunnel token, which only a public one has any use for.
reset '{"instances":[]}'
VERSION=1.0.0 CF_TUNNEL_TOKEN=test-tunnel-token DEMO_ADMIN_PASSWORD=test-admin-password "$ROOT/scripts/staging.sh" up >/dev/null
if jq -r .user_data "$WORK/body.json" | base64 -d | grep -qE 'test-tunnel-token|test-admin-password'; then
	fail 'a private server received the tunnel token or admin password'
fi
reset '{"instances":[]}'
VERSION=1.0.0 STAGING_PUBLIC_URL=https://demo.example.test CF_TUNNEL_TOKEN=test-tunnel-token DEMO_ADMIN_PASSWORD=test-admin-password \
	"$ROOT/scripts/staging.sh" up >/dev/null
jq -r .user_data "$WORK/body.json" | base64 -d | grep -q test-tunnel-token || fail 'a public server got no tunnel token'
jq -r .user_data "$WORK/body.json" | base64 -d | grep -q test-admin-password || fail 'a public server got no admin password'

# The database choice reaches the server.
reset '{"instances":[]}'
VERSION=1.0.0 STAGING_DATABASE=postgres "$ROOT/scripts/staging.sh" up >/dev/null
[[ "$(jq -r .user_data "$WORK/body.json" | base64 -d)" == *STAGING_DATABASE=postgres* ]] || fail 'the boot script ignored the database choice'

# Up refuses anything that would run unbounded, twice, or from an unclear source.
reset "{\"instances\":[$(ours running "$FUTURE")]}"
expect_refused 'a second server' VERSION=1.0.0
reset '{"instances":[]}'
expect_refused 'a zero lifetime' VERSION=1.0.0 STAGING_HOURS=0
expect_refused 'a lifetime over the cap' VERSION=1.0.0 STAGING_HOURS=25
expect_refused 'a non-numeric lifetime' VERSION=1.0.0 STAGING_HOURS=3h
expect_refused 'no source' VERSION=
expect_refused 'two sources' VERSION=1.0.0 STAGING_IMAGE="$IMAGE" STAGING_COMMIT="$COMMIT"
expect_refused 'an image with no commit' STAGING_IMAGE="$IMAGE"
expect_refused 'a short commit' STAGING_IMAGE="$IMAGE" STAGING_COMMIT=0123456
expect_refused 'a version with shell syntax' 'VERSION=1.0.0;reboot'
expect_refused 'no tailnet key' VERSION=1.0.0 TS_AUTHKEY=
expect_refused 'an unknown database' VERSION=1.0.0 STAGING_DATABASE=mysql
ADMIN_OK=DEMO_ADMIN_PASSWORD=correct-horse-battery
expect_refused 'a public site with no tunnel' VERSION=1.0.0 STAGING_PUBLIC_URL=https://demo.example.test CF_TUNNEL_TOKEN= "$ADMIN_OK"
expect_refused 'a public address that is not https' VERSION=1.0.0 STAGING_PUBLIC_URL=http://demo.example.test CF_TUNNEL_TOKEN=tunnel "$ADMIN_OK"
expect_refused 'a public site with no admin password' VERSION=1.0.0 STAGING_PUBLIC_URL=https://demo.example.test CF_TUNNEL_TOKEN=tunnel DEMO_ADMIN_PASSWORD=
expect_refused 'a short admin password' VERSION=1.0.0 STAGING_PUBLIC_URL=https://demo.example.test CF_TUNNEL_TOKEN=tunnel DEMO_ADMIN_PASSWORD=short
expect_refused 'an admin password that needs escaping' VERSION=1.0.0 STAGING_PUBLIC_URL=https://demo.example.test CF_TUNNEL_TOKEN=tunnel 'DEMO_ADMIN_PASSWORD=correct"horse"battery'

# A failed boot powers the server off, and the sweep clears it even if it was kept.
reset '{"instances":[{"id":"dead","label":"loomarr-staging-z","tags":["loomarr-staging","keep"],"power_status":"stopped"}]}'
"$ROOT/scripts/staging.sh" sweep >/dev/null
[ "$(deleted)" = "dead " ] || fail "sweep kept a server whose boot failed: $(deleted)"

# A server with no tags or label is simply not ours, and must not break the listing.
reset '{"instances":[{"id":"bare"},{"id":"null-label","label":null,"tags":null}]}'
"$ROOT/scripts/staging.sh" sweep >/dev/null
[ -z "$(deleted)" ] || fail "sweep deleted a bare server: $(deleted)"

# An unreachable API or a rotated key must fail loudly, never look like an empty account.
reset '{"instances":[]}'
touch "$WORK/api-down"
if "$ROOT/scripts/staging.sh" sweep >/dev/null 2>&1; then fail 'sweep went green with the API down'; fi
if "$ROOT/scripts/staging.sh" down >/dev/null 2>&1; then fail 'down went green with the API down'; fi
expect_refused 'an unreadable account' VERSION=1.0.0
# The provider's reason is the only clue to a bad key, so it has to reach the run's log.
reason="$("$ROOT/scripts/staging.sh" sweep 2>&1 >/dev/null || true)"
[[ "$reason" == *'Invalid API token.'* ]] || fail "the provider's reason was lost: $reason"
rm "$WORK/api-down"
reset 'not json'
if "$ROOT/scripts/staging.sh" sweep >/dev/null 2>&1; then fail 'sweep went green on a bad response'; fi

# Without a key there is nothing the sweeper could have made, so it must not fail hourly.
reset '{"instances":[]}'
VULTR_API_KEY='' "$ROOT/scripts/staging.sh" sweep >/dev/null
[ ! -s "$WORK/requests" ] || fail 'sweep called the API without a key'

# The template runs only on the server, so nothing else in CI would notice it stop parsing.
bash -n "$ROOT/docker/staging-boot.sh.tmpl" || fail 'the boot template does not parse'

printf 'staging-test: ok\n'
