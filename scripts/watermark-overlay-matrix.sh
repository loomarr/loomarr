#!/usr/bin/env bash
# watermark-overlay-matrix.sh — which GPU overlay graph draws the channel bug correctly on THIS GPU
# (#1512 phase 1d, #1541).
#
# WHY THIS EXISTS. Whether a GPU overlay blends the bug right is a hardware question: on the
# household Intel Arc a premultiplied bgra bug blended as straight alpha (129 where 65% white is
# 178), and the straight bug then drew its white at full-range 255 instead of limited-range 235
# (#1541). Speed and exit codes prove nothing, so this script measures a matrix of candidate graphs
# through the production decode, filters and encoder, each changing ONE thing from production.
#
# overlay_vaapi hands the bug layer the MAIN frame's colour labels (ffmpeg memcpys the main's
# VAProcPipelineParameterBuffer into the blend's), and an untagged SDR file and a tone-mapped HDR
# programme reach the blend labelled differently. So each candidate says whether the main is
# labelled like the output (tag) or left as the source left it (-), and the "labels" section
# prints what each class's main carries into the blend and, on VAAPI, what overlay_vaapi mapped
# that to for the driver. Measured on a GeForce with the self-check's fixtures: the SDR main is
# range tv with no matrix, primaries or transfer; the tone-mapped HDR main is tv and bt709.
# If the labels are what switches the Arc's RGB conversion, a VAAPI run shows: PROD DRAWS in both
# classes; tagged-full reads FULL-RANGE in both; untagged-limited reads WRONG-WHITE (≈218) in SDR
# only; untagged-full DRAWS in SDR only (the beta.7 graph).
#
# For every candidate and programme class (SDR H.264; HDR10 HEVC tone-mapped by tonemap_opencl when
# the host has an OpenCL runtime for the GPU) it encodes 25 frames bug-off and bug-on and prints:
#   - bug luma vs the expected blend (65% of limited-range white 235 over the measured background,
#     or 235 itself for the opaque controls) and the background;
#   - alpha and white: the bug sits on a horizontal luma ramp, so a per-pixel regression of bug-on
#     against bug-off luma (on = (1-alpha)·off + alpha·white) separates a wrong alpha from a wrong
#     white. White ≈255 is a full-range conversion (FULL-RANGE), ≈235 is right;
#   - control ΔY: mean luma change of the picture left of the bug (the programme must survive);
#   - SPS/PPS: identical to bug-off, or the differing fields (breaks air bug-off in the same stream).
# REF-cpu blends on the CPU; it is NOT a production candidate (the bug is never drawn on the CPU),
# it only proves the measurement reads a correct blend as DRAWS.
#
# It writes only to its own scratch directory. On an Intel/AMD host, in a throwaway container of
# the image under test (an image built from main has Intel's OpenCL runtime, so HDR runs too):
#
#   docker run --rm --device /dev/dri --group-add "$(stat -c %g /dev/dri/renderD128)" \
#     -v "$PWD/scripts/watermark-overlay-matrix.sh:/matrix.sh:ro" --entrypoint bash <image> /matrix.sh
#
# On an NVIDIA host: FAMILY=nvenc scripts/watermark-overlay-matrix.sh (or the container with the
# NVIDIA runtime).
#
# Env (optional): FAMILY (vaapi, the default, or nvenc), RENDER_NODE (default /dev/dri/renderD128),
# FFMPEG (default ffmpeg), KEEP=1 keeps the scratch directory, SELFTEST=1 (vaapi only) swaps every
# GPU stage for its CPU twin so the script's own plumbing can be checked without a GPU (its
# verdicts say nothing about any GPU).

set -u

FAMILY="${FAMILY:-vaapi}"
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

TAIL="fps=25,setsar=1,setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv:chroma_location=left,sidedata=mode=delete:type=MASTERING_DISPLAY_METADATA,sidedata=mode=delete:type=CONTENT_LIGHT_LEVEL"
FIT="w=1920:h=1080:force_original_aspect_ratio=decrease:force_divisible_by=2"
TONEMAP="tonemap_opencl=tonemap=hable:desat=0:t=bt709:m=bt709:p=bt709:r=tv:format=nv12"
# LIMITED maps the bug's full-range RGB into limited range (0→16, 255→235), for a GPU conversion
# that carries RGB values straight into Y.
LIMITED="lutrgb=r=16+val*219/255:g=16+val*219/255:b=16+val*219/255"
# CONFORM labels the main like the output, before the blend (production's VAAPI graph, #1541).
CONFORM="setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv:chroma_location=left"
ST="movie=filename=$W/bug.png"

