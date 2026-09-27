#!/usr/bin/env bash
# watermark-vaapi-matrix.sh — which overlay_vaapi graph draws the channel bug on THIS GPU (#1512 1d).
#
# WHY THIS EXISTS. On the household Intel Arc the first production graph (a premultiplied bgra
# bug, hwupload, overlay_vaapi after pad_vaapi) failed the self-check's blend at full speed, exit 0.
# Which graph blends correctly is a hardware question, so this script measures a matrix of
# candidates on the real GPU instead of guessing. Each candidate changes ONE thing from production.
# Its first run on the Arc (ffmpeg n8.1.2, iHD) found overlay_vaapi blends a bgra bug as STRAIGHT
# alpha: the premultiplied bug read 129 where 65% white is 178, the straight one 178. PROD is now
# the straight bgra bug; pm-bgra is the old graph. Run it on any new VAAPI driver or ffmpeg.
#
# For every candidate and programme class (SDR H.264; HDR10 HEVC tone-mapped by tonemap_opencl when
# the image has an OpenCL runtime for the GPU) it encodes 25 frames bug-off and bug-on through the
# production decode, filters and h264_vaapi encoder, then prints:
#   - bug luma vs the expected blend (65% white over the measured background, or 235 for the opaque
#     control) and the background itself: bug == background means NOTHING was drawn;
#   - control ΔY: mean luma change of the picture left of the bug (the programme must survive);
#   - SPS/PPS: identical to bug-off, or the differing fields (breaks air bug-off in the same stream).
# REF-cpu blends on the CPU; it is NOT a production candidate (the bug is never drawn on the CPU),
# it only proves the measurement reads 65% white where a correct blend happened.
#
# It writes only to its own scratch directory. Run it in a throwaway container of the image under
# test, on the host with the GPU:
#
#   docker run --rm --device /dev/dri --group-add "$(stat -c %g /dev/dri/renderD128)" \
#     -v "$PWD/scripts/watermark-vaapi-matrix.sh:/matrix.sh:ro" --entrypoint bash \
#     ghcr.io/loomarr/loomarr:0.2.0-beta.7 /matrix.sh
#
# and again with an image built from main (Intel's OpenCL runtime, so the HDR class runs too).
#
# Env (optional): RENDER_NODE (default /dev/dri/renderD128), FFMPEG (default ffmpeg), KEEP=1 keeps
# the scratch directory, SELFTEST=1 swaps every GPU stage for its CPU twin so the script's own
# plumbing can be checked on a machine without VAAPI (its verdicts say nothing about any GPU).

set -u

NODE="${RENDER_NODE:-/dev/dri/renderD128}"
FF="${FFMPEG:-ffmpeg}"
W="$(mktemp -d)"
if [ "${KEEP:-}" = 1 ]; then
	echo "scratch: $W"
else
	trap 'rm -rf "$W"' EXIT
fi

X=1760 Y=54 B=64 INSET=6 FRAME=12
ALPHA=0.651 # 166/255, baked into the test bug

PRE=(-init_hw_device "vaapi=va:$NODE" -filter_hw_device va -hwaccel vaapi -hwaccel_device va -hwaccel_output_format vaapi)
ENC=(-c:v h264_vaapi -profile:v high -rc_mode QVBR -global_quality 22 -b:v 8000k -maxrate 12000k -g 25 -bf 0 -sei 0)
TAIL="fps=25,setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv:chroma_location=left"
FIT="w=1920:h=1080:force_original_aspect_ratio=decrease:force_divisible_by=2"
PAD="pad_vaapi=w=1920:h=1080:x=(ow-iw)/2:y=(oh-ih)/2"
SDR_PREPAD="scale_vaapi=$FIT:format=nv12"
HDR_PREPAD="scale_vaapi=$FIT:format=p010,hwmap=derive_device=opencl,tonemap_opencl=tonemap=hable:desat=0:t=bt709:m=bt709:p=bt709:r=tv:format=nv12,hwmap=derive_device=vaapi:reverse=1"

