#!/usr/bin/env bash
# On-demand staging: one cloud server running the demo site, destroyed when it expires.
#
#   scripts/staging.sh up          create it (VERSION=<release>, or STAGING_IMAGE + STAGING_COMMIT)
#   scripts/staging.sh down        destroy the test server now
#   scripts/staging.sh down-demo   destroy the long-running demo site now
#   scripts/staging.sh sweep       destroy anything past its expiry
#   scripts/staging.sh status      show each server's state
#
# STAGING_HOURS=keep makes the long-running demo site: the sweep leaves it alone, and only
# `down-demo` destroys it. It is opt-in by name, so an unreadable expiry still means destroy. One
# demo site and one test server can run side by side, so testing never takes the demo site down.
#
# STAGING_GPU=nvidia puts a test server on an NVIDIA A16, about eight times the price of the
# plain server, so it must expire.
#
# The provider bills a stopped server, so destroying is the only way to stop paying. The cloud
# account also holds unrelated servers and its key can reach them, so this acts only on a server
# carrying both our tag and our label prefix.

set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd -P)"
API="${VULTR_API_URL:-https://api.vultr.com/v2}"
TAG=loomarr-staging
LABEL_PREFIX=loomarr-staging-
PLAN=vc2-4c-8gb
GPU_PLAN=vcg-a16-6c-60g-16vram # one whole A16: NVENC, and Vultr installs its driver at boot
REGION=ewr
# Newark's A16s reported running on 2026-11-07 but never reached the network, and the console
# could not connect; a Chicago A16's console worked at once.
GPU_REGION=ord
OS_ID=2284 # Ubuntu 24.04 LTS x64
MAX_HOURS=24
REPLACE_TIMEOUT_SECONDS="${STAGING_REPLACE_TIMEOUT:-2700}" # first boot to seeded is about 25 minutes
REPLACE_SETTLE_SECONDS="${STAGING_REPLACE_SETTLE:-30}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

die() {
	printf 'staging: %s\n' "$1" >&2
	exit 1
}

# The key goes through curl's config on stdin, so it never appears in a process list.
api() {
	local method="$1" path="$2" body="${3:-}"
	local args=(--fail-with-body -sS --max-time 30 -X "$method" -K -)
	if [ -n "$body" ]; then
		args+=(-H 'Content-Type: application/json' --data "@$body")
	else
		# Never retry a create: a retry after a lost response would make a second billed server.
		args+=(--retry 3)
	fi
	printf 'header = "Authorization: Bearer %s"\n' "$VULTR_API_KEY" | curl "${args[@]}" "$API$path"
}

