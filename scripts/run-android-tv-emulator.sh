#!/usr/bin/env bash

set -euo pipefail

readonly avd_name="${LOOMARR_TV_AVD:-loomarr-tv}"
readonly emulator_port="${LOOMARR_TV_EMULATOR_PORT:-5560}"
readonly emulator_serial="emulator-${emulator_port}"
readonly emulator_gpu="${LOOMARR_TV_EMULATOR_GPU:-auto}"
readonly memory_high="${LOOMARR_TV_EMULATOR_MEMORY_HIGH:-3G}"
readonly memory_max="${LOOMARR_TV_EMULATOR_MEMORY_MAX:-3584M}"
readonly min_available_kb="${LOOMARR_TV_EMULATOR_MIN_AVAILABLE_KB:-8388608}"
readonly boot_timeout_seconds="${LOOMARR_TV_EMULATOR_BOOT_TIMEOUT_SECONDS:-120}"
readonly scope_marker="${1:-}"
readonly window_title="Android Emulator - ${avd_name}"
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly script_dir
readonly script_path="${script_dir}/run-android-tv-emulator.sh"
readonly center_script="${script_dir}/center-android-tv-emulator.js"

if [[ -n "$scope_marker" && "$scope_marker" != "--inside-memory-scope" ]]; then
  printf 'usage: %s\n' "$0" >&2
  exit 2
fi

if [[ "$scope_marker" != "--inside-memory-scope" ]]; then
  available_kb="$(awk '/^MemAvailable:/ { print $2 }' /proc/meminfo)"
  if [[ "$available_kb" -lt "$min_available_kb" ]]; then
    printf 'refusing emulator start: %s MiB available; require at least %s MiB\n' \
      "$((available_kb / 1024))" "$((min_available_kb / 1024))" >&2
    exit 1
  fi

  if ! command -v systemd-run >/dev/null 2>&1 \
    || ! systemctl --user show-environment >/dev/null 2>&1; then
    printf 'refusing emulator start: user systemd is required for the memory ceiling\n' >&2
    exit 1
  fi

  exec systemd-run --user --scope --quiet --nice=10 \
    -p MemoryAccounting=yes \
    -p "MemoryHigh=${memory_high}" \
    -p "MemoryMax=${memory_max}" \
    -p CPUQuota=200% \
    /usr/bin/ionice -c 2 -n 7 "$script_path" --inside-memory-scope
fi

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
  boot_deadline=$((SECONDS + boot_timeout_seconds))
  while [[ "$(timeout 5 "$adb_bin" -s "$emulator_serial" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" != "1" ]]; do
    if ! kill -0 "$emulator_pid" 2>/dev/null; then
      return 1
    fi
    if (( SECONDS >= boot_deadline )); then
      printf 'emulator failed to provide a responsive Android shell within %s seconds\n' \
        "$boot_timeout_seconds" >&2
      return 1
    fi
    sleep 0.25
  done

  if command -v timedatectl >/dev/null 2>&1; then
    host_timezone="$(timedatectl show --property=Timezone --value)"
    timeout 5 "$adb_bin" -s "$emulator_serial" shell cmd alarm set-timezone "$host_timezone" >/dev/null
  fi
}

"$emulator_bin" \
  -avd "$avd_name" \
  -no-audio \
  -gpu "$emulator_gpu" \
  -port "$emulator_port" \
  -no-snapshot &
emulator_pid=$!

center_window &
if ! configure_guest; then
  kill "$emulator_pid" 2>/dev/null || true
  wait "$emulator_pid" 2>/dev/null || true
  exit 1
fi
wait "$emulator_pid"