# cpu_twin GRAPH → the graph with every GPU stage replaced by its CPU equivalent (SELFTEST only).
cpu_twin() {
	sed -E -e 's/,scale_vaapi=format=bgra//g' -e 's/scale_vaapi=[^,;[]*/scale=1920:1080:force_original_aspect_ratio=decrease,format=nv12/g' \
		-e 's/,hwmap=derive_device=opencl,tonemap_opencl=[^,]*,hwmap=derive_device=vaapi:reverse=1/,zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,format=nv12/' \
		-e 's/pad_vaapi=w=([0-9]+):h=([0-9]+):x=([^:]*):y=([^,;[]*)/pad=\1:\2:\3:\4/g' \
		-e 's/overlay_vaapi=x=([0-9]+):y=([0-9]+)[^,;[]*/overlay=x=\1:y=\2/g' \
		-e 's/,hwupload(=extra_hw_frames=[0-9]+)?//g; s/,hwdownload,format=nv12//g' <<<"$1"
}
if [ "${SELFTEST:-}" = 1 ]; then
	PRE=()
	ENC=(-c:v libx264 -preset ultrafast -pix_fmt yuv420p -g 25 -bf 0)
	echo "SELFTEST: CPU stand-ins for every GPU stage; these verdicts say nothing about a GPU."
fi

# encode SRC VF OUT → 25 frames through the production encoder; prints ffmpeg's first error line.
encode() {
	local vf="$2"
	[ "${SELFTEST:-}" = 1 ] && vf="$(cpu_twin "$vf")"
	"$FF" -hide_banner -nostdin -loglevel error -y "${PRE[@]}" -i "$1" -map 0:v:0 -frames:v 25 \
		-vf "$vf" "${ENC[@]}" -f h264 "$3" 2>"$3.err" || { head -1 "$3.err"; return 1; }
}

# yavg FILE CROP → mean luma of frame $FRAME inside crop w:h:x:y.
yavg() {
	"$FF" -hide_banner -nostdin -loglevel error -i "$1" \
		-vf "select=eq(n\,$FRAME),crop=$2,signalstats,metadata=print:key=lavfi.signalstats.YAVG:file=-" \
		-frames:v 1 -f null - 2>/dev/null | sed -n 's/^lavfi.signalstats.YAVG=//p' | head -1
}

# params FILE → the first SPS and PPS, field by field (trace_headers, addresses stripped).
params() {
	"$FF" -hide_banner -nostdin -loglevel trace -i "$1" -map 0:v:0 -c copy -bsf:v trace_headers \
		-frames:v 1 -f null - 2>&1 | grep '^\[trace_headers' | sed -E 's/^\[trace_headers @ [^]]*\] //' | grep -v '^Packet:' |
		awk '/Sequence Parameter Set|Picture Parameter Set/{on=1} /Slice Header|Supplemental Enhancement|Access Unit Delimiter/{if(on)exit} on'
}

# --- fixtures ----------------------------------------------------------------

echo "== host"
"$FF" -hide_banner -version | head -1
if [ "${SELFTEST:-}" != 1 ]; then
	"$FF" -hide_banner -nostdin -v verbose -init_hw_device "vaapi=va:$NODE" -f lavfi -i nullsrc=s=64x64 \
		-frames:v 1 -f null - 2>&1 | grep -i -m2 "driver\|VAAPI" | sed -E 's/^\[[^]]*\] //'
fi

# A flat dark patch under the bug keeps the hypotheses apart. Over testsrc2's mid-grey there, a
# premultiplied bug through a straight blend lands within 3 luma of "nothing drawn"; over Y≈71 the
# outcomes are nothing ≈71, correct blend ≈178, premultiplied-as-straight ≈128, straight-as-
# premultiplied 235 (clipped).
PATCH="drawbox=x=1700:y=0:w=220:h=200:color=0x404040:t=fill"

# The test bug: a 64x64 white square at 65% alpha, straight and premultiplied.
if ! "$FF" -hide_banner -nostdin -loglevel error -y -f lavfi -i "color=c=0xFFFFFFA6:s=${B}x${B},format=rgba" -frames:v 1 "$W/bug.png" ||
	! "$FF" -hide_banner -nostdin -loglevel error -y -f lavfi -i "color=c=0xA6A6A6A6:s=${B}x${B},format=rgba" -frames:v 1 "$W/bug.pm.png"; then
	echo "cannot write the test bug"
	exit 1
