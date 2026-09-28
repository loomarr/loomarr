#!/usr/bin/env bash
# Shared helpers for the Shield certification scripts (#1037). Sourced, never run. Bash 3.2 safe.
# shellcheck disable=SC2034 # the variables set here are read by the sourcing scripts

cert_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

# One decoder instantiation: ExoPlayer's DefaultMediaCodecAdapterFactory logs this line (tag
# DMCodecAdapterFactory) each time it creates a MediaCodec, whatever the vendor stack beneath
# (OMX or Codec2), release builds included. Measured on the Android 11 TV emulator; override with
# CODEC_INIT_RE if a device logs otherwise (docs/engineering/shield-certification.md).
default_codec_re='DMCodecAdapterFactory.*Creating an? (a)?synchronous MediaCodec adapter'

cert_die() {
	printf '%s: %s\n' "${0##*/}" "$1" >&2
	exit "${2:-1}"
}

# Every adb call names the device: a certification run must never touch a device by accident.
cert_require_serial() {
	[ -n "${ADB_SERIAL:-}" ] || cert_die 'ADB_SERIAL is required (adb devices -l lists the serials)' 2
	command -v adb >/dev/null 2>&1 || cert_die 'adb is required' 2
}

cert_adb() {
	adb -s "$ADB_SERIAL" "$@"
}

# A network device (host:port) is re-attached with `adb connect`; USB and emulators just return.
cert_wait_for_device() {
	case "$ADB_SERIAL" in
	*:*) adb connect "$ADB_SERIAL" >/dev/null 2>&1 || true ;;
	esac
	cert_adb wait-for-device
}

cert_online() {
	[ "$(cert_adb get-state 2>/dev/null)" = "device" ]
}

# The installed TV app: the permanent Shield identity first, then the prototype identity.
cert_package() {
	if [ -n "${PACKAGE:-}" ]; then
		printf '%s\n' "$PACKAGE"
		return
	fi
	local candidate
	for candidate in loomarr.media media.loomarr.tv.prototype; do
		if cert_adb shell pm path "$candidate" 2>/dev/null | grep -q '^package:'; then
			printf '%s\n' "$candidate"
			return
		fi
	done
	cert_die 'the Loomarr TV app is not installed (set PACKAGE to override)'
}

# Brings the app to the foreground on its launcher activity (TV launcher first).
cert_launch() {
	local package=$1 activity
	activity=$(cert_adb shell cmd package resolve-activity --brief -c android.intent.category.LEANBACK_LAUNCHER "$package" 2>/dev/null | tr -d '\r' | tail -n 1)
	case "$activity" in
	*/*) ;;
	*) activity=$(cert_adb shell cmd package resolve-activity --brief -c android.intent.category.LAUNCHER "$package" 2>/dev/null | tr -d '\r' | tail -n 1) ;;
	esac
	case "$activity" in
	*/*) cert_adb shell am start -n "$activity" >/dev/null 2>&1 ;;
	*) cert_die "no launcher activity for $package" ;;
	esac
}

# Whole seconds since the epoch on the device's clock (the clock logcat stamps lines with).
cert_device_epoch() {
	cert_adb shell date +%s 2>/dev/null | tr -d '\r'
}

cert_keyevent() {
	cert_adb shell input keyevent "$@" >/dev/null 2>&1
}

# The marks and the codec lines only: nothing else from the device reaches the capture, so no
# URL or title can.
cert_logcat_filter() {
	awk -v re="${CODEC_INIT_RE:-$default_codec_re}" '/ LoomarrCert v=/ || $0 ~ re'
}
