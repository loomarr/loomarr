#!/usr/bin/env bash

set -euo pipefail

library_root=${1:?usage: verify-android-native-libraries.sh <base-lib-directory>}

mapfile -t libraries < <(find "$library_root" -type f -name '*.so' -print | sort)
if ((${#libraries[@]} == 0)); then
	echo 'android release: bundle inspection found no native libraries' >&2
	exit 1
fi

for abi in arm64-v8a armeabi-v7a x86 x86_64; do
	if [[ ! -d "$library_root/$abi" ]]; then
		echo "android release: bundle is missing required ABI $abi" >&2
		exit 1
	fi
done

for library in "${libraries[@]}"; do
	case "$library" in
		*/arm64-v8a/*|*/x86_64/*) ;;
		*) continue ;;
	esac
	mapfile -t alignments < <(readelf -lW "$library" | awk '$1 == "LOAD" {print $NF}')
	if ((${#alignments[@]} == 0)); then
		echo "android release: no ELF load segments found in ${library#"$library_root/"}" >&2
		exit 1
	fi
	for alignment in "${alignments[@]}"; do
		if ((alignment < 0x4000)); then
			echo "android release: ${library#"$library_root/"} has LOAD alignment $alignment below 16 KiB" >&2
			exit 1
		fi
	done
done