# Per family: the production decode, bug-off and bug-on main chains, the bug's blend and what
# follows it, and the candidates. Rows: name | main label (tag: CONFORM, -: as decoded) | bug chain |
# blend | expect (blend|opaque).
case "$FAMILY" in
vaapi)
	PRE_SDR=(-init_hw_device "vaapi=va:$NODE" -filter_hw_device va -hwaccel vaapi -hwaccel_device va -hwaccel_output_format vaapi)
	PRE_HDR=("${PRE_SDR[@]}")
	ENC=(-c:v h264_vaapi -profile:v high -rc_mode QVBR -global_quality 22 -b:v 8000k -maxrate 12000k -g 25 -bf 0 -sei 0)
	PAD="pad_vaapi=w=1920:h=1080:x=(ow-iw)/2:y=(oh-ih)/2"
	SDR_OFF="scale_vaapi=$FIT:format=nv12,$PAD" SDR_MAIN="$SDR_OFF"
	HDR_OFF="scale_vaapi=$FIT:format=p010,hwmap=derive_device=opencl,$TONEMAP,hwmap=derive_device=vaapi:reverse=1,$PAD"
	HDR_MAIN="$HDR_OFF"
	OV="overlay_vaapi=x=$X:y=$Y" AFTER=""
	CANDIDATES="$(
		cat <<EOF
PROD|tag|$ST,format=bgra,$LIMITED,hwupload|$OV|blend
untagged-limited|-|$ST,format=bgra,$LIMITED,hwupload|$OV|blend
untagged-full|-|$ST,format=bgra,hwupload|$OV|blend
tagged-full|tag|$ST,format=bgra,hwupload|$OV|blend
opaque-limited-bgr0|tag|$ST,format=bgr0,$LIMITED,hwupload|$OV|opaque
opaque-bgr0|-|$ST,format=bgr0,hwupload|$OV|opaque
REF-cpu|-|$ST,format=yuva420p|overlay=x=$X:y=$Y|blend
EOF
	)"
	REF_MAIN_TAIL="hwdownload,format=nv12" REF_AFTER="format=nv12,hwupload"
	;;
nvenc)
	PRE_SDR=(-init_hw_device cuda=cu:0 -filter_hw_device cu -hwaccel cuda -hwaccel_device cu -hwaccel_output_format cuda)
	PRE_HDR=(-init_hw_device cuda=cu:0 -init_hw_device opencl=ocl -filter_hw_device ocl -hwaccel cuda -hwaccel_device cu -hwaccel_output_format cuda)
	ENC=(-c:v h264_nvenc -preset p4 -tune ll -profile:v high -rc vbr -cq 22 -b:v 8000k -maxrate 12000k -bufsize 12000k -g 25 -bf 0 -forced-idr 1 -strict_gop 1 -no-scenecut 1)
	PAD="pad_cuda=w=1920:h=1080:x=-1:y=-1"
	SDR_OFF="scale_cuda=$FIT:format=nv12,$PAD" SDR_MAIN="scale_cuda=$FIT:format=yuv420p,$PAD"
	HDR_TM="scale_cuda=$FIT:format=p010le,hwdownload,format=p010le,hwupload,$TONEMAP,hwdownload,format=nv12,hwupload_cuda"
	HDR_OFF="$HDR_TM,$PAD" HDR_MAIN="$HDR_TM,scale_cuda=format=yuv420p,$PAD"
	OV="overlay_cuda=x=$X:y=$Y" AFTER="scale_cuda=w=1920:h=1080:format=yuv420p:passthrough=0"
	CANDIDATES="$(
		cat <<EOF
PROD|-|$ST,format=yuva420p,hwupload_cuda|$OV|blend
tagged|tag|$ST,format=yuva420p,hwupload_cuda|$OV|blend
limited-yuva|-|$ST,format=bgra,$LIMITED,format=yuva420p,hwupload_cuda|$OV|blend
opaque-yuv420p|-|$ST,format=yuv420p,hwupload_cuda|$OV|opaque
EOF
	)"
	# No REF-cpu row: hwdownload refuses the NVDEC-decoded frames after pad_cuda. The opaque control
	# (white 235.0, alpha 1.000 on a GeForce) anchors the measurement instead.
	REF_MAIN_TAIL="" REF_AFTER=""
	;;
*)
	echo "FAMILY must be vaapi or nvenc" >&2
	exit 2
	;;
esac

