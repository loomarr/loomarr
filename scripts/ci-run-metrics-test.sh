#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output="$(CI_ASSURANCE_LANE=merge-queue "$ROOT/scripts/ci-run-metrics.sh" "$ROOT/scripts/testdata/ci-run-metrics.json")"

for expected in \
  '**Lane:** merge-queue · **Selected jobs:** 3 · **Skipped jobs:** 1' \
  '**End-to-end:** 11.7m' \
  '**Occupied runner time:** 15.5m' \
  '**Longest job:** Apple clients — native build + launch (mobile) (10m)' \
  '| Apple clients — native build + launch (mobile) | success | 1m | 10m |' \
  '| Go — race-policy tests (1/3) | success | 0.5m | 5m |'; do
  if ! grep -Fq -- "$expected" <<<"$output"; then
    printf 'ci-run-metrics-test: missing %s in:\n%s\n' "$expected" "$output" >&2
    exit 1
  fi
done

if grep -Fq 'Manual CI — full scope' <<<"$output"; then
  printf 'ci-run-metrics-test: skipped jobs must not consume timing rows\n' >&2
  exit 1
fi

echo 'ci-run-metrics-test: ok'
