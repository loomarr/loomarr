#!/usr/bin/env bash
# Burns each bug onto real programme excerpts through the production NVENC 1080p graph (playout.Build,
# printed into graphs.env) with overlay_cuda, then tiles sample sheets. Throwaway spike code (#1512 1d).
#
#   W=<work dir> burn.sh    W holds: graphs.env, src/{bright,dark,busy,hdr}.mkv (30 s stream-copy
#                           excerpts, never committed), logos/*.png (TMDB), fonts/*.ttf.
# Output: $W/sheets/*.png. Every GPU encode runs under flock /tmp/loomarr-gpu.lock.
set -euo pipefail
W=${W:?}; HERE=$(cd "$(dirname "$0")" && pwd); cd "$W"; source graphs.env
mkdir -p bugs out frames sheets

# overlay_cuda blends alpha only onto a yuv420p main (yuva420p overlay); an nv12 main takes an nv12
# overlay, which has no alpha. So the bug-on graph asks scale_cuda for yuv420p instead of nv12.
# overlay_cuda then emits the decoder's aligned 1920x1088 surface (8 garbage rows, SPS without
# cropping), so a non-passthrough scale_cuda relabels it 1920x1080. The SPS is then byte-identical to
# the bug-off graph's (programmes and breaks share one SPS). A format=nv12 post-scale also works on
# ffmpeg n9 but faults (CUDA_ERROR_ILLEGAL_ADDRESS) on the image's n8.1.2.
# ⚠ On the image's n8.1.2, overlay_cuda drops the main picture (all-green output, exit 0) whenever the
# process decodes on NVDEC — every production NVENC graph. These sheets are native n9.0.2. See the note.
pre_sdr=${sdr_VF%%,fps=*}; pre_sdr=${pre_sdr/format=nv12,pad_cuda/format=yuv420p,pad_cuda}
pre_hdr=${hdr_VF%%,fps=*}; pre_hdr=${pre_hdr/hwupload_cuda,pad_cuda/hwupload_cuda,scale_cuda=format=yuv420p,pad_cuda}
tail="scale_cuda=w=1920:h=1080:format=yuv420p:passthrough=0,fps=${sdr_VF#*,fps=}"
MX=96 MY=54 # 5% of 1920 and of 1080: EBU R95 graphics-safe

SOURCES=(hbo:logo:logos/hbo.png nbc:logo:logos/nbc.png nick:logo:logos/nick.png nickw:logo:logos/nickw.png
  plate:plate:RETRO mono:monogram:N stack:stacked:MIDNIGHT/HORROR tag:tag:42/KIDS)
# variant: name size opacity shadow
VARIANTS=("def 0.06 0.65 0" "alt 0.045 0.85 0" "shd 0.06 0.65 1")
SCENES=(bright dark busy hdr)

for s in "${SOURCES[@]}"; do n=${s%%:*}; src=${s#*:}
  for v in "${VARIANTS[@]}"; do read -r vn size op sh <<<"$v"
    python3 "$HERE/render_bugs.py" "$src" bugs/${n}_$vn.png --size $size --opacity $op --fonts fonts $([ $sh = 1 ] && echo --shadow)
  done
done

burn() { # scene bug.png x y out.mkv
  local pre=$sdr_PRE vf=$pre_sdr
  [ $1 = hdr ] && pre=$hdr_PRE vf=$pre_hdr
  ffmpeg -nostdin -hide_banner -v error $pre -i src/$1.mkv -i $2 -filter_complex \
    "[0:v:0]$vf[m];[1:v]format=yuva420p,hwupload_cuda[b];[m][b]overlay_cuda=x=$3:y=$4,$tail[v]" \
    -map "[v]" $sdr_ENC -t 2.5 -y $5
}
export -f burn; export sdr_PRE hdr_PRE pre_sdr pre_hdr tail sdr_ENC

flock /tmp/loomarr-gpu.lock bash -c '
for s in '"${SOURCES[*]%%:*}"'; do for v in def alt shd; do
  b=bugs/${s%%:*}_$v.png; w=$(magick identify -format %w $b); h=$(magick identify -format %h $b)
  pad=0; [ $v = shd ] && pad=$(( (w - $(magick identify -format %w bugs/${s%%:*}_def.png)) / 2 ))
  for sc in '"${SCENES[*]}"'; do burn $sc $b $((1920 - w - '$MX' + pad)) $(('$MY' - pad)) out/${s%%:*}_${v}_$sc.mkv; done
done; done'

for f in out/*.mkv; do ffmpeg -nostdin -v error -ss 2 -i $f -frames:v 1 -y frames/$(basename ${f%.mkv}).png; done

FONT=DejaVu-Sans
label() { magick -size ${2}x44 xc:'#111' -font $FONT -pointsize 22 -fill '#ddd' -gravity west -annotate +14+0 "$1" png:-; }
magick <(label "full frame (default)" 640) <(label "1:1 top-right: default 6% / 65%" 640) \
  <(label "1:1: alt 4.5% / 85%" 640) <(label "1:1: default + soft shadow" 640) +append out/header.png
for s in "${SOURCES[@]}"; do n=${s%%:*}; rows=()
  for sc in "${SCENES[@]}"; do
    magick \( frames/${n}_def_$sc.png -resize 640x360 \) \( frames/${n}_def_$sc.png -crop 640x360+1280+0 \) \
      \( frames/${n}_alt_$sc.png -crop 640x360+1280+0 \) \( frames/${n}_shd_$sc.png -crop 640x360+1280+0 \) \
      +repage +append -bordercolor '#111' -border 0x2 out/row_${n}_$sc.png
    rows+=(out/row_${n}_$sc.png)
  done
  magick <(label "$n  —  rows: bright / dark / busy / 4K HDR→SDR (tonemap_opencl hable)   ·   NVENC h264 1080p, overlay_cuda" 2560) \
    out/header.png "${rows[@]}" -append sheets/$n.png
done
# overview: every source (rows) × every scene (columns), default setting, 1:1 top-right crops
rows=()
for s in "${SOURCES[@]}"; do n=${s%%:*}
  magick $(for sc in "${SCENES[@]}"; do echo "( frames/${n}_def_$sc.png -crop 480x200+1440+0 +repage )"; done) +append \
    -gravity northwest -background '#111' -splice 110x0 -font $FONT -pointsize 22 -fill '#ddd' -annotate +10+85 "$n" out/ov_$n.png
  rows+=(out/ov_$n.png)
done
magick "${rows[@]}" -append sheets/overview_default.png
echo "sheets: $(ls sheets)"
