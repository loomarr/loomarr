#!/usr/bin/env bash
# Install the reviewed ccache binary used by the Android producer.
set -euo pipefail

readonly destination_arg=${1:?usage: install-android-ccache.sh DESTINATION}
readonly version=4.14

case "$(uname -s):$(uname -m)" in
  Darwin:arm64|Darwin:x86_64)
    readonly archive="ccache-4.14-darwin.tar.gz"
    readonly archive_dir="ccache-4.14-darwin"
    readonly size=2203957
    readonly sha256=353a81ea8680d93387102cfde288ebed381a54272cfdc18224f241add6332b39
    ;;
  Linux:x86_64)
    readonly archive="ccache-4.14-linux-x86_64-glibc.tar.xz"
    readonly archive_dir="ccache-4.14-linux-x86_64-glibc"
    readonly size=1344580
    readonly sha256=45a91165db7092e67c6208ada03f54700e684c4cd3735f9031de95669ed9272c
    ;;
  *)
    printf 'ccache 4.14 has no reviewed Loomarr binary for %s/%s\n' "$(uname -s)" "$(uname -m)" >&2
    exit 2
    ;;
esac
readonly url="https://github.com/ccache/ccache/releases/download/v4.14/${archive}"

mkdir -p "$destination_arg"
destination=$(realpath "$destination_arg")
readonly destination
archive_path="$destination/$archive"
launcher="$destination/$archive_dir/ccache"
if [[ -x "$launcher" ]] && "$launcher" --version | grep -Fxq "ccache version ${version}"; then
  [[ -z "${GITHUB_OUTPUT:-}" ]] || printf 'launcher=%s\n' "$launcher" >> "$GITHUB_OUTPUT"
  printf '%s\n' "$launcher"
  exit 0
fi
curl --fail --location --retry 3 --retry-all-errors --connect-timeout 20 \
  --max-time 180 --retry-max-time 180 --silent --show-error "$url" --output "$archive_path"
[[ "$(wc -c < "$archive_path" | tr -d ' ')" == "$size" ]] || {
  echo "ccache archive size mismatch" >&2
  exit 1
}
if command -v sha256sum >/dev/null 2>&1; then
  printf '%s  %s\n' "$sha256" "$archive_path" | sha256sum -c - >/dev/null
else
  [[ "$(shasum -a 256 "$archive_path" | awk '{print $1}')" == "$sha256" ]] || {
    echo "ccache archive SHA-256 mismatch" >&2
    exit 1
  }
fi
tar -xf "$archive_path" -C "$destination"
launcher=$(realpath "$destination/$archive_dir/ccache")
[[ -x "$launcher" ]] || { echo "ccache launcher is missing or not executable" >&2; exit 1; }
"$launcher" --version | grep -Fxq "ccache version ${version}" || {
  echo "ccache launcher version mismatch" >&2
  exit 1
}
[[ -z "${GITHUB_OUTPUT:-}" ]] || printf 'launcher=%s\n' "$launcher" >> "$GITHUB_OUTPUT"
printf '%s\n' "$launcher"
