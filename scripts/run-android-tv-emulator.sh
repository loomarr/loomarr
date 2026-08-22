#!/usr/bin/env bash

set -euo pipefail

readonly avd_name="${LOOMARR_TV_AVD:-loomarr-tv}"
readonly emulator_port="${LOOMARR_TV_EMULATOR_PORT:-5560}"
readonly emulator_serial="emulator-${emulator_port}"
readonly window_title="Android Emulator - ${avd_name}"
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly script_dir
readonly center_script="${script_dir}/center-android-tv-emulator.js"

if command -v emulator >/dev/null 2>&1; then
  emulator_bin="$(command -v emulator)"
elif [[ -n "${ANDROID_HOME:-}" && -x "${ANDROID_HOME}/emulator/emulator" ]]; then
  emulator_bin="${ANDROID_HOME}/emulator/emulator"
else
  echo "Android emulator not found; add it to PATH or set ANDROID_HOME." >&2
  exit 1
fi

if command -v adb >/dev/null 2>&1; then
  adb_bin="$(command -v adb)"
elif [[ -n "${ANDROID_HOME:-}" && -x "${ANDROID_HOME}/platform-tools/adb" ]]; then
  adb_bin="${ANDROID_HOME}/platform-tools/adb"
else
  echo "adb not found; add it to PATH or set ANDROID_HOME." >&2
  exit 1
fi

center_window() {
  while ! wmctrl -l | grep -Fq "$window_title"; do
    if ! kill -0 "$emulator_pid" 2>/dev/null; then
      return
    fi
    sleep 0.25
  done

  # KWin must place this by the REAL Wayland output geometry. wmctrl reports Xwayland's synthetic
  # coordinates on mixed portrait/landscape displays, and the generic "centre active window"
  # shortcut can race the emulator's Extended Controls window. The script targets the emulator
  # caption itself and chooses KWin's primary output, so neither ambiguity can recur.
  if command -v qdbus6 >/dev/null 2>&1 && [[ -f "$center_script" ]]; then
    qdbus6 org.kde.KWin /Scripting org.kde.kwin.Scripting.unloadScript \
      loomarr-center-android-tv >/dev/null 2>&1 || true
    qdbus6 org.kde.KWin /Scripting org.kde.kwin.Scripting.loadScript \
      "$center_script" loomarr-center-android-tv >/dev/null
    qdbus6 org.kde.KWin /Scripting org.kde.kwin.Scripting.start >/dev/null
    sleep 0.25
    qdbus6 org.kde.KWin /Scripting org.kde.kwin.Scripting.unloadScript \
      loomarr-center-android-tv >/dev/null
  else
    wmctrl -a "$window_title"
  fi
}

configure_guest() {
  "$adb_bin" -s "$emulator_serial" wait-for-device
  while [[ "$("$adb_bin" -s "$emulator_serial" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" != "1" ]]; do
    if ! kill -0 "$emulator_pid" 2>/dev/null; then
      return
    fi
    sleep 0.25
  done

  if command -v timedatectl >/dev/null 2>&1; then
    host_timezone="$(timedatectl show --property=Timezone --value)"
    "$adb_bin" -s "$emulator_serial" shell cmd alarm set-timezone "$host_timezone" >/dev/null
  fi
}

"$emulator_bin" \
  -avd "$avd_name" \
  -no-audio \
  -gpu swiftshader_indirect \
  -port "$emulator_port" \
  -no-snapshot &
emulator_pid=$!

center_window &
configure_guest &
wait "$emulator_pid"
