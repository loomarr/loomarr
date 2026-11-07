#!/usr/bin/env bash
# Rehearse an upgrade: boot a candidate image on a copy of a real backup, and report whether the
# migrations ran and the server came up. Run it where the backup lives, so the data stays home.
#
#   scripts/migration-rehearsal.sh <backup directory> <candidate image>
#
# The directory holds loomarr.db and, if the install has one, encryption.key. The backup is only
# ever read. The container has no network, because a restored database names the household's real
# media server and requesters, and a rehearsal must not be able to reach them.

set -euo pipefail

BACKUP="${1:-}"
IMAGE="${2:-}"
TIMEOUT_SECONDS="${REHEARSAL_TIMEOUT:-300}"
[ -n "$BACKUP" ] && [ -n "$IMAGE" ] || {
	echo 'usage: migration-rehearsal.sh <backup directory> <candidate image>' >&2
	exit 2
}
[ -f "$BACKUP/loomarr.db" ] || {
	echo "migration-rehearsal: no loomarr.db in $BACKUP" >&2
	exit 2
}

WORK="$(mktemp -d "${TMPDIR:-/tmp}/loomarr-rehearsal.XXXXXX")"
NAME="loomarr-rehearsal-$$"
cleanup() {
	docker rm -f "$NAME" >/dev/null 2>&1 || true
	rm -rf "$WORK"
}
trap cleanup EXIT

cp "$BACKUP/loomarr.db" "$WORK/loomarr.db"
if [ -f "$BACKUP/encryption.key" ]; then
	cp "$BACKUP/encryption.key" "$WORK/encryption.key"
fi

docker run -d --name "$NAME" --network none --cpus 2 --user "$(id -u):$(id -g)" \
	-e DATABASE_URL=sqlite:///data/loomarr.db -e AUTO_MIGRATE=true \
	-v "$WORK:/data" "$IMAGE" >/dev/null

waited=0
until docker exec "$NAME" /loomarr healthcheck >/dev/null 2>&1; do
	if [ "$(docker inspect -f '{{.State.Running}}' "$NAME")" != true ]; then
		docker logs --tail 20 "$NAME" >&2
		echo "migration-rehearsal: FAILED: $IMAGE stopped on this backup" >&2
		exit 1
	fi
	if [ "$waited" -ge "$TIMEOUT_SECONDS" ]; then
		docker logs --tail 20 "$NAME" >&2
		echo "migration-rehearsal: FAILED: $IMAGE was not healthy in ${TIMEOUT_SECONDS}s" >&2
		exit 1
	fi
	sleep 2
	waited=$((waited + 2))
done
echo "migration-rehearsal: passed: $IMAGE migrated and served this backup in about ${waited}s"
