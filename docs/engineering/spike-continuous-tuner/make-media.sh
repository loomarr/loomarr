#!/usr/bin/env bash
# SPIKE (#1460): synthesises two "channels" packaged exactly like prepared v3
# (internal/prepared/ffmpeg.go: -bf 0, 200 ms forced keyframes, aac, 2 s fmp4 HLS), then a
# 200 ms-fragment copy-remux of each (the splice unit), plus a codec-change control (480p).
# Output goes to $OUT (default ./out, git-ignored). Needs ffmpeg + ffprobe.
set -euo pipefail
OUT=${OUT:-out}; mkdir -p "$OUT"; cd "$OUT"
DUR=${DUR:-12}
pack() { # name videosrc audiofreq w h tsoffset
  local n=$1 vs=$2 af=$3 w=$4 h=$5 off=$6 prof=high lvl=4.1
  mkdir -p "$n"
  ffmpeg -v error -y -f lavfi -i "$vs=size=${w}x${h}:rate=30" -f lavfi -i "sine=frequency=$af:sample_rate=48000" -t "$DUR" \
    -vf "scale=$w:$h:force_original_aspect_ratio=decrease,pad=$w:$h:(ow-iw)/2:(oh-ih)/2,setsar=1" \
    -c:v libx264 -profile:v $prof -level:v $lvl -pix_fmt yuv420p -r 30 -b:v 3000k -maxrate 6000k -bufsize 6000k \
    -g 6 -keyint_min 6 -force_key_frames "expr:gte(t,n_forced*0.200)" -sc_threshold 0 -bf 0 \
    -c:a aac -b:a 128k -ac 2 \
    -f hls -hls_time 2.000 -hls_playlist_type vod -hls_flags independent_segments \
    -hls_segment_type fmp4 -hls_fmp4_init_filename init.mp4 -hls_segment_filename "$n/segment-%06d.m4s" "$n/media.m3u8"
  # splice unit: copy-only re-fragmentation, one moof per video access point (<= 200 ms)
  ffmpeg -v error -y -i "$n/media.m3u8" -c copy -movflags +frag_keyframe+empty_moov+default_base_moof+omit_tfhd_offset "$n.frag.mp4"
}
pack A testsrc2 440 1280 720 0
pack B rgbtestsrc 880 1280 720 0
pack C rgbtestsrc 660 854 480 0
for f in A B C; do ffprobe -v error -show_entries stream=codec_name,profile,width,height,start_time,time_base -of compact "$f.frag.mp4"; done
