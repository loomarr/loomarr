#!/usr/bin/env bash
# make db-view: open a copy of a Loomarr database in CloudBeaver on loopback, until Ctrl-C.
#
#   make db-view                    this worktree's database, SQLite or Postgres
#   make db-view FILE=<backup.db>   a SQLite file at rest, such as a nightly backup
#   make db-view URL=postgres://…   a pg_dump of that database
#   make db-view ENGINE=postgres    a throwaway Postgres built by the migrations and cmd/seed
#
# The viewer only ever sees a copy (docker/compose.db-view.yaml says why), and every copy is
# removed on exit. A dump reads its source; nothing here writes to a real database.
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
compose_file="$repo_root/docker/compose.db-view.yaml"
file=${DB_VIEW_FILE:-}
url=${DB_VIEW_URL:-}
engine=${DB_VIEW_ENGINE:-}

die() {
  echo "db-view: $*" >&2
  exit 1
}

given=0
for value in "$file" "$url" "$engine"; do
  [[ -n "$value" ]] && given=$((given + 1))
done
((given <= 1)) || die 'pass at most one of FILE, URL and ENGINE'
[[ -z "$engine" || "$engine" == postgres ]] || die "ENGINE=$engine: only ENGINE=postgres is supported"

# Captured first: eval of a failed command substitution would carry on with nothing set.
dev_env=$(LOOMARR_REPO_ROOT="$repo_root" "$repo_root/scripts/dev-env.sh" export)
eval "$dev_env"

# This worktree's database, resolved the way make dev-be and observability-dev resolve it. The
# subshell keeps .env out of this script's environment, so cmd/seed below never inherits the
# checkout's integrations.
worktree_database_url() {
  (
    if [[ -f "$repo_root/.env" ]]; then
      set -a
      # shellcheck disable=SC1091 # The per-checkout environment is intentionally dynamic.
      source "$repo_root/.env"
      set +a
    fi
    printf '%s\n' "${LOOMARR_AGENT_DATABASE_URL:-${DATABASE_URL:-sqlite://$repo_root/loomarr-dev.db}}"
  )
}

source_path=''
if [[ -n "$file" ]]; then
  mode=sqlite-file
  source_path=$file
elif [[ -n "$url" ]]; then
  mode=postgres-dump
elif [[ -n "$engine" ]]; then
  mode=postgres-seed
