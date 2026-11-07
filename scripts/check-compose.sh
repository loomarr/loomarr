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
printf '%s\n' "$sqlite" | grep -q "image: ghcr.io/loomarr/loomarr:$VERSION"
if printf '%s\n' "$sqlite" | grep -q 'DATABASE_URL:'; then
	echo 'compose-verify: SQLite deployment pins DATABASE_URL and disables in-app migration' >&2
	exit 1
fi
printf '%s\n' "$sqlite" | grep -q 'image: traefik:v3.7.16@sha256:1336e85aaed293bd63d005086e0dd013b04078b26aaeb1c18b26a2f928af7e65'
printf '%s\n' "$sqlite" | grep -q 'image: busybox:1.37@sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e'
# shellcheck disable=SC2016 # the backticks are literal Traefik rule syntax
printf '%s\n' "$sqlite" | grep -q 'providers.docker.constraints=Label(`media.loomarr.edge`,`true`)'
printf '%s\n' "$sqlite" | grep -q -- '--ping.entrypoint=health'
printf '%s\n' "$sqlite" | grep -q 'http://127.0.0.1:8082/ping'
printf '%s\n' "$sqlite" | grep -q 'traefik.http.routers.loomarr.service: loomarr'
printf '%s\n' "$sqlite" | grep -q 'traefik.http.services.loomarr.loadbalancer.server.port: "8080"'
printf '%s\n' "$sqlite" | grep -q 'SERVER_PUBLIC_URL: http://192.0.2.10'
printf '%s\n' "$sqlite" | grep -q -- '--image-tag'
printf '%s\n' "$sqlite" | grep -q 'condition: service_completed_successfully'

if "$ROOT/scripts/check-release-tag.sh" --image-tag latest >/dev/null 2>&1; then
	echo 'compose-verify: release Compose validator accepts the mutable latest alias' >&2
	exit 1
fi
"$ROOT/scripts/check-release-tag.sh" --image-tag "$VERSION"

loomarr="$(printf '%s\n' "$sqlite" | sed -n '/^  loomarr:$/,/^  [a-zA-Z0-9_-][a-zA-Z0-9_-]*:$/p')"
loomarr_port_targets="$(printf '%s\n' "$loomarr" | awk '
	/^    ports:$/ { ports = 1; next }
	ports && /^    [a-zA-Z_]/ { exit }
	ports && /target:/ { print $2 }
')"
if [ "$loomarr_port_targets" != 51029 ]; then
	echo 'compose-verify: Loomarr publishes a host port other than its single LAN-discovery UDP port' >&2
	exit 1
fi
printf '%s\n' "$loomarr" | grep -q 'published: "51029"'
printf '%s\n' "$loomarr" | grep -q 'protocol: udp'
if printf '%s\n' "$loomarr" | grep -q '^    build:'; then
	echo 'compose-verify: release Compose can fall back to an unsigned local build' >&2
	exit 1
fi
if printf '%s\n' "$sqlite" | grep -q 'target: 8082'; then
	echo 'compose-verify: Traefik admin/ping entrypoint is published' >&2
	exit 1
fi

# The demo sign-in is public, so the demo site must stay on loopback unless an operator widens it.
demo="$(LOOMARR_IMAGE=loomarr-test docker compose -f "$ROOT/docker/compose.demo.yaml" config --format json)" || {
	echo 'compose-verify: docker/compose.demo.yaml does not render' >&2
	exit 1
}
if ! printf '%s\n' "$demo" | grep -q '"host_ip": "127.0.0.1"'; then
	echo 'compose-verify: the demo site publishes beyond loopback by default' >&2
	exit 1
fi

# Both the server and the seeder must move to Postgres: a seeder left on SQLite would seed nothing
# the server can see, and the site would boot empty.
demo_postgres="$(
	LOOMARR_IMAGE=loomarr-test docker compose -f "$ROOT/docker/compose.demo.yaml" \
		-f "$ROOT/docker/compose.demo.postgres.yaml" config --format json
)" || {
	echo 'compose-verify: docker/compose.demo.postgres.yaml does not render' >&2
	exit 1
}
for service in loomarr demo-seed; do
	if ! printf '%s\n' "$demo_postgres" | jq -e --arg s "$service" \
		'.services[$s].environment.DATABASE_URL | startswith("postgres://")' >/dev/null; then
		echo "compose-verify: the Postgres demo site leaves $service off Postgres" >&2
		exit 1
	fi
done
if printf '%s\n' "$demo_postgres" | jq -e '.services.postgres.ports' >/dev/null; then
	echo 'compose-verify: the demo site publishes its database' >&2
	exit 1
fi

# The tunnel must never start with the rest of the stack: demo-site.sh starts it only after retiring
# the public admin password, and a plain `up` would skip that.
demo_public="$(
	LOOMARR_IMAGE=loomarr-test docker compose -f "$ROOT/docker/compose.demo.yaml" \
		-f "$ROOT/docker/compose.demo.public.yaml" config --format json
)" || {
	echo 'compose-verify: docker/compose.demo.public.yaml does not render' >&2
	exit 1
}
if printf '%s\n' "$demo_public" | jq -e '.services.cloudflared' >/dev/null; then
	echo 'compose-verify: the demo tunnel starts without its profile' >&2
	exit 1
fi

# make db-view serves a database copy with no sign-in, so it must publish only on loopback.
db_view="$(DB_VIEW_RUN_DIR=/run DB_VIEW_ADMIN_PASSWORD=x DB_VIEW_PG_PASSWORD=x DB_VIEW_SQLITE_FILE=/db \
	docker compose -f "$ROOT/docker/compose.db-view.yaml" --profile postgres config --format json)" || {
	echo 'compose-verify: docker/compose.db-view.yaml does not render' >&2
	exit 1
}
if ! printf '%s\n' "$db_view" | jq -e '[.services[].ports[]?.host_ip] | length > 0 and all(. == "127.0.0.1")' >/dev/null; then
	echo 'compose-verify: make db-view publishes beyond loopback' >&2
	exit 1
fi

postgres="$(
	LOOMARR_VERSION="$VERSION" SERVER_PUBLIC_URL="$PUBLIC_URL" docker compose \
		-f "$ROOT/docker/compose.yaml" \
		-f "$ROOT/docker/compose.postgres.yaml" \
		--profile postgres config 2>/dev/null
)"
printf '%s\n' "$postgres" | grep -q "image: ghcr.io/loomarr/loomarr:$VERSION"
printf '%s\n' "$postgres" | grep -q 'image: postgres:16@sha256:9e2cf6287f25c3151f4c9421ccce2b17f3e60061c111a29165e0d2cdd9ff0a97'
printf '%s\n' "$postgres" | grep -q 'DATABASE_URL: postgres://loomarr:loomarr@postgres:5432/loomarr?sslmode=disable'
if printf '%s\n' "$postgres" | grep -q 'DATABASE_URL: sqlite:'; then
	echo 'compose-verify: postgres deployment resolved Loomarr to SQLite' >&2
	exit 1
fi

echo 'compose-verify: Traefik, SQLite, and Postgres deployments are wired'