# cpu_twin GRAPH → the vaapi graph with every GPU stage replaced by its CPU equivalent (SELFTEST).
cpu_twin() {
	sed -E -e 's/scale_vaapi=[^,;[]*/scale=1920:1080:force_original_aspect_ratio=decrease,format=nv12/g' \
		-e 's/,hwmap=derive_device=opencl,tonemap_opencl=[^,]*,hwmap=derive_device=vaapi:reverse=1/,zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,format=nv12/' \
		-e 's/pad_vaapi=w=([0-9]+):h=([0-9]+):x=([^:]*):y=([^,;[]*)/pad=\1:\2:\3:\4/g' \
		-e 's/overlay_vaapi=x=([0-9]+):y=([0-9]+)[^,;[]*/overlay=x=\1:y=\2/g' \
		-e 's/,hwupload//g; s/,hwdownload,format=nv12//g' <<<"$1"
}
if [ "${SELFTEST:-}" = 1 ]; then
	[ "$FAMILY" = vaapi ] || { echo "SELFTEST covers FAMILY=vaapi only" >&2; exit 2; }
	PRE_SDR=() PRE_HDR=()
	ENC=(-c:v libx264 -preset ultrafast -pix_fmt yuv420p -g 25 -bf 0)
	echo "SELFTEST: CPU stand-ins for every GPU stage; these verdicts say nothing about a GPU."
	echo "SELFTEST: PROD reads WRONG-WHITE here by design: the CPU overlay converts range itself, so the"
	echo "          limited-range mapping the Arc needs is applied twice (white ≈218)."
fi

# encode CLASS SRC VF OUT → 25 frames through the production encoder; prints ffmpeg's first error.
encode() {
	local vf="$3" pre=("${PRE_SDR[@]}")
	[ "$1" = hdr ] && pre=("${PRE_HDR[@]}")
	[ "${SELFTEST:-}" = 1 ] && vf="$(cpu_twin "$vf")"
	"$FF" -hide_banner -nostdin -loglevel error -y "${pre[@]}" -i "$2" -map 0:v:0 -frames:v 25 \
		-vf "$vf" "${ENC[@]}" -f h264 "$4" 2>"$4.err" || { head -1 "$4.err"; return 1; }
}

# yavg FILE CROP → mean luma of frame $FRAME inside crop w:h:x:y.
yavg() {
	"$FF" -hide_banner -nostdin -loglevel error -i "$1" \
		-vf "select=eq(n\,$FRAME),crop=$2,signalstats,metadata=print:key=lavfi.signalstats.YAVG:file=-" \
		-frames:v 1 -f null - 2>/dev/null | sed -n 's/^lavfi.signalstats.YAVG=//p' | head -1
}

# luma FILE CROP → frame $FRAME's coded luma inside crop w:h:x:y, one value per line. The Y plane
# of yuv420p, never -pix_fmt gray: swscale converts to full-range grey instead of copying Y (a
# neutral Y 171 reads 180), which would put the measurement in the wrong space.
luma() {
	local w="${2%%:*}" rest="${2#*:}"
	"$FF" -hide_banner -nostdin -loglevel error -i "$1" -vf "select=eq(n\,$FRAME),crop=$2" \
		-frames:v 1 -f rawvideo -pix_fmt yuv420p - 2>/dev/null | head -c "$((w * ${rest%%:*}))" | od -An -v -tu1 -w1
}

# labels CLASS SRC MAIN → the colour labels frame 0 carries out of the nv12 main chain MAIN (its
# trailing pad dropped: pad only copies them, and hwdownload refuses NVDEC frames after pad_cuda).
labels() {
	local vf="${3%,"$PAD"},hwdownload,format=nv12" pre=("${PRE_SDR[@]}")
	[ "$1" = hdr ] && pre=("${PRE_HDR[@]}")
	[ "${SELFTEST:-}" = 1 ] && vf="$(cpu_twin "$vf")"
	"$FF" -hide_banner -nostdin -y "${pre[@]}" -i "$2" -map 0:v:0 -frames:v 1 -vf "$vf,showinfo" -f null - 2>&1 |
		grep -o -m1 'color_range:[a-z]* color_space:[a-z0-9-]* color_primaries:[a-z0-9-]* color_trc:[a-z0-9-]*' || echo "no showinfo line"
}

# vppmap CLASS SRC GRAPH → what overlay_vaapi told the driver for the main (input) and its output:
# ffmpeg's debug "Mapped colour properties" lines, which the bug layer inherits.
vppmap() {
	local pre=("${PRE_SDR[@]}")
	[ "$1" = hdr ] && pre=("${PRE_HDR[@]}")
	"$FF" -hide_banner -nostdin -loglevel debug -y "${pre[@]}" -i "$2" -map 0:v:0 -frames:v 1 -vf "$3" "${ENC[@]}" -f null - 2>&1 |
		grep 'overlay_vaapi.*Mapped colour properties' | head -2 | sed -E 's/^\[[^]]*\] /    /'
}

