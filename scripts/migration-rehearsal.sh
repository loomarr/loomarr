#!/usr/bin/env bash
# Rehearse an upgrade: boot a candidate image on a copy of a real backup, and report whether the
# migrations ran, the server came up, and the migrated database is whole: SQLite's integrity check
# and foreign-key check must both come back clean (#2793). Every table whose row count the upgrade
# changed is listed, for the person releasing to compare with what the release's migrations
# meant to delete. Run it where the backup lives, so the data stays home.
#
#   scripts/migration-rehearsal.sh <backup directory> <candidate image>
#
# The directory holds loomarr.db and, if the install has one, encryption.key. The backup is only
# ever read. The container has no network, because a restored database names the household's real
# media server and requesters, and a rehearsal must not be able to reach them. Needs sqlite3.

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
command -v sqlite3 >/dev/null || {
	echo 'migration-rehearsal: needs sqlite3 to check the migrated database' >&2
	exit 2
}

# table_counts prints "<table> <rows>" for every table in the database at $1, sorted by name.
table_counts() {
	sqlite3 "$1" "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'" |
		while read -r table; do
			printf '%s %s\n' "$table" "$(sqlite3 "$1" "SELECT COUNT(*) FROM \"$table\"")"
		done | LC_ALL=C sort
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
table_counts "$WORK/loomarr.db" >"$WORK/rows-before.txt"

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

# Stop the server before reading its database, so nothing writes while it is checked.
docker stop "$NAME" >/dev/null
integrity="$(sqlite3 "$WORK/loomarr.db" 'PRAGMA integrity_check')"
if [ "$integrity" != ok ]; then
	printf '%s\n' "$integrity" | head -20 >&2
	echo "migration-rehearsal: FAILED: the migrated database fails SQLite's integrity check" >&2
	exit 1
fi
violations="$(sqlite3 "$WORK/loomarr.db" 'PRAGMA foreign_key_check')"
if [ -n "$violations" ]; then
	printf '%s\n' "$violations" | head -20 >&2
	echo "migration-rehearsal: FAILED: the migrated database has rows pointing at rows that don't exist" >&2
	exit 1
fi
table_counts "$WORK/loomarr.db" >"$WORK/rows-after.txt"
changed="$(LC_ALL=C join -a1 -a2 -e - -o 0,1.2,2.2 "$WORK/rows-before.txt" "$WORK/rows-after.txt" |
	awk '$2 != $3 { printf "  %-45s %s -> %s\n", $1, $2, $3 }')"
if [ -n "$changed" ]; then
	echo "migration-rehearsal: row counts the upgrade changed (- is a table that didn't exist):"
	printf '%s\n' "$changed"
fi
echo "migration-rehearsal: passed: $IMAGE migrated and served this backup in about ${waited}s; integrity and foreign keys clean"

# RECORD=1 leaves the evidence a final release tag needs (scripts/validate-release-source.sh): a
# commit status on the commit the image was built from. It says nothing about the backup.
if [ "${RECORD:-}" = 1 ]; then
	commit="$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$IMAGE")"
	[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || {
		echo "migration-rehearsal: $IMAGE names no commit, so nothing was recorded" >&2
		exit 1
	}
	gh api -X POST "repos/${GITHUB_REPOSITORY:-loomarr/loomarr}/statuses/$commit" \
		-f state=success -f context=loomarr/migration-rehearsal \
		-f description="Migrated a real backup and served it in about ${waited}s" >/dev/null
	echo "migration-rehearsal: recorded on $commit"
fi
