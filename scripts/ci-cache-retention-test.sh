#!/usr/bin/env bash

set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
retention="${repo_root}/scripts/ci-cache-retention.sh"
test_root=$(mktemp -d /tmp/ci-cache-retention-test.XXXXXX)
trap 'rm -rf -- "${test_root}"' EXIT

hash_a=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
hash_b=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb

plan() {
  "${retention}" plan "$1" | sort -n | tr '\n' ' '
}

# One listing with every managed family, a superseded entry in each, and entries the script must
# leave alone: another ref, a family it does not manage, and a family with a single entry.
cat >"${test_root}/caches.json" <<EOF
{"actions_caches": [
  {"id": 1,  "ref": "refs/heads/main", "key": "go-test-Linux-2026W47-1/2-${hash_a}-warm", "created_at": "2026-11-16T00:03:00Z"},
  {"id": 2,  "ref": "refs/heads/main", "key": "go-test-Linux-2026W47-1/2-${hash_b}-warm", "created_at": "2026-11-22T00:14:00Z"},
  {"id": 3,  "ref": "refs/heads/main", "key": "go-test-Linux-2026W47-certification-1/3-${hash_a}-warm", "created_at": "2026-11-16T00:02:00Z"},
  {"id": 4,  "ref": "refs/heads/main", "key": "codeql-overlay-base-database-1-45a68cc62958b166-go-2.27.5-eccaeb160deb", "created_at": "2026-11-21T17:06:00Z"},
  {"id": 5,  "ref": "refs/heads/main", "key": "codeql-overlay-base-database-1-45a68cc62958b166-go-2.27.5-c61392752aaa", "created_at": "2026-11-22T00:38:00Z"},
  {"id": 6,  "ref": "refs/heads/main", "key": "codeql-overlay-base-database-1-14bfe9eef27ba5b2-javascript-2.27.5-c61392752aaa", "created_at": "2026-11-22T00:37:00Z"},
  {"id": 7,  "ref": "refs/pull/9/merge", "key": "go-test-Linux-2026W47-1/2-${hash_a}-warm", "created_at": "2026-11-15T00:00:00Z"},
  {"id": 8,  "ref": "refs/heads/main", "key": "android-tv-ccache-v2-Linux-ccache-4.14-${hash_a}-1", "created_at": "2026-11-01T00:00:00Z"},
  {"id": 9,  "ref": "refs/heads/main", "key": "android-tv-ccache-v2-Linux-ccache-4.14-${hash_a}-2", "created_at": "2026-11-02T00:00:00Z"},
  {"id": 10, "ref": "refs/heads/main", "key": "merge-gate-go-Linux-2026112200", "created_at": "2026-11-22T00:15:00Z"},
  {"id": 11, "ref": "refs/heads/main", "key": "merge-gate-go-Linux-2026112201", "created_at": "2026-11-22T01:15:00Z"},
  {"id": 12, "ref": "refs/heads/main", "key": "pnpm-Linux-${hash_a}", "created_at": "2026-11-20T00:00:00Z"},
  {"id": 13, "ref": "refs/heads/main", "key": "pnpm-Linux-${hash_b}", "created_at": "2026-11-21T00:00:00Z"},
  {"id": 14, "ref": "refs/heads/main", "key": "ffmpeg-Linux-X64-${hash_a}", "created_at": "2026-11-21T00:00:00Z"},
  {"id": 15, "ref": "refs/heads/main", "key": "gobuild-pg-Linux-2026W46-${hash_a}-warm", "created_at": "2026-11-09T00:00:00Z"},
  {"id": 16, "ref": "refs/heads/main", "key": "gobuild-pg-Linux-2026W47-${hash_a}-warm", "created_at": "2026-11-16T00:00:00Z"},
  {"id": 17, "ref": "refs/heads/main", "key": "go-contracts-Linux-2026W47-${hash_a}-warm", "created_at": "2026-11-16T00:04:00Z"},
  {"id": 18, "ref": "refs/heads/main", "key": "go-contracts-Linux-2026W47-${hash_b}-warm", "created_at": "2026-11-22T00:18:00Z"}
]}
EOF

got=$(plan "${test_root}/caches.json")
want='1 4 10 12 15 17 '
if [[ "${got}" != "${want}" ]]; then
  printf 'ci-cache-retention-test: plan got "%s", want "%s"\n' "${got}" "${want}" >&2
  exit 1
