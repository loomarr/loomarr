#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
temp_dir=$(mktemp -d)
trap 'rm -r -- "$temp_dir"' EXIT

mkdir -p "$temp_dir/bin" "$temp_dir/lib"/{arm64-v8a,armeabi-v7a,x86,x86_64}
for abi in arm64-v8a armeabi-v7a x86 x86_64; do
	: >"$temp_dir/lib/$abi/libfixture.so"
done
ln -s "$script_dir/testdata/fake-readelf.sh" "$temp_dir/bin/readelf"

PATH="$temp_dir/bin:$PATH" "$script_dir/verify-android-native-libraries.sh" "$temp_dir/lib"

: >"$temp_dir/lib/arm64-v8a/libunaligned.so"
if PATH="$temp_dir/bin:$PATH" "$script_dir/verify-android-native-libraries.sh" "$temp_dir/lib" 2>"$temp_dir/error"; then
	echo 'android native verifier: accepted an unaligned 64-bit library' >&2
	exit 1
fi
grep -Fq 'arm64-v8a/libunaligned.so has LOAD alignment 0x1000 below 16 KiB' "$temp_dir/error"

rm -r -- "$temp_dir/lib/x86"
if PATH="$temp_dir/bin:$PATH" "$script_dir/verify-android-native-libraries.sh" "$temp_dir/lib" 2>"$temp_dir/error"; then
	echo 'android native verifier: accepted a bundle without x86 coverage' >&2
	exit 1
fi
grep -Fq 'bundle is missing required ABI x86' "$temp_dir/error"

echo 'android-native-libraries-test: ok'
