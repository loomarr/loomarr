#!/usr/bin/env bash
# Contract test for the Shield certification analyser (#1037): fixed logcat captures in, exact
# reports out. The fixtures' expected numbers are worked by hand in the comments below.
set -euo pipefail

here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=scripts/shield-cert/lib.sh
. "$here/lib.sh"
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
failures=0

check() {
	local name=$1 expected=$2 actual=$3
	if [ "$expected" = "$actual" ]; then
		printf 'ok   %s\n' "$name"
	else
		printf 'FAIL %s\n  want %s\n  got  %s\n' "$name" "$expected" "$actual"
		failures=$((failures + 1))
	fi
}

# analyse <capture> [awk -v assignments...]: prints the JSON report.
analyse() {
	local capture=$1
	shift
	awk -v json="$work/out.json" -v codec_re="$default_codec_re" "$@" -f "$here/marks.awk" "$capture" >"$work/summary.txt"
	cat "$work/out.json"
}

# Surf: seven surfs after the boot tune. Warm 400, 550, 300, one that never framed (it counts as
# unfinished, so warm p95 fails rather than hiding it) and one a retry recovered (51500 - 50000);
# one more got a 503 and its retry never framed, so it is refused, not timed. The number jump is
# timed from its commit (31300 - 30003); the held OSD is 30, 40, 50, 120, 20, 30, 25; retries are
# part of their surf, not other tunes; the v=2 line is ignored.
check 'surf report' \
	'{"mode":"surf","surfs":7,"unfinished":1,"refused":1,"refusedBy":{"http_503":1},"unkeyed":0,"otherTunes":1,"warm":{"n":5,"p50":550,"p95":null,"max":null},"cold":{"n":1,"p50":1297,"p95":1297,"max":1297},"held":{"n":7,"p50":30,"p95":120,"max":120},"still":{"n":1,"p50":90,"p95":90,"max":90},"keyToTune":{"n":6,"p50":1,"p95":2,"max":2},"stalls":0,"errors":2,"gates":{"warmP95Max600":"FAIL","coldP95Max1500":"PASS","heldP95Max100":"FAIL","refusedMax0":"FAIL"}}' \
	"$(analyse "$here/testdata/surf.log" -v mode=surf)"
check 'surf summary names the refusal' 'refused          1 (http_503 x1)   [G3 none refused: FAIL]' \
	"$(grep '^refused ' "$work/summary.txt")"

# Soak: process 5151 watches 1798001 + 199600 ms (its last attempt ends when 6161 first logs),
# 6161 watches 1598900 ms to the end: 0.999 h with one stall. Six codec inits in the app (the
# other app's line is not counted): five inside tune windows, one mid-play video init beside the
# avc -> hevc format change. The empty-track format mark at a tune is not a change.
check 'soak report' \
	'{"mode":"soak","viewerHours":0.999,"targetHours":24,"tunes":3,"appProcesses":2,"adbReconnects":2,"stalls":1,"openStalls":0,"stallsPerViewerHour":1.001,"stallMs":{"n":1,"p50":500,"p95":500,"max":500},"codecInits":{"state":"COUNTED","total":6,"atTune":5,"midPlay":1,"midPlayVideo":1,"midPlayAudio":0},"formatChangesMidAttempt":1,"errors":{"network":1},"gate":"FAIL"}' \
	"$(analyse "$here/testdata/soak.log" -v mode=soak -v end_epoch=1790103600 -v reconnects=2)"

grep -v -e stall- -e 1790101000.010 "$here/testdata/soak.log" >"$work/clean.log"
check 'clean soak passes its target' '"gate":"PASS"}' \
	"$(analyse "$work/clean.log" -v mode=soak -v end_epoch=1790103600 -v target_h=0.5 | grep -o '"gate":.*')"
check 'clean soak short of 24 h is incomplete' '"gate":"INCOMPLETE"}' \
	"$(analyse "$work/clean.log" -v mode=soak -v end_epoch=1790103600 | grep -o '"gate":.*')"

# A codec pattern that never matches must not read as zero re-inits.
check 'unmatched codec pattern is unverified' '"state":"UNVERIFIED"' \
	"$(awk -v mode=soak -v json=/dev/stdout -v codec_re='no-such-line' -v end_epoch=1790103600 \
		-f "$here/marks.awk" "$here/testdata/soak.log" | head -n 1 | grep -o '"state":"[A-Z]*"')"

# The live filter keeps marks and codec lines and drops everything else (URLs included).
printf '%s\n' \
	'1790000000.000  1  1 I ReactNativeJS: LoomarrCert v=1 ev=first-frame t=1 att=1' \
	'1790000000.001  1  1 E ExoPlayerImplInternal: HttpDataSource failed https://host/live.m3u8?sig=x' \
	'1790000000.002  1  1 I DMCodecAdapterFactory: Creating an asynchronous MediaCodec adapter for track type video' \
	>"$work/raw.log"
check 'capture filter' '1790000000.000 1790000000.002' "$(cert_logcat_filter <"$work/raw.log" | awk '{ printf "%s%s", sep, $1; sep = " " }')"

# A running soak keeps each mark on disk as it arrives (a sparse capture must not sit in a block
# buffer), and a TERM stops it at once with a report, as a detached run on the Shield is stopped.
soak_out="$work/soak"
PATH="$here/testdata/fake-adb:$PATH" ADB_SERIAL=fake MINUTES=5 ROTATE_MINUTES=0 OUT="$soak_out" \
	bash "$here/soak.sh" </dev/null >"$work/soak.log" 2>&1 &
soak_pid=$!
for _ in $(seq 50); do
	[ -s "$soak_out/marks.log" ] && break
	sleep 0.1
done
check 'soak keeps a mark while running' 1 "$(wc -l <"$soak_out/marks.log" 2>/dev/null | tr -d ' ' || echo 0)"
stop_at=$SECONDS
kill -TERM "$soak_pid"
wait "$soak_pid" || true
check 'soak stops within 5 s of TERM' yes "$([ $((SECONDS - stop_at)) -le 5 ] && echo yes || echo "no ($((SECONDS - stop_at)) s)")"
check 'soak stopped by TERM reports the tune' '"tunes":1' "$(grep -o '"tunes":[0-9]*' "$soak_out/soak.json")"

if [ "$failures" -gt 0 ]; then
	printf '%d shield-cert check(s) failed\n' "$failures" >&2
	exit 1
fi