# ours prints one "id expiry power" line per staging server. The expiry is "keep" for the demo
# site, and 0 when it is unreadable. Callers assign it first and check the status: a failure read
# through a pipe or process substitution looks like an empty account, and the sweep would go green
# while a server keeps billing.
ours() {
	local response
	# Only a refusal is printed: a good response lists every server in the account, and logs are public.
	if ! response="$(api GET '/instances?per_page=500')"; then
		printf 'staging: the provider refused the listing: %s\n' "${response:0:300}" >&2
		return 1
	fi
	jq -r --arg tag "$TAG" --arg prefix "$LABEL_PREFIX" '
		.instances[]
		| ((.tags // []) | map(tostring)) as $tags
		| select(($tags | index($tag)) and ((.label // "") | startswith($prefix)))
		| [.id, (if ($tags | index("keep")) then "keep"
			else ([$tags[] | select(test("^expires-[0-9]+$")) | ltrimstr("expires-")][0] // "0") end),
		   (.power_status // "unknown")]
		| @tsv' <<<"$response"
}

destroy() {
	api DELETE "/instances/$1" >/dev/null
	printf 'staging: destroyed %s\n' "$1"
}

up() {
	local hours="${STAGING_HOURS:-4}" version="${VERSION:-}" image="${STAGING_IMAGE:-}" commit="${STAGING_COMMIT:-}"
	local database="${STAGING_DATABASE:-sqlite}" public_url="${STAGING_PUBLIC_URL:-}" tunnel_token='' admin_password=''
	local gpu="${STAGING_GPU:-none}" plan="$PLAN" region="$REGION"
	[ -n "${VULTR_API_KEY:-}" ] || die 'VULTR_API_KEY is not set'
	[ -n "${TS_AUTHKEY:-}" ] || die 'TS_AUTHKEY is not set'
	[[ "$database" =~ ^(sqlite|postgres)$ ]] || die 'STAGING_DATABASE must be sqlite or postgres'
	case "$gpu" in
	none) ;;
	nvidia)
		[ "$hours" != keep ] || die 'a GPU server costs about 344 dollars a month, so it must expire; set STAGING_HOURS'
		plan="$GPU_PLAN" region="$GPU_REGION"
		;;
	*) die 'STAGING_GPU must be none or nvidia' ;;
	esac
	if [ -n "$public_url" ]; then
		# Every server with the tunnel token answers the public address, so a second one would
		# split the demo site's visitors between two servers.
		[ "$hours" = keep ] || die 'only the demo site goes public; set STAGING_HOURS=keep'
		[[ "$public_url" =~ ^https://[a-z0-9.-]+$ ]] || die "invalid STAGING_PUBLIC_URL: $public_url"
		[ -n "${CF_TUNNEL_TOKEN:-}" ] || die 'a public site needs CF_TUNNEL_TOKEN'
		# Checked here too, so a bad secret fails the run instead of a server that then powers off.
		[[ "${DEMO_ADMIN_PASSWORD:-}" =~ ^[A-Za-z0-9._~-]{16,}$ ]] ||
			die 'a public site needs DEMO_ADMIN_PASSWORD: 16 or more letters, digits or ._~-'
		# Only a public server gets these: a staging run has no use for them.
		tunnel_token="$CF_TUNNEL_TOKEN"
		admin_password="$DEMO_ADMIN_PASSWORD"
	fi
	if [ "$hours" != keep ]; then
		[[ "$hours" =~ ^[0-9]+$ ]] && [ "$hours" -ge 1 ] && [ "$hours" -le "$MAX_HOURS" ] ||
			die "STAGING_HOURS must be 1 to $MAX_HOURS, or keep"
	fi

	local name
	if [ -n "$version" ]; then
		[ -z "$image$commit" ] || die 'set VERSION, or STAGING_IMAGE and STAGING_COMMIT, not both'
		[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || die "invalid VERSION: $version"
		name="$LABEL_PREFIX$(printf '%s' "$version" | tr -c '0-9A-Za-z' '-')"
	else
		[ -n "$image" ] || die 'set VERSION, or STAGING_IMAGE and STAGING_COMMIT'
		[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || die 'STAGING_COMMIT must be a full commit id'
		[[ "$image" =~ ^[a-z0-9./-]+:[0-9A-Za-z._-]+$ ]] || die "invalid STAGING_IMAGE: $image"
		name="$LABEL_PREFIX${commit:0:12}"
	fi
	# A test server may run the demo site's version, and the tailnet needs the two names distinct.
	local slot=demo
	if [ "$hours" != keep ]; then
		slot='test'
		name="$name-test"
	fi
	local existing replacing=''
	existing="$(ours)" || die 'could not list servers, so nothing was created'
	existing="$(in_slot "$slot" <<<"$existing")"
	if [ -n "$existing" ]; then
		# A public demo site is replaced without a gap: the new one joins the tunnel beside it.
		if [ "$slot" = demo ] && [ -n "$public_url" ]; then
			replacing="$(cut -f1 <<<"$existing")"
		else
			die "a $slot server already exists; run $([ "$slot" = demo ] && echo down-demo || echo down) first"
		fi
	fi

	local lifetime=keep
	if [ "$hours" != keep ]; then
		lifetime="expires-$(($(date +%s) + hours * 3600))"
	fi
	{
		printf '#!/bin/bash\n'
		printf 'STAGING_NAME=%q\nVERSION=%q\nSTAGING_IMAGE=%q\nSTAGING_COMMIT=%q\nSTAGING_DATABASE=%q\n' \
			"$name" "$version" "$image" "$commit" "$database"
		printf 'STAGING_PUBLIC_URL=%q\nSTAGING_GPU=%q\nTS_AUTHKEY=%q\nCF_TUNNEL_TOKEN=%q\nDEMO_ADMIN_PASSWORD=%q\n' \
			"$public_url" "$gpu" "$TS_AUTHKEY" "$tunnel_token" "$admin_password"
		cat "$ROOT/docker/staging-boot.sh.tmpl"
	} >"$TMP/boot"
	# Through a file, because the boot script holds the tailnet key and argv is visible.
	base64 -w0 "$TMP/boot" >"$TMP/boot.b64"
	jq -n --arg region "$region" --arg plan "$plan" --argjson os "$OS_ID" --arg label "$name" \
		--arg tag "$TAG" --arg lifetime "$lifetime" --rawfile boot "$TMP/boot.b64" \
		'{region: $region, plan: $plan, os_id: $os, label: $label, hostname: $label, backups: "disabled",
		  tags: [$tag, $lifetime], user_data: $boot}' >"$TMP/body.json"
	local id
	id="$(api POST /instances "$TMP/body.json" | jq -r .instance.id)"
	printf 'staging: created %s (%s), lifetime %s\n' "$name" "$id" "$lifetime"
	printf 'staging: when it has booted, open http://%s:8080 on the tailnet\n' "$name"
	if [ -n "$replacing" ]; then
		await_public "$public_url" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
		local old
		for old in $replacing; do destroy "$old"; done
	fi
}

# await_public waits until the public address is answered by a server that started after $2, which
# can only be the new one: the tunnel opens only once it has seeded and locked itself down. Until
# then the old server keeps serving. A new server that never arrives fails the run and leaves the
# old one up.
await_public() {
	local url="$1" since="$2" started='' waited=0
	while [ "$waited" -lt "$REPLACE_TIMEOUT_SECONDS" ]; do
		# Several tries a round, because the tunnel spreads requests over both servers.
		for _ in 1 2 3 4 5; do
			started="$(curl -sS --max-time 10 "$url/v1/system/version" 2>/dev/null | jq -r '.startedAt // empty' 2>/dev/null || true)"
			if [ -n "$started" ] && [[ ! "$started" < "$since" ]]; then
				printf 'staging: %s is answered by the new server (started %s)\n' "$url" "$started"
				# Let the old server finish what it is serving before it goes.
				sleep "$REPLACE_SETTLE_SECONDS"
				return
			fi
		done
		sleep 30
		waited=$((waited + 30))
	done
	# A failed boot powers off and the sweep removes it; a slow but healthy one stays beside the old.
	die "the new server did not reach $url in ${REPLACE_TIMEOUT_SECONDS}s, so the old one is still serving"
}

# in_slot keeps the lines of `ours` output that belong to a slot: demo is the kept server, test is
# everything else, so a server with an unreadable expiry counts as a test server.
in_slot() {
	if [ "$1" = demo ]; then
		awk -F '\t' '$2 == "keep"'
	else
		awk -F '\t' '$1 != "" && $2 != "keep"'
	fi
}

down() {
	[ -n "${VULTR_API_KEY:-}" ] || die 'VULTR_API_KEY is not set'
	local list id _rest
	list="$(ours)" || die 'could not list servers, so nothing was destroyed'
	list="$(in_slot "$1" <<<"$list")"
	[ -n "$list" ] || printf 'staging: no %s server to destroy\n' "$1"
	while IFS=$'\t' read -r id _rest; do
		[ -z "$id" ] || destroy "$id"
	done <<<"$list"
}

# status prints each staging server's state. Never its address: the run's log is public.
status() {
	[ -n "${VULTR_API_KEY:-}" ] || die 'VULTR_API_KEY is not set'
	local response
	if ! response="$(api GET '/instances?per_page=500')"; then
		die "the provider refused the listing: ${response:0:300}"
	fi
	jq -r --arg tag "$TAG" --arg prefix "$LABEL_PREFIX" '
		.instances[]
		| select(((.tags // []) | index($tag)) and ((.label // "") | startswith($prefix)))
		| "staging: \(.label) plan=\(.plan) power=\(.power_status) server=\(.server_status) status=\(.status) created=\(.date_created) tags=\(.tags | join(","))"' \
		<<<"$response"
}

sweep() {
	if [ -z "${VULTR_API_KEY:-}" ]; then
		printf 'staging: VULTR_API_KEY is not set, so nothing to sweep\n'
		return
	fi
	local list id expiry power now
	list="$(ours)" || die 'could not list servers, so nothing was swept'
	now="$(date +%s)"
	while IFS=$'\t' read -r id expiry power; do
		[ -n "$id" ] || continue
		# A failed boot powers the server off, the one signal it can send without a cloud key.
		if [ "$power" = stopped ]; then
			printf 'staging: %s is powered off, so its boot failed\n' "$id"
			destroy "$id"
		elif [ "$expiry" != keep ] && [ "$expiry" -le "$now" ]; then
			destroy "$id"
		fi
	done <<<"$list"
}

case "${1:-}" in
up) up ;;
down) down test ;;
down-demo) down demo ;;
sweep) sweep ;;
status) status ;;
*) die 'usage: staging.sh up|down|down-demo|sweep|status' ;;
esac
