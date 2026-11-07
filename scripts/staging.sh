#!/usr/bin/env bash
# On-demand staging: one cloud server running the demo site, destroyed when it expires.
#
#   scripts/staging.sh up      create it (VERSION=<release>, or STAGING_IMAGE + STAGING_COMMIT)
#   scripts/staging.sh down    destroy it now
#   scripts/staging.sh sweep   destroy anything past its expiry
#
# STAGING_HOURS=keep makes the long-running demo site: the sweep leaves it alone, and only `down`
# destroys it. It is opt-in by name, so an unreadable expiry still means destroy.
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
REGION=ewr
OS_ID=2284 # Ubuntu 24.04 LTS x64
MAX_HOURS=24
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

# ours prints one "id expiry" line per staging server. The expiry is "keep" for the demo site, and
# 0 when it is unreadable.
ours() {
	api GET '/instances?per_page=500' | jq -r --arg tag "$TAG" --arg prefix "$LABEL_PREFIX" '
		.instances[]
		| select((.tags | index($tag)) and (.label | startswith($prefix)))
		| [.id, (if (.tags | index("keep")) then "keep"
			else ([.tags[] | select(test("^expires-[0-9]+$")) | ltrimstr("expires-")][0] // "0") end)]
		| @tsv'
}

destroy() {
	api DELETE "/instances/$1" >/dev/null
	printf 'staging: destroyed %s\n' "$1"
}

up() {
	local hours="${STAGING_HOURS:-4}" version="${VERSION:-}" image="${STAGING_IMAGE:-}" commit="${STAGING_COMMIT:-}"
	[ -n "${VULTR_API_KEY:-}" ] || die 'VULTR_API_KEY is not set'
	[ -n "${TS_AUTHKEY:-}" ] || die 'TS_AUTHKEY is not set'
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
	[ -z "$(ours)" ] || die 'a staging server already exists; run down first'

	local lifetime=keep
	if [ "$hours" != keep ]; then
		lifetime="expires-$(($(date +%s) + hours * 3600))"
	fi
	{
		printf '#!/bin/bash\n'
		printf 'STAGING_NAME=%q\nVERSION=%q\nSTAGING_IMAGE=%q\nSTAGING_COMMIT=%q\nTS_AUTHKEY=%q\n' \
			"$name" "$version" "$image" "$commit" "$TS_AUTHKEY"
		cat "$ROOT/docker/staging-boot.sh.tmpl"
	} >"$TMP/boot"
	jq -n --arg region "$REGION" --arg plan "$PLAN" --argjson os "$OS_ID" --arg label "$name" \
		--arg tag "$TAG" --arg lifetime "$lifetime" --arg boot "$(base64 -w0 "$TMP/boot")" \
		'{region: $region, plan: $plan, os_id: $os, label: $label, hostname: $label, backups: "disabled",
		  tags: [$tag, $lifetime], user_data: $boot}' >"$TMP/body.json"
	local id
	id="$(api POST /instances "$TMP/body.json" | jq -r .instance.id)"
	printf 'staging: created %s (%s), lifetime %s\n' "$name" "$id" "$lifetime"
	printf 'staging: when it has booted, open http://%s:8080 on the tailnet\n' "$name"
}

down() {
	[ -n "${VULTR_API_KEY:-}" ] || die 'VULTR_API_KEY is not set'
	local id _expiry
	while IFS=$'\t' read -r id _expiry; do
		destroy "$id"
	done < <(ours)
}

sweep() {
	if [ -z "${VULTR_API_KEY:-}" ]; then
		printf 'staging: VULTR_API_KEY is not set, so nothing to sweep\n'
		return
	fi
	local id expiry now
	now="$(date +%s)"
	while IFS=$'\t' read -r id expiry; do
		if [ "$expiry" != keep ] && [ "$expiry" -le "$now" ]; then
			destroy "$id"
		fi
	done < <(ours)
}

case "${1:-}" in
up) up ;;
down) down ;;
sweep) sweep ;;
*) die 'usage: staging.sh up|down|sweep' ;;
esac
