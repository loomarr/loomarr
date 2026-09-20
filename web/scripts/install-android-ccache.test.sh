#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
installer="$script_dir/install-android-ccache.sh"

grep -Fq 'ccache-4.14-linux-x86_64-glibc.tar.xz' "$installer"
grep -Fq '45a91165db7092e67c6208ada03f54700e684c4cd3735f9031de95669ed9272c' "$installer"
grep -Fq '1344580' "$installer"
grep -Fq 'https://github.com/ccache/ccache/releases/download/v4.14/' "$installer"
grep -Fq 'sha256sum -c -' "$installer"
grep -Fq 'ccache version 4.14' "$installer"
# The installer source must retain this literal version interpolation.
# shellcheck disable=SC2016
grep -Fq 'ccache-${version}-linux-x86_64-glibc/ccache' "$installer"
grep -Fq 'realpath' "$installer"
grep -Fq 'GITHUB_OUTPUT' "$installer"
for flag in '--retry-all-errors' '--connect-timeout 20' '--max-time 180' '--retry-max-time 180'; do
  grep -Fq -- "$flag" "$installer"
done
