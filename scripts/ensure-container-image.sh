#!/usr/bin/env bash
set -u

if [[ $# -ne 1 ]]; then
  echo "usage: $0 IMAGE" >&2
  exit 2
fi

image=$1
if docker image inspect "$image" >/dev/null 2>&1; then
  exit 0
fi

pull_status=1
for attempt in 1 2 3 4 5; do
  if docker pull "$image"; then
    exit 0
  else
    pull_status=$?
  fi
  if [[ $attempt -lt 5 ]]; then
    sleep 2 || exit $?
  fi
done

exit "$pull_status"
