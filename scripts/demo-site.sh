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
# DEMO_SITE_PUBLIC=1 with CF_TUNNEL_TOKEN and DEMO_ADMIN_PASSWORD puts it on the internet through a
# Cloudflare Tunnel. The demo admin's password is in this repository, so visitors get a member
# account instead, and the tunnel starts only once that password no longer works.
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

PUBLIC="${DEMO_SITE_PUBLIC:-0}"

# DEMO_SITE_CONTENT=open serves the licensed open content pack (docs/demo-content-credits.md)
# instead of the invented library. The public site defaults to it; everything else to invented.
DEFAULT_CONTENT=invented
[ "$PUBLIC" = 1 ] && DEFAULT_CONTENT=open
export DEMO_SITE_CONTENT="${DEMO_SITE_CONTENT:-$DEFAULT_CONTENT}"
case "$DEMO_SITE_CONTENT" in
invented | open) ;;
*)
	echo "demo-site: DEMO_SITE_CONTENT must be invented or open, not $DEMO_SITE_CONTENT" >&2
	exit 2
	;;
esac

ADMIN_LOGIN='{"username":"demo","password":"loomarr-demo"}'
GUEST_USER=guest
GUEST_PASSWORD=loomarr-guest

compose() {
	local files=(-f "$ROOT/docker/compose.demo.yaml")
	if [ "$DATABASE" = postgres ]; then
		files+=(-f "$ROOT/docker/compose.demo.postgres.yaml")
	fi
	if [ "$PUBLIC" = 1 ]; then
		files+=(-f "$ROOT/docker/compose.demo.public.yaml")
	fi
	docker compose "${files[@]}" "$@"
}

# admin_login prints the status of signing in with the public admin password, keeping the session
# in the cookie jar named by $1.
admin_login() {
	curl -sS -o /dev/null -w '%{http_code}' --max-time 30 -c "$1" -H 'Content-Type: application/json' \
		-d "$ADMIN_LOGIN" "$BASE/v1/auth/login"
}

# lock_down leaves visitors a member account and replaces the public admin password with
# DEMO_ADMIN_PASSWORD, which only the maintainer knows.
lock_down() {
	local BASE="http://${DEMO_SITE_BIND:-127.0.0.1}:$DEMO_SITE_PORT" jar status
	jar="$(mktemp)"
	# shellcheck disable=SC2064 # the path is fixed now, and a local is gone by the time this runs
	trap "rm -f '$jar'" RETURN
	status="$(admin_login "$jar")"
	if [ "$status" = 401 ]; then
		echo 'demo-site: the public admin password is already retired'
		return
	fi
	[ "$status" = 200 ] || {
		echo "demo-site: sign-in returned $status, so the site stays private" >&2
		exit 1
	}

	status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 30 -b "$jar" -H 'X-Loomarr-Csrf: 1' -H 'Content-Type: application/json' \
		-d "{\"username\":\"$GUEST_USER\",\"password\":\"$GUEST_PASSWORD\",\"role\":\"member\"}" "$BASE/v1/users")"
	case "$status" in
	2?? | 409) ;;
	*)
		echo "demo-site: creating the visitor account returned $status, so the site stays private" >&2
		exit 1
		;;
	esac

	status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 30 -b "$jar" -H 'X-Loomarr-Csrf: 1' -H 'Content-Type: application/json' \
		-d "{\"current\":\"loomarr-demo\",\"next\":\"$DEMO_ADMIN_PASSWORD\"}" "$BASE/v1/auth/password")"
	case "$status" in
	2??) ;;
	*)
		echo "demo-site: changing the admin password returned $status, so the site stays private" >&2
		exit 1
		;;
	esac

	# Proof, not a status code: the tunnel opens only if the public password is really refused.
	status="$(admin_login "$jar")"
	[ "$status" = 401 ] || {
		echo "demo-site: the public admin password still returns $status, so the site stays private" >&2
		exit 1
	}
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
	if [ "$PUBLIC" = 1 ]; then
		if [ -z "${CF_TUNNEL_TOKEN:-}" ]; then
			echo 'demo-site: DEMO_SITE_PUBLIC=1 needs CF_TUNNEL_TOKEN' >&2
			exit 2
		fi
		# Plain characters only: it goes into a JSON body, and the server has no jq to escape it.
		if ! [[ "${DEMO_ADMIN_PASSWORD:-}" =~ ^[A-Za-z0-9._~-]{16,}$ ]] || [ "$DEMO_ADMIN_PASSWORD" = loomarr-demo ]; then
			echo 'demo-site: DEMO_SITE_PUBLIC=1 needs DEMO_ADMIN_PASSWORD: 16 or more letters, digits or ._~-' >&2
			exit 2
		fi
	fi
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

	# On SQLite the seeder and the server write to one file from two processes, so a seed can lose
	# the lock. It is safe to rerun, and a released seeder can't be changed, so try again here.
	local seed status attempt waited=0
	seed="$(compose ps -a -q demo-seed)"
	for attempt in 1 2 3; do
		while [ "$(docker inspect -f '{{.State.Status}}' "$seed")" != exited ]; do
			if [ "$waited" -ge "$SEED_TIMEOUT_SECONDS" ]; then
				echo "demo-site: the seeder did not finish in ${SEED_TIMEOUT_SECONDS}s" >&2
				exit 1
			fi
			sleep 5
			waited=$((waited + 5))
		done
		status="$(docker inspect -f '{{.State.ExitCode}}' "$seed")"
		if [ "$status" = 0 ] || [ "$attempt" = 3 ]; then break; fi
		echo "demo-site: the seeder failed with status $status, trying again" >&2
		compose start demo-seed
	done
	if [ "$status" != 0 ]; then
		compose logs --tail 40 demo-seed >&2
		echo "demo-site: the seeder failed with status $status" >&2
		exit 1
	fi
	if [ "$PUBLIC" = 1 ]; then
		lock_down
		compose --profile public up -d cloudflared
		printf 'demo site ready: %s\nsign in as %s / %s\n' "$DEMO_SITE_URL" "$GUEST_USER" "$GUEST_PASSWORD"
		return
	fi
	printf 'demo site ready: %s\nsign in as demo / loomarr-demo\n' "$DEMO_SITE_URL"
}

case "${1:-}" in
up) up ;;
down)
	# Compose reads the whole file even to tear down, and teardown needs no image. Always include
	# the optional layers, or a run started on them would leave containers and a volume behind.
	DATABASE=postgres PUBLIC=1 LOOMARR_IMAGE="${LOOMARR_IMAGE:-unused}" compose --profile public down -v
	;;
*)
	echo 'usage: demo-site.sh up|down' >&2
	exit 2
	;;
esac
