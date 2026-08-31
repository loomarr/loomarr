#!/usr/bin/env bash

set -euo pipefail

library=${*: -1}
case "$library" in
	*/arm64-v8a/libunaligned.so|*/x86_64/libunaligned.so) alignment=0x1000 ;;
	*/armeabi-v7a/*|*/x86/*) alignment=0x1000 ;;
	*) alignment=0x4000 ;;
esac

printf '  LOAD 0 0 0 0 0 R E %s\n' "$alignment"
