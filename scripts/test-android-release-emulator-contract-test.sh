#!/usr/bin/env bash

set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
script="${root}/scripts/test-android-release-emulator.sh"

launcher_open=$(grep -nF 'shell input keyevent KEYCODE_HOME' "${script}" | head -1 | cut -d: -f1)
launcher_observed=$(grep -nF 'launcher surface identity' "${script}" | head -1 | cut -d: -f1)
launcher_capture=$(grep -nF 'launcher-surface.png' "${script}" | head -1 | cut -d: -f1)
activity_launch=$(grep -nFx 'launch' "${script}" | head -1 | cut -d: -f1)

[[ -n "${launcher_open}" && -n "${launcher_observed}" && -n "${launcher_capture}" && -n "${activity_launch}" ]] || {
	printf 'release emulator harness must open, observe, and capture the HOME launcher before MainActivity\n' >&2
	exit 1
}
if grep -Fq 'android.intent.category.LEANBACK_LAUNCHER' "${script}"; then
	printf 'release emulator harness must not use LEANBACK_LAUNCHER to open the HOME surface\n' >&2
	exit 1
fi
[[ "${launcher_open}" -lt "${launcher_observed}" && "${launcher_observed}" -lt "${launcher_capture}" && "${launcher_capture}" -lt "${activity_launch}" ]] || {
  printf 'launcher identity observation and capture must precede MainActivity\n' >&2
  exit 1
}
grep -Fq 'launcherSurfaceObserved: true, launcherSurfaceScreenshot: "launcher-surface.png"' "${script}" || {
	printf 'release emulator evidence manifest must retain the observed launcher screenshot\n' >&2
	exit 1
}

echo 'test-android-release-emulator-contract-test: ok'
