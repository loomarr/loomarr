#!/usr/bin/env bash
# Install the reviewed Linux ccache binary used only by the Android CI producer.
set -euo pipefail

readonly destination_arg=${1:?usage: install-android-ccache.sh DESTINATION}
readonly version=4.14
readonly archive="ccache-4.14-linux-x86_64-glibc.tar.xz"
readonly url="https://github.com/ccache/ccache/releases/download/v4.14/${archive}"
readonly size=1344580
readonly sha256=45a91165db7092e67c6208ada03f54700e684c4cd3735f9031de95669ed9272c

[[ -n "${GITHUB_OUTPUT:-}" ]] || { echo 'GITHUB_OUTPUT is required' >&2; exit 2; }
mkdir -p "$destination_arg"
destination=$(realpath "$destination_arg")
readonly destination
archive_path="$destination/$archive"
curl --fail --location --retry 3 --retry-all-errors --connect-timeout 20 \
  --max-time 180 --retry-max-time 180 --silent --show-error "$url" --output "$archive_path"
[[ "$(wc -c < "$archive_path" | tr -d ' ')" == "$size" ]] || {
  echo "ccache archive size mismatch" >&2
  exit 1
}
printf '%s  %s\n' "$sha256" "$archive_path" | sha256sum -c - >/dev/null
tar -xJf "$archive_path" -C "$destination"
launcher=$(realpath "$destination/ccache-${version}-linux-x86_64-glibc/ccache")
[[ -x "$launcher" ]] || { echo "ccache launcher is missing or not executable" >&2; exit 1; }
"$launcher" --version | grep -Fxq "ccache version ${version}" || {
  echo "ccache launcher version mismatch" >&2
  exit 1
}
printf 'launcher=%s\n' "$launcher" >> "$GITHUB_OUTPUT"