# params FILE → the first SPS and PPS, field by field (trace_headers, addresses stripped).
params() {
	"$FF" -hide_banner -nostdin -loglevel trace -i "$1" -map 0:v:0 -c copy -bsf:v trace_headers \
		-frames:v 1 -f null - 2>&1 | grep '^\[trace_headers' | sed -E 's/^\[trace_headers @ [^]]*\] //' | grep -v '^Packet:' |
		awk '/Sequence Parameter Set|Picture Parameter Set/{on=1} /Slice Header|Supplemental Enhancement|Access Unit Delimiter/{if(on)exit} on'
}

# --- fixtures ----------------------------------------------------------------

echo "== host (FAMILY=$FAMILY)"
"$FF" -hide_banner -version | head -1
if [ "${SELFTEST:-}" != 1 ] && [ "$FAMILY" = vaapi ]; then
	"$FF" -hide_banner -nostdin -v verbose -init_hw_device "vaapi=va:$NODE" -f lavfi -i nullsrc=s=64x64 \
		-frames:v 1 -f null - 2>&1 | grep -i -m2 "driver\|VAAPI" | sed -E 's/^\[[^]]*\] //'
fi

# A horizontal luma ramp (Y 40→200) under the bug: the regression needs varied background, and a
# dark-to-mid range keeps the outcomes apart (nothing drawn, a wrong alpha, a full-range white).
RAMP="format=yuv420p,geq=lum='if(between(X\,1700\,1919)*between(Y\,0\,199)\,40+160*(X-1700)/219\,lum(X\,Y))':cb='if(between(X/SW\,1700\,1919)*between(Y/SH\,0\,199)\,128\,cb(X\,Y))':cr='if(between(X/SW\,1700\,1919)*between(Y/SH\,0\,199)\,128\,cr(X\,Y))'"

# The test bug: a 64x64 white square at 65% alpha, straight.
if ! "$FF" -hide_banner -nostdin -loglevel error -y -f lavfi -i "color=c=0xFFFFFFA6:s=${B}x${B},format=rgba" -frames:v 1 "$W/bug.png"; then
	echo "cannot write the test bug"
	exit 1
fi

"$FF" -hide_banner -nostdin -loglevel error -y -f lavfi -i "testsrc2=size=1920x1080:rate=25:duration=1.2" -vf "$RAMP" \
	-c:v libx264 -preset ultrafast -pix_fmt yuv420p -g 25 "$W/sdr.mkv" || { echo "cannot write the SDR fixture"; exit 1; }
"$FF" -hide_banner -nostdin -loglevel error -y -f lavfi -i "testsrc2=size=1920x1080:rate=25:duration=1.2" \
	-vf "$RAMP,zscale=tin=bt709:min=bt709:pin=bt709:rin=tv:t=smpte2084:p=bt2020:m=bt2020nc:r=tv:npl=203,format=yuv420p10le" \
	-c:v libx265 -preset ultrafast -x265-params "log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc" \
	-color_primaries bt2020 -color_trc smpte2084 -colorspace bt2020nc "$W/hdr.mkv" || echo "no HDR fixture (libx265/zscale missing): HDR class skipped"

# --- run -----------------------------------------------------------------------

join() { local IFS=,; local out=() p; for p in "$@"; do [ -n "$p" ] && out+=("$p"); done; echo "${out[*]}"; }