fi
printf 'bug pixel (RGBA): straight %s, premultiplied %s\n' \
	"$("$FF" -loglevel error -i "$W/bug.png" -f rawvideo -pix_fmt rgba -frames:v 1 - | od -An -tu1 -N4 | xargs)" \
	"$("$FF" -loglevel error -i "$W/bug.pm.png" -f rawvideo -pix_fmt rgba -frames:v 1 - | od -An -tu1 -N4 | xargs)"

"$FF" -hide_banner -nostdin -loglevel error -y -f lavfi -i "testsrc2=size=1920x1080:rate=25:duration=1.2,$PATCH" \
	-c:v libx264 -preset ultrafast -pix_fmt yuv420p -g 25 "$W/sdr.mkv" || { echo "cannot write the SDR fixture"; exit 1; }
"$FF" -hide_banner -nostdin -loglevel error -y -f lavfi -i "testsrc2=size=1920x1080:rate=25:duration=1.2,$PATCH" \
	-vf "zscale=tin=bt709:min=bt709:pin=bt709:rin=tv:t=smpte2084:p=bt2020:m=bt2020nc:r=tv:npl=203,format=yuv420p10le" \
	-c:v libx265 -preset ultrafast -x265-params "log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc" \
	-color_primaries bt2020 -color_trc smpte2084 -colorspace bt2020nc "$W/hdr.mkv" || echo "no HDR fixture (libx265/zscale missing): HDR class skipped"

# --- candidates --------------------------------------------------------------
#
# name | main chain (before the overlay) | bug chain | blend | after the blend | expect (blend|opaque)
# @PREPAD@ is the class's scale (and tone-map) chain. Each row changes one thing from PROD.

PM="movie=filename=$W/bug.pm.png"
ST="movie=filename=$W/bug.png"
OV="overlay_vaapi=x=$X:y=$Y"
CANDIDATES="$(
	cat <<EOF
PROD|@PREPAD@,$PAD|$ST,format=bgra,hwupload|$OV||blend
pm-bgra|@PREPAD@,$PAD|$PM,format=bgra,hwupload|$OV||blend
pm-rgba|@PREPAD@,$PAD|$PM,format=rgba,hwupload|$OV||blend
straight-rgba|@PREPAD@,$PAD|$ST,format=rgba,hwupload|$OV||blend
pm-argb|@PREPAD@,$PAD|$PM,format=argb,hwupload|$OV||blend
pm-abgr|@PREPAD@,$PAD|$PM,format=abgr,hwupload|$OV||blend
straight-vuya|@PREPAD@,$PAD|$ST,format=vuya,hwupload|$OV||blend
opaque-bgr0|@PREPAD@,$PAD|$ST,format=bgr0,hwupload|$OV||opaque
pm-bgra-galpha|@PREPAD@,$PAD|$PM,format=bgra,hwupload|$OV:alpha=0.999||blend
straight-bgra-galpha|@PREPAD@,$PAD|$ST,format=bgra,hwupload|$OV:alpha=0.999||blend
pm-bgra-wh|@PREPAD@,$PAD|$PM,format=bgra,hwupload|$OV:w=$B:h=$B||blend
pm-bgra-nopad|@PREPAD@|$PM,format=bgra,hwupload|$OV||blend
pm-bgra-loop|@PREPAD@,$PAD|$PM:loop=0,setpts=N/(25*TB),format=bgra,hwupload|$OV||blend
pm-bgra-pool|@PREPAD@,$PAD|$PM,format=bgra,hwupload=extra_hw_frames=8|$OV||blend
pm-bgra-vpp|@PREPAD@,$PAD|$PM,format=bgra,hwupload,scale_vaapi=format=bgra|$OV||blend
straight-bgra-vpp|@PREPAD@,$PAD|$ST,format=bgra,hwupload,scale_vaapi=format=bgra|$OV||blend
REF-cpu|@PREPAD@,$PAD,hwdownload,format=nv12|$ST,format=yuva420p|overlay=x=$X:y=$Y|format=nv12,hwupload|blend
EOF
)"

# --- run -----------------------------------------------------------------------

