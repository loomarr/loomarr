#!/usr/bin/env bash
# Contract for scripts/check-hardware-gate.sh, against a fake gh: no network.

set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd -P)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
mkdir "$WORK/bin"

# The fake prints $WORK/runs for the run list, and $WORK/jobs-<id> for a run's passing jobs.
cat >"$WORK/bin/gh" <<'FAKE'
#!/usr/bin/env bash
if [ -e "$FAKE_DIR/api-down" ]; then exit 1; fi
case "$2" in
*/workflows/*) cat "$FAKE_DIR/runs" ;;
*/runs/*/jobs*) id="${2#*/runs/}"; cat "$FAKE_DIR/jobs-${id%%/*}" ;;
esac
FAKE
chmod +x "$WORK/bin/gh"
export PATH="$WORK/bin:$PATH" FAKE_DIR="$WORK"

COMMIT=0123456789abcdef0123456789abcdef01234567
ARC='Self-hosted — Intel Arc (VAAPI)'
GEFORCE='Self-hosted — NVIDIA GeForce (NVENC)'

fail() {
	printf 'check-hardware-gate-test: %s\n' "$1" >&2
	exit 1
}
passes() { "$ROOT/scripts/check-hardware-gate.sh" "$COMMIT" >/dev/null 2>&1; }

printf '1\n' >"$WORK/runs"
printf '%s\n%s\n' "$ARC" "$GEFORCE" >"$WORK/jobs-1"
passes || fail 'both families passing was refused'

# Two runs can each supply one family, since a leg may need a retry on its own.
printf '1\n2\n' >"$WORK/runs"
printf '%s\n' "$ARC" >"$WORK/jobs-1"
printf '%s\n' "$GEFORCE" >"$WORK/jobs-2"
passes || fail 'families passing across two runs were refused'

printf '1\n' >"$WORK/runs"
printf '%s\n' "$ARC" >"$WORK/jobs-1"
if passes; then fail 'a missing NVIDIA leg passed'; fi
printf '%s\n' "$GEFORCE" >"$WORK/jobs-1"
if passes; then fail 'a missing Intel leg passed'; fi

# The nightly's shape: only macOS passed, and the GPU legs were skipped.
printf 'macos-15 — VideoToolbox\nHardware coverage\n' >"$WORK/jobs-1"
if passes; then fail 'a run with no GPU leg passed'; fi

: >"$WORK/runs"
if passes; then fail 'a commit with no bench run passed'; fi

touch "$WORK/api-down"
if passes; then fail 'an unreachable API passed'; fi
rm "$WORK/api-down"

if "$ROOT/scripts/check-hardware-gate.sh" abc123 >/dev/null 2>&1; then fail 'a short commit passed'; fi

printf 'check-hardware-gate-test: ok\n'