box="$((B - 2 * INSET)):$((B - 2 * INSET)):$((X + INSET)):$((Y + INSET))"
ctrl="1600:1080:0:0"
declare -A DRAWS=()
for class in sdr hdr; do
	src="$W/$class.mkv"
	[ -s "$src" ] || continue
	off_main="$SDR_OFF" on_main="$SDR_MAIN"
	[ "$class" = hdr ] && off_main="$HDR_OFF" on_main="$HDR_MAIN"
	echo
	echo "== $class"
	off="$W/$class-off.h264"
	if ! err="$(encode "$class" "$src" "$(join "$off_main" "$TAIL")" "$off")"; then
		echo "the bug-off production graph fails ($err): $class class skipped"
		continue
	fi
	echo "labels the main carries into the blend: $(labels "$class" "$src" "$off_main")"
	if [ "$FAMILY" = vaapi ] && [ "${SELFTEST:-}" != 1 ]; then
		for tag in - tag; do
			m="$on_main"
			[ "$tag" = tag ] && m="$on_main,$CONFORM"
			echo "  overlay_vaapi's mapping, main $([ "$tag" = tag ] && echo "labelled like the output" || echo "as decoded") (input, then output):"
			vppmap "$class" "$src" "${m}[main];$ST,format=bgra,hwupload[wm];[main][wm]$OV"
		done
	fi
	printf '%-20s %-13s %7s %7s %7s %6s %6s %7s  %s\n' candidate verdict bug want bg alpha white ctrlΔY SPS/PPS
	while IFS='|' read -r name tag bug blend expect; do
		main="$on_main" after="$AFTER"
		[ "$tag" = tag ] && main="$on_main,$CONFORM"
		if [ "$name" = REF-cpu ]; then
			main="$on_main,$REF_MAIN_TAIL" after="$(join "$REF_AFTER" "$AFTER")"
		fi
		on="$W/$class-$name.h264"
		if ! err="$(encode "$class" "$src" "${main}[main];${bug}[wm];[main][wm]$(join "$blend" "$after" "$TAIL")" "$on")"; then
			printf '%-20s %-13s %s\n' "$name" FAILED "$err"
			continue
		fi
		ctrl_on="$(yavg "$on" "$ctrl")" ctrl_off="$(yavg "$off" "$ctrl")"
		if diff -q <(params "$off") <(params "$on") >/dev/null; then
			sps=identical
		else
			sps="DIFFERS: $(diff <(params "$off") <(params "$on") | grep '^>' | head -3 | tr -s ' ' | tr '\n' ';')"
		fi
		read -r verdict bug_on want bug_off alpha white dctrl < <(paste <(luma "$off" "$box") <(luma "$on" "$box") |
			awk -v c1="$ctrl_on" -v c0="$ctrl_off" -v a="$ALPHA" -v expect="$expect" -v sps="$sps" '
				{ o = $1; y = $2; n++; so += o; sy += y; soo += o * o; soy += o * y }
				END {
					if (n == 0 || c1 == "" || c0 == "") { print "NO-MEASURE - - - - - -"; exit }
					bg = so / n; on = sy / n
					va = soo / n - bg * bg
					al = "n/a"; wh = "n/a"
					if (va >= 4) {
						slope = (soy / n - bg * on) / va
						al = 1 - slope
						if (al > 0.05) wh = (on - slope * bg) / al
					}
					wantA = (expect == "opaque") ? 1 : a
					want = wantA * 235 + (1 - wantA) * bg
					d = c1 - c0; if (d < 0) d = -d
					g = on - bg; if (g < 0) g = -g
					e = on - want; if (e < 0) e = -e
					if (d > 3) v = "PICTURE-LOST"
					else if (g <= 1.5) v = "NOTHING-DRAWN"
					else if (al != "n/a" && (al - wantA > 0.05 || wantA - al > 0.05)) v = "WRONG-ALPHA"
					else if (wh != "n/a" && wh > 245) v = "FULL-RANGE"
					else if (wh != "n/a" && (wh < 229 || wh > 241)) v = "WRONG-WHITE"
					else if (e > 6) v = "WRONG-BLEND"
					else if (sps != "identical") v = "SPS-DIFFERS"
					else v = "DRAWS"
					printf "%s %.1f %.1f %.1f %s %s %.2f\n", v, on, want, bg,
						(al == "n/a") ? al : sprintf("%.3f", al), (wh == "n/a") ? wh : sprintf("%.1f", wh), d
				}')
		printf '%-20s %-13s %7s %7s %7s %6s %6s %7s  %s\n' "$name" "$verdict" "$bug_on" "$want" "$bug_off" "$alpha" "$white" "$dctrl" "$sps"
		[ "$verdict" = DRAWS ] && [ "$name" != REF-cpu ] && [ "$expect" = blend ] && DRAWS[$name]+="$class "
	done <<<"$CANDIDATES"
done

echo
echo "== candidates that draw a correct blend: alpha 0.651±0.05, white 235±6, luma within 6 (REF-cpu and opaque controls excluded)"
if [ "${#DRAWS[@]}" -eq 0 ]; then
	echo "none"
else
	for n in "${!DRAWS[@]}"; do echo "$n: ${DRAWS[$n]}"; done | sort
fi
echo
echo "== graphs (main label | bug chain | blend), for reference; tag = ,$CONFORM"
while IFS='|' read -r name tag bug blend expect; do
	printf '%-20s %-3s | bug=%s | %s\n' "$name" "$tag" "${bug//$W\//}" "$blend"
done <<<"$CANDIDATES"
