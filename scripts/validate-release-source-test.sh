#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
test_root=$(mktemp -d /tmp/release-source-test.XXXXXX)
trap 'find "$test_root" -mindepth 1 -depth -delete; rmdir "$test_root"' EXIT
mkdir -p "$test_root/bin"

sha=0123456789abcdef0123456789abcdef01234567

cat > "$test_root/bin/git" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  'rev-parse HEAD'|'rev-parse origin/main') printf '%s\n' "$RELEASE_TEST_SHA" ;;
  'fetch --no-tags origin main') ;;
  *) echo "unexpected git invocation: $*" >&2; exit 64 ;;
esac
STUB

cat > "$test_root/bin/gh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" == *'/statuses?per_page=100'* ]]; then
  printf '%s\n' "$RELEASE_TEST_REHEARSAL"
elif [[ "$*" == *'/workflows/playout-bench.yml/runs'* ]]; then
  printf '7\n8\n'
elif [[ "$*" == *'/runs/7/jobs'* ]]; then
  printf '%b\n' "${RELEASE_TEST_BENCH_JOBS%%;*}"
elif [[ "$*" == *'/runs/8/jobs'* ]]; then
  [[ "$RELEASE_TEST_BENCH_JOBS" != *';'* ]] || printf '%b\n' "${RELEASE_TEST_BENCH_JOBS#*;}"
elif [[ "$*" == *'/jobs?per_page=100'* ]]; then
  printf '%b\n' "$RELEASE_TEST_JOBS"
else
  printf '%b\n' "$RELEASE_TEST_RUN"
fi
STUB
chmod +x "$test_root/bin/git" "$test_root/bin/gh"

common_jobs=$'CI\tsuccess\nImage — release build / Image — release build (linux/amd64)\tsuccess\nImage — release build / Image — release build (linux/arm64)\tsuccess'
candidate_jobs=$'Release candidate — exact main scope\tsuccess\nGo — repository contracts / Go — repository contracts (1/2)\tsuccess\nGo — repository contracts / Go — repository contracts (2/2)\tsuccess\nRust — repository contracts / Rust — repository contracts\tsuccess\nRust image — runtime certification / Rust image — runtime certification\tsuccess'

arc='Self-hosted — Intel Arc (VAAPI)'
geforce='Self-hosted — NVIDIA GeForce (NVENC)'
# Successful bench job names per run: run 7, then (after ';') run 8.
both_legs="$arc\\n$geforce"

run_case() {
  local name=$1 event=$2 jobs=$3 expected=$4 tag=${5:-v1.2.3} rehearsal=${6-success} bench=${7-$both_legs}
  local output status
  set +e
  output=$(cd "$root" && \
    PATH="$test_root/bin:$PATH" \
    RELEASE_TEST_SHA="$sha" \
    RELEASE_TEST_RUN=$'42\tsuccess\t'"$event" \
    RELEASE_TEST_JOBS="$jobs" \
    RELEASE_TEST_REHEARSAL="$rehearsal" \
    RELEASE_TEST_BENCH_JOBS="$bench" \
    GITHUB_REF_NAME="$tag" \
    GITHUB_REPOSITORY=loomarr/loomarr \
    GITHUB_SHA="$sha" \
    ./scripts/validate-release-source.sh 2>&1)
  status=$?
  set -e
  if [[ "$expected" == pass && $status -ne 0 ]]; then
    printf 'validate-release-source-test: %s unexpectedly failed:\n%s\n' "$name" "$output" >&2
    exit 1
  fi
  if [[ "$expected" == fail && $status -eq 0 ]]; then
    printf 'validate-release-source-test: %s unexpectedly passed\n' "$name" >&2
    exit 1
  fi
}

run_case push-evidence push "$common_jobs" fail
run_case final-without-rehearsal workflow_dispatch "$common_jobs"$'\n'"$candidate_jobs" fail v1.2.3 ''
run_case final-with-failed-rehearsal workflow_dispatch "$common_jobs"$'\n'"$candidate_jobs" fail v1.2.3 failure
run_case rc-without-rehearsal workflow_dispatch "$common_jobs"$'\n'"$candidate_jobs" pass v1.2.3-rc.1 ''
run_case candidate-evidence workflow_dispatch "$common_jobs"$'\n'"$candidate_jobs" pass
run_case final-without-hardware-bench workflow_dispatch "$common_jobs"$'\n'"$candidate_jobs" fail v1.2.3 success ''
run_case final-without-geforce workflow_dispatch "$common_jobs"$'\n'"$candidate_jobs" fail v1.2.3 success "$arc"
run_case final-legs-in-separate-runs workflow_dispatch "$common_jobs"$'\n'"$candidate_jobs" pass v1.2.3 success "$arc;$geforce"
run_case rc-without-hardware-bench workflow_dispatch "$common_jobs"$'\n'"$candidate_jobs" pass v1.2.3-rc.1 '' ''
run_case full-evidence workflow_dispatch "$common_jobs"$'\nManual CI — full scope\tsuccess' fail
run_case candidate-missing-contract workflow_dispatch \
  "$common_jobs"$'\nRelease candidate — exact main scope\tsuccess\nRust — repository contracts / Rust — repository contracts\tsuccess\nRust image — runtime certification / Rust image — runtime certification\tsuccess' fail
run_case candidate-missing-contract-shard workflow_dispatch \
  "$common_jobs"$'\nRelease candidate — exact main scope\tsuccess\nGo — repository contracts / Go — repository contracts (1/2)\tsuccess\nRust — repository contracts / Rust — repository contracts\tsuccess\nRust image — runtime certification / Rust image — runtime certification\tsuccess' fail
run_case candidate-missing-rust-contract workflow_dispatch \
  "$common_jobs"$'\nRelease candidate — exact main scope\tsuccess\nGo — repository contracts / Go — repository contracts (1/2)\tsuccess\nGo — repository contracts / Go — repository contracts (2/2)\tsuccess\nRust image — runtime certification / Rust image — runtime certification\tsuccess' fail
run_case dispatch-missing-marker workflow_dispatch "$common_jobs" fail
run_case unsupported-event pull_request "$common_jobs" fail
run_case missing-aggregate push \
  $'Image — release build / Image — release build (linux/amd64)\tsuccess\nImage — release build / Image — release build (linux/arm64)\tsuccess' fail

echo 'validate-release-source-test: ok'