join() { local IFS=,; local out=() p; for p in "$@"; do [ -n "$p" ] && out+=("$p"); done; echo "${out[*]}"; }

declare -A DRAWS=()
for class in sdr hdr; do
	src="$W/$class.mkv"
	[ -s "$src" ] || continue
	prepad="$SDR_PREPAD"
	[ "$class" = hdr ] && prepad="$HDR_PREPAD"
	echo
	echo "== $class"
	printf '%-22s %-14s %9s %9s %9s %9s  %s\n' candidate verdict bug want bg ctrlΔY SPS/PPS
	declare -A OFF=()
	while IFS='|' read -r name main bug blend after expect; do
		main="${main//@PREPAD@/$prepad}"
		off="$W/$class-off-$(printf '%s|%s' "$main" "$after" | cksum | cut -d' ' -f1).h264"
		if [ -z "${OFF[$off]:-}" ]; then
			if ! err="$(encode "$src" "$(join "$main" "$after" "$TAIL")" "$off")"; then
				printf '%-22s %-14s %s\n' "$name" FAILED-OFF "$err"
				[ "$name" = PROD ] && { echo "the bug-off production graph fails: $class class skipped"; break; }
				continue
			fi
			OFF[$off]=1
		fi
		on="$W/$class-$name.h264"
		if ! err="$(encode "$src" "${main}[main];${bug}[wm];[main][wm]$(join "$blend" "$after" "$TAIL")" "$on")"; then
			printf '%-22s %-14s %s\n' "$name" FAILED "$err"
			continue
		fi
		box="$((B - 2 * INSET)):$((B - 2 * INSET)):$((X + INSET)):$((Y + INSET))"
		ctrl="1600:1080:0:0"
		bug_on="$(yavg "$on" "$box")" bug_off="$(yavg "$off" "$box")"
		ctrl_on="$(yavg "$on" "$ctrl")" ctrl_off="$(yavg "$off" "$ctrl")"
		if diff -q <(params "$off") <(params "$on") >/dev/null; then
			sps=identical
		else
			sps="DIFFERS: $(diff <(params "$off") <(params "$on") | grep '^>' | head -3 | tr -s ' ' | tr '\n' ';')"
		fi
		read -r verdict want dctrl bug_on bug_off < <(awk -v on="$bug_on" -v bg="$bug_off" -v c1="$ctrl_on" -v c0="$ctrl_off" \
			-v a="$ALPHA" -v expect="$expect" -v sps="$sps" 'BEGIN {
				want = (expect == "opaque") ? 235 : a * 235 + (1 - a) * bg
				d = c1 - c0; if (d < 0) d = -d
				e = on - want; if (e < 0) e = -e
				g = on - bg; if (g < 0) g = -g
				if (on == "" || bg == "") v = "NO-MEASURE"
				else if (d > 3) v = "PICTURE-LOST"
				else if (g <= 1.5) v = "NOTHING-DRAWN"
				else if (e > 14) v = "WRONG-BLEND"
				else if (sps != "identical") v = "SPS-DIFFERS"
				else v = "DRAWS"
				printf "%s %.1f %.2f %.1f %.1f\n", v, want, d, on, bg
			}')
		printf '%-22s %-14s %9s %9s %9s %9s  %s\n' "$name" "$verdict" "$bug_on" "$want" "$bug_off" "$dctrl" "$sps"
		[ "$verdict" = DRAWS ] && [ "$name" != REF-cpu ] && [ "$expect" = blend ] && DRAWS[$name]+="$class "
	done <<<"$CANDIDATES"
	unset OFF
done

echo
echo "== candidates that draw a correct 65% blend (production-eligible; REF-cpu and the opaque control excluded)"
if [ "${#DRAWS[@]}" -eq 0 ]; then
	echo "none"
else
	for n in "${!DRAWS[@]}"; do echo "$n: ${DRAWS[$n]}"; done | sort
fi
echo
echo "== graphs (bug chain | blend), for reference"
while IFS='|' read -r name main bug blend after expect; do
	printf '%-22s main=%s | bug=%s | %s%s\n' "$name" "${main//@PREPAD@/<class scale>}" "${bug//$W\//}" "$blend" "${after:+ | after=$after}"
done <<<"$CANDIDATES"
