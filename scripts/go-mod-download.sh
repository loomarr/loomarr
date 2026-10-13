#!/usr/bin/env bash
set -u

if [[ $# -ne 0 ]]; then
  echo "usage: $0" >&2
  exit 2
fi

download_status=1
for attempt in 1 2 3 4 5; do
  if go mod download; then
    exit 0
  else
    download_status=$?
  fi
  if [[ $attempt -lt 5 ]]; then
    sleep 2 || exit $?
  fi
done

exit "$download_status"