else
  database_url=$(worktree_database_url)
  case "$database_url" in
    sqlite://*)
      mode=sqlite-snapshot
      source_path=${database_url#sqlite://}
      [[ "$source_path" == /* ]] || source_path="$repo_root/$source_path"
      ;;
    postgres://* | postgresql://*)
      mode=postgres-dump
      url=$database_url
      ;;
    *) die "unrecognised database URL scheme: ${database_url%%://*}" ;;
  esac
fi

if [[ "$mode" == postgres-dump ]]; then
  [[ "$url" == postgres://* || "$url" == postgresql://* ]] || die 'URL must start with postgres:// or postgresql://'
fi

if [[ -n "$source_path" ]]; then
  [[ -f "$source_path" ]] || die "no SQLite database at $source_path (make seed creates one)"
  source_path=$(cd "$(dirname "$source_path")" && pwd)/$(basename "$source_path")
fi

project="$COMPOSE_PROJECT_NAME-db-view"
compose() {
  docker compose -p "$project" -f "$compose_file" "$@"
}

# By label: `compose ps` would need the per-run variables the Compose file requires.
if [[ -n "$(docker ps -aq --filter "label=com.docker.compose.project=$project")" ]]; then
  die "already running for this worktree. If no db-view is open, remove the leftovers with:
  docker rm -fv \$(docker ps -aq --filter label=com.docker.compose.project=$project)"
fi

random_hex() {
  od -An -N16 -tx1 /dev/urandom | tr -d ' \n'
}

run_dir="${LOOMARR_ARTIFACT_DIR:?}/db-view"
idle_pid=''
cleanup() {
  [[ -z "$idle_pid" ]] || kill "$idle_pid" 2>/dev/null || true
  compose --profile postgres down -v --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$run_dir"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# The directory is private to you. The files in it stay readable because CloudBeaver's launcher
# switches from root to its own user, and chowns the runtime config it rewrites.
rm -rf "$run_dir"
mkdir -p "$run_dir"
chmod 700 "$run_dir"

export DB_VIEW_RUN_DIR="$run_dir"
export DB_VIEW_ADMIN_PASSWORD DB_VIEW_PG_PASSWORD
# The admin password meets CloudBeaver's policy: mixed case and a digit.
DB_VIEW_ADMIN_PASSWORD="Dbview-$(random_hex)"
DB_VIEW_PG_PASSWORD=$(random_hex)
# CloudBeaver signs in to the Postgres copy as this pg_read_all_data role, so the server refuses
# writes. Its own read-only flag is a session setting that its SQL editor can switch off.
viewer_password=$(random_hex)
# Compose mounts this path whatever the mode; the Postgres modes leave it empty.
export DB_VIEW_SQLITE_FILE="$run_dir/empty.db"
: >"$DB_VIEW_SQLITE_FILE"

case "$mode" in
  sqlite-snapshot)
    command -v sqlite3 >/dev/null || die 'the sqlite3 CLI is needed to snapshot a live database; or pass FILE=<backup.db>'
    # The source goes into a file: URI, where these characters mean something else.
    [[ "$source_path" != *[?#%]* ]] || die "a path containing ?, # or % can't be snapshotted: $source_path"
    snapshot="$run_dir/snapshot.db"
    # The snapshot path goes inside an SQL string literal.
    [[ "$snapshot" != *"'"* ]] || die "the artifact directory contains a quote: $run_dir"
    echo "db-view: snapshotting $source_path"
    # VACUUM INTO only reads its source, and a read-only handle makes sure of it. Under WAL it
    # neither blocks nor waits for the app's writer, and the copy is one consistent transaction.
    sqlite3 "file:$source_path?mode=ro" "VACUUM INTO '$snapshot'"
    DB_VIEW_SQLITE_FILE=$snapshot
    ;;
  sqlite-file)
    # A -wal beside the file means a running database, whose newest commits live in the WAL that
    # an immutable open would skip. Snapshot that one instead.
    [[ ! -e "$source_path-wal" ]] || die "$source_path has a -wal file, so it's live; snapshot it, or pass a backup"
    DB_VIEW_SQLITE_FILE=$source_path
    ;;
esac

if [[ "$mode" == sqlite-* ]]; then
  # immutable=1 skips locking and the -shm file, which a read-only mount can't create.
  connection='"loomarr": {"provider": "sqlite", "driver": "sqlite_jdbc", "name": "Loomarr (SQLite copy)", "read-only": true,
    "configuration": {"url": "jdbc:sqlite:file:/snapshot/loomarr.db?immutable=1", "database": "/snapshot/loomarr.db", "type": "dev", "auth-model": "native"}}'
else
  connection='"loomarr": {"provider": "postgresql", "driver": "postgres-jdbc", "name": "Loomarr (Postgres copy)", "read-only": true, "save-password": true,
    "configuration": {"host": "postgres", "port": "5432", "database": "loomarr", "url": "jdbc:postgresql://postgres:5432/loomarr",
      "user": "dbview", "password": "'"$viewer_password"'", "type": "dev", "auth-model": "native"}}'
fi
printf '{"folders": {}, "connections": {%s}}\n' "$connection" >"$run_dir/data-sources.conf"

# SQLite is an embedded driver, which CloudBeaver disables unless enabledDrivers names it. Anonymous
# viewers see the preconfigured connection without signing in; the port is loopback-only.
cat >"$run_dir/cloudbeaver.runtime.conf" <<'CONF'
{
  app: {
    anonymousAccessEnabled: true,
    grantConnectionsAccessToAnonymousTeam: true,
    supportsCustomConnections: false,
    enabledDrivers: ["sqlite:sqlite_jdbc"]
  }
}
CONF

if [[ "$mode" == postgres-* ]]; then
  echo 'db-view: starting a throwaway Postgres'
  compose --progress quiet --profile postgres up -d --wait postgres
  case "$mode" in
    postgres-seed)
      pg_address=$(compose port postgres 5432)
      echo 'db-view: building the schema with the real migrations and seeding it (cmd/seed)'
      (cd "$repo_root" && DATABASE_URL="postgres://loomarr:$DB_VIEW_PG_PASSWORD@$pg_address/loomarr?sslmode=disable" go run ./cmd/seed >/dev/null)
      ;;
    postgres-dump)
      pg_image=$(compose --profile postgres config --images | grep '^postgres:')
      echo 'db-view: copying the database with pg_dump'
      # pg_dump runs on the host network, so a localhost URL works on Linux; Docker Desktop needs
      # host.docker.internal. It reads one consistent snapshot and writes nothing to its source.
      # A server newer than this pg_dump refuses.
      docker run --rm --network host "$pg_image" \
        pg_dump --format=custom --no-owner --no-privileges "$url" |
        compose exec -T postgres pg_restore --no-owner --no-privileges --exit-on-error -U loomarr -d loomarr
      ;;
  esac
  compose exec -T postgres psql -q -v ON_ERROR_STOP=1 -U loomarr -d loomarr \
    -c "CREATE ROLE dbview LOGIN PASSWORD '$viewer_password' IN ROLE pg_read_all_data"
fi

echo 'db-view: starting CloudBeaver'
compose --progress quiet up -d --wait cloudbeaver
viewer_address=$(compose port cloudbeaver 8978)

case "$mode" in
  sqlite-snapshot) what="a snapshot of $source_path" ;;
  sqlite-file) what="$source_path, read-only" ;;
  postgres-dump) what='a copy of the Postgres database' ;;
  postgres-seed) what='a throwaway Postgres with seed data' ;;
esac
cat <<EOF

db-view: ready at http://$viewer_address
  showing  $what
  Open "Loomarr" in the navigator. Ctrl-C removes the viewer and the copy.
EOF

# Open a browser on a desktop session; over SSH there is no display, so the URL above is the way in.
if [[ -z "${DB_VIEW_NO_BROWSER:-}" ]]; then
  if [[ "$(uname -s)" == Darwin ]]; then
    open "http://$viewer_address" || true
  elif [[ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]] && command -v xdg-open >/dev/null; then
    xdg-open "http://$viewer_address" >/dev/null 2>&1 || true
  fi
fi

# wait returns as soon as a trapped signal arrives. cleanup stops the sleep, because a background
# child of a script ignores SIGINT and would outlive it. macOS sleep has no `infinity`.
while :; do
  sleep 3600 &
  idle_pid=$!
  wait "$idle_pid" || true
done