fi

# A family whose newest save never happened keeps the entry it has: nothing names a key to keep.
cat >"${test_root}/single.json" <<EOF
{"actions_caches": [
  {"id": 21, "ref": "refs/heads/main", "key": "merge-gate-go-Linux-2026112200", "created_at": "2026-11-22T00:15:00Z"},
  {"id": 22, "ref": "refs/heads/main", "key": "pnpm-Linux-${hash_a}", "created_at": "2026-11-20T00:00:00Z"}
]}
EOF
got=$(plan "${test_root}/single.json")
if [[ -n "${got}" ]]; then
  printf 'ci-cache-retention-test: single-entry families got "%s", want nothing\n' "${got}" >&2
  exit 1
fi

# Entries saved in the same second are ordered by id, so the plan is stable.
cat >"${test_root}/tie.json" <<EOF
{"actions_caches": [
  {"id": 32, "ref": "refs/heads/main", "key": "pnpm-Linux-${hash_b}", "created_at": "2026-11-20T00:00:00Z"},
  {"id": 31, "ref": "refs/heads/main", "key": "pnpm-Linux-${hash_a}", "created_at": "2026-11-20T00:00:00Z"}
]}
EOF
got=$(plan "${test_root}/tie.json")
if [[ "${got}" != '31 ' ]]; then
  printf 'ci-cache-retention-test: same-second entries got "%s", want "31 "\n' "${got}" >&2
  exit 1
fi

printf '{"actions_caches": []}\n' >"${test_root}/empty.json"
got=$(plan "${test_root}/empty.json")
if [[ -n "${got}" ]]; then
  printf 'ci-cache-retention-test: empty listing got "%s"\n' "${got}" >&2
  exit 1
fi

# A listing it cannot trust deletes nothing.
printf '{"actions_caches": {"total": 1}}\n' >"${test_root}/not-an-array.json"
if "${retention}" plan "${test_root}/not-an-array.json" >/dev/null 2>&1; then
  echo 'ci-cache-retention-test: plan accepted a listing that is not an array' >&2
  exit 1
fi
cat >"${test_root}/bad-id.json" <<EOF
{"actions_caches": [
  {"id": "12", "ref": "refs/heads/main", "key": "pnpm-Linux-${hash_a}", "created_at": "2026-11-20T00:00:00Z"},
  {"id": 13, "ref": "refs/heads/main", "key": "pnpm-Linux-${hash_b}", "created_at": "2026-11-21T00:00:00Z"}
]}
EOF
if "${retention}" plan "${test_root}/bad-id.json" >/dev/null 2>&1; then
  echo 'ci-cache-retention-test: plan accepted a non-numeric cache id' >&2
  exit 1
fi
if "${retention}" plan "${test_root}/missing.json" >/dev/null 2>&1; then
  echo 'ci-cache-retention-test: plan accepted a missing listing' >&2
  exit 1
fi

# apply lists main's caches and deletes exactly what plan prints. `gh` is a recorder here.
mkdir -p "${test_root}/bin"
cat >"${test_root}/bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${GH_LOG}"
case "$*" in
  "api repos/loomarr/loomarr/actions/caches?ref=refs/heads/main&per_page=100 --paginate")
    cat "${GH_LISTING}"
    ;;
  "api -X DELETE repos/loomarr/loomarr/actions/caches/"*) ;;
  *)
    echo "unexpected gh call: $*" >&2
    exit 1
    ;;
esac
EOF
chmod +x "${test_root}/bin/gh"
GH_LOG="${test_root}/gh.log" GH_LISTING="${test_root}/caches.json" REPO=loomarr/loomarr \
  PATH="${test_root}/bin:${PATH}" "${retention}" apply >/dev/null
deleted=$(grep -c -- '-X DELETE' "${test_root}/gh.log")
if [[ "${deleted}" != 6 ]]; then
  printf 'ci-cache-retention-test: apply deleted %s caches, want 6\n' "${deleted}" >&2
  exit 1
fi
if grep -q 'caches/2$\|caches/8$\|caches/9$\|caches/7$' "${test_root}/gh.log"; then
  echo 'ci-cache-retention-test: apply deleted a cache it must keep' >&2
  exit 1
fi

echo 'ci-cache-retention-test: ok'
