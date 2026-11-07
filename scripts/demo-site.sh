#!/usr/bin/env bash
# The demo site: real Loomarr beside the invented demo library (docker/compose.demo.yaml).
#
#   scripts/demo-site.sh up      start it, then wait until the seeder has finished
#   scripts/demo-site.sh down    stop it and delete its data
#
# `up` needs one of:
#   VERSION=0.2.0-beta.9-rc.11   a published release; its tag supplies the demo tool's source
#   LOOMARR_IMAGE=<image>        an image built from this checkout
#
# DEMO_SITE_DATABASE=postgres runs it on Postgres instead of SQLite.
#
# A seeder newer than the server would migrate the store past what the server can open, which is
# why a release builds the tool from its own tag and never from this checkout.

set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd -P)"
RELEASE_IMAGE=ghcr.io/loomarr/loomarr
SEED_TIMEOUT_SECONDS="${DEMO_SITE_SEED_TIMEOUT:-1500}"

eval "$("$ROOT/scripts/dev-env.sh" export)"
export COMPOSE_PROJECT_NAME="${DEMO_SITE_PROJECT:-$COMPOSE_PROJECT_NAME-demo}"
export DEMO_SITE_PORT="${DEMO_SITE_PORT:-$((23000 + LOOMARR_AGENT_SLOT))}"
export DEMO_SITE_URL="${DEMO_SITE_URL:-http://localhost:$DEMO_SITE_PORT}"
export LOOMARR_IMAGE="${LOOMARR_IMAGE:-}"

DATABASE="${DEMO_SITE_DATABASE:-sqlite}"
case "$DATABASE" in
sqlite | postgres) ;;
*)
	echo "demo-site: DEMO_SITE_DATABASE must be sqlite or postgres, not $DATABASE" >&2
	exit 2
	;;
esac

compose() {
	local files=(-f "$ROOT/docker/compose.demo.yaml")
	if [ "$DATABASE" = postgres ]; then
		files+=(-f "$ROOT/docker/compose.demo.postgres.yaml")
	fi
	docker compose "${files[@]}" "$@"
}

# release_source exports the tagged tree with this checkout's demo Dockerfile, which an older tag
# does not have.
release_source() {
	local dir="$LOOMARR_ARTIFACT_DIR/demo-source/$VERSION"
	git -C "$ROOT" rev-parse --verify --quiet "refs/tags/v$VERSION" >/dev/null || {
		echo "demo-site: no tag v$VERSION here; run git fetch --tags" >&2
		exit 2
	}
	rm -rf "$dir"
	mkdir -p "$dir"
	git -C "$ROOT" archive "v$VERSION" | tar -x -C "$dir"
	mkdir -p "$dir/docker"
	cp "$ROOT/docker/demo.Dockerfile" "$dir/docker/demo.Dockerfile"
	printf '%s\n' "$dir"
}

up() {
	if [ -n "${VERSION:-}" ] && [ -n "$LOOMARR_IMAGE" ]; then
		echo 'demo-site: set VERSION or LOOMARR_IMAGE, not both' >&2
		exit 2
	fi
	if [ -n "${VERSION:-}" ]; then
		LOOMARR_IMAGE="$RELEASE_IMAGE:$VERSION"
		LOOMARR_DEMO_SOURCE="$(release_source)"
		export LOOMARR_DEMO_SOURCE
	elif [ -z "$LOOMARR_IMAGE" ]; then
		echo 'demo-site: set VERSION=<release> or LOOMARR_IMAGE=<image built from this checkout>' >&2
		exit 2
	fi

	compose up -d --build

	local seed status waited=0
	seed="$(compose ps -a -q demo-seed)"
	while [ "$(docker inspect -f '{{.State.Status}}' "$seed")" != exited ]; do
		if [ "$waited" -ge "$SEED_TIMEOUT_SECONDS" ]; then
			echo "demo-site: the seeder did not finish in ${SEED_TIMEOUT_SECONDS}s" >&2
			exit 1
		fi
		sleep 5
		waited=$((waited + 5))
	done
	status="$(docker inspect -f '{{.State.ExitCode}}' "$seed")"
	if [ "$status" != 0 ]; then
		compose logs --tail 40 demo-seed >&2
		echo "demo-site: the seeder failed with status $status" >&2
		exit 1
	fi
	printf 'demo site ready: %s\nsign in as demo / loomarr-demo\n' "$DEMO_SITE_URL"
}

case "${1:-}" in
up) up ;;
down)
	# Compose reads the whole file even to tear down, and teardown needs no image. Always include
	# the Postgres layer, or a run started on it would leave its container and volume behind.
	DATABASE=postgres LOOMARR_IMAGE="${LOOMARR_IMAGE:-unused}" compose down -v
	;;
*)
	echo 'usage: demo-site.sh up|down' >&2
	exit 2
	;;
esac
