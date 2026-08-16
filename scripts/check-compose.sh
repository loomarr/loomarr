#!/usr/bin/env sh

set -eu

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
ROOT="$(CDPATH='' cd -- "$SCRIPT_DIR/.." && pwd -P)"
VERSION=0.1.0-beta.1
PUBLIC_URL=http://192.0.2.10

if LOOMARR_VERSION='' SERVER_PUBLIC_URL="$PUBLIC_URL" docker compose \
	-f "$ROOT/docker/compose.yaml" --profile sqlite config >/dev/null 2>&1; then
	echo 'compose-verify: release Compose accepts an unpinned Loomarr image' >&2
	exit 1
fi
if LOOMARR_VERSION="$VERSION" SERVER_PUBLIC_URL='' docker compose \
	-f "$ROOT/docker/compose.yaml" --profile sqlite config >/dev/null 2>&1; then
	echo 'compose-verify: release Compose accepts an empty public URL' >&2
	exit 1
fi

sqlite="$(
	LOOMARR_VERSION="$VERSION" SERVER_PUBLIC_URL="$PUBLIC_URL" docker compose \
		-f "$ROOT/docker/compose.yaml" \
		--profile sqlite config 2>/dev/null
)"
printf '%s\n' "$sqlite" | grep -q "image: ghcr.io/mantonx/loomarr:$VERSION"
printf '%s\n' "$sqlite" | grep -q 'DATABASE_URL: sqlite:///data/loomarr.db'
printf '%s\n' "$sqlite" | grep -q 'image: traefik:v3.7.1@sha256:6b9cbca6fac42ab0075f5437d8dc1685cfd188626d8d515839ea94f8b6271c42'
# shellcheck disable=SC2016 # the backticks are literal Traefik rule syntax
printf '%s\n' "$sqlite" | grep -q 'providers.docker.constraints=Label(`com.mantonx.loomarr.edge`,`true`)'
printf '%s\n' "$sqlite" | grep -q -- '--ping.entrypoint=health'
printf '%s\n' "$sqlite" | grep -q 'traefik.http.routers.loomarr.service: loomarr'
printf '%s\n' "$sqlite" | grep -q 'traefik.http.services.loomarr.loadbalancer.server.port: "8080"'
printf '%s\n' "$sqlite" | grep -q 'SERVER_PUBLIC_URL: http://192.0.2.10'

loomarr="$(printf '%s\n' "$sqlite" | sed -n '/^  loomarr:$/,/^  [a-zA-Z0-9_-][a-zA-Z0-9_-]*:$/p')"
if printf '%s\n' "$loomarr" | grep -q '^    ports:'; then
	echo 'compose-verify: Loomarr bypasses Traefik through a host port' >&2
	exit 1
fi
if printf '%s\n' "$loomarr" | grep -q '^    build:'; then
	echo 'compose-verify: release Compose can fall back to an unsigned local build' >&2
	exit 1
fi
if printf '%s\n' "$sqlite" | grep -q 'target: 8082'; then
	echo 'compose-verify: Traefik admin/ping entrypoint is published' >&2
	exit 1
fi

postgres="$(
	LOOMARR_VERSION="$VERSION" SERVER_PUBLIC_URL="$PUBLIC_URL" docker compose \
		-f "$ROOT/docker/compose.yaml" \
		-f "$ROOT/docker/compose.postgres.yaml" \
		--profile postgres config 2>/dev/null
)"
printf '%s\n' "$postgres" | grep -q "image: ghcr.io/mantonx/loomarr:$VERSION"
printf '%s\n' "$postgres" | grep -q 'DATABASE_URL: postgres://loomarr:loomarr@postgres:5432/loomarr?sslmode=disable'
if printf '%s\n' "$postgres" | grep -q 'DATABASE_URL: sqlite:'; then
	echo 'compose-verify: postgres deployment resolved Loomarr to SQLite' >&2
	exit 1
fi

echo 'compose-verify: Traefik, SQLite, and Postgres deployments are wired'
