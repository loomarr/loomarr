# 2f: rate control on a HARD live-action sample ($SAMPLE_HARD: a grain-heavy 35 mm film remux, ~34 Mbps AVC,
# the highest-bitrate SDR 1080p remux tier in the library). Three 15 s windows at 25/50/80% of runtime.
# Each window is first cut to a lossless FFV1 reference; every mode encodes FROM that file and VMAF
# compares against the same file, so seek/decoder differences cannot misalign frames (the first attempt,
# The reference is decoded in software and uploaded (hwupload) so the encoder sees exactly the reference frames. -fps_mode passthrough: the mp4 muxer's default CFR sync shifted the encode by one frame
# Frames are paired by INDEX (setpts=N/FRAME_RATE/TB), not timestamp: the MKV reference has 1 ms timestamps
# and the MP4 1/24000, so libvmaf's timestamp framesync slipped a frame mid-window (VMAF min 0, and the best
# PSNR offset flipped between windows). Each row records Y-PSNR at offsets 0 and +1 as the alignment guard.
# seeking the source separately for encode and reference, gave VMAF min 0 and bitrate-independent scores).
cd /work; mkdir -p rc3 res
H="$SAMPLE_HARD"
D=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$H" | cut -d. -f1)
for pct in 25 50 80; do ffmpeg -nostdin -v error -y -ss $(( D * pct / 100 )) -i "$H" -t 15 -map 0:v:0 -c:v ffv1 -pix_fmt yuv420p rc3/ref-$pct.mkv; done
HW="-vaapi_device /dev/dri/renderD128"
VF="format=nv12,hwupload,scale_vaapi=w=1920:h=1080:format=nv12,setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv:chroma_location=left"
echo "mode,args,window_pct,mbps,vmaf_mean,vmaf_p5,vmaf_min,frames_enc,frames_ref,psnr_y_off0,psnr_y_off1" > res/rc2.csv
ps() { ffmpeg -nostdin -i "$1" -i "$2" -lavfi "[0:v]setpts=N/FRAME_RATE/TB,format=yuv420p[d];[1:v]trim=start_frame=$3,setpts=N/FRAME_RATE/TB,format=yuv420p[r];[d][r]psnr" -f null - 2>&1 | grep -o "PSNR y:[0-9.]*" | cut -d: -f2; } # alignment guard: offset 0 must beat offset 1
while IFS='|' read -r name rc; do for pct in 25 50 80; do R=rc3/ref-$pct.mkv
  ffmpeg -nostdin -loglevel error -y $HW -i $R -fps_mode passthrough -vf "$VF" -c:v h264_vaapi -profile:v high -level 4.1 -bf 0 -g 30 -sei 0 $rc -f mp4 rc3/$name-$pct.mp4
  mbps=$(awk -v b=$(stat -c %s rc3/$name-$pct.mp4) 'BEGIN {printf "%.2f", b*8/15/1e6}')
  ffmpeg -nostdin -loglevel error -i rc3/$name-$pct.mp4 -i $R -lavfi "[0:v]setpts=N/FRAME_RATE/TB,format=yuv420p[d];[1:v]setpts=N/FRAME_RATE/TB,format=yuv420p[r];[d][r]libvmaf=n_threads=4:log_fmt=csv:log_path=rc3/$name-$pct.csv" -f null - 2>/dev/null
  cnt() { ffprobe -v error -count_packets -select_streams v:0 -show_entries stream=nb_read_packets -of csv=p=0 "$1"; }
  stats=$(awk -F, 'NR==1{for(i=1;i<=NF;i++) if($i=="vmaf") c=i; next} {print $c}' rc3/$name-$pct.csv | sort -n | awk '{a[NR]=$1; s+=$1} END {printf "%.2f,%.2f,%.2f", s/NR, a[int(NR*0.05)+1], a[1]}')
  echo "$name,$rc,$pct,$mbps,$stats,$(cnt rc3/$name-$pct.mp4),$(cnt $R),$(ps rc3/$name-$pct.mp4 $R 0),$(ps rc3/$name-$pct.mp4 $R 1)" >> res/rc2.csv
done; done <<'M'
cbr5|-rc_mode CBR -b:v 5M
vbr5|-rc_mode VBR -b:v 5M -maxrate 10M
qvbr5|-rc_mode QVBR -b:v 5M -maxrate 10M -global_quality 22
cbr8|-rc_mode CBR -b:v 8M
vbr8|-rc_mode VBR -b:v 8M -maxrate 12M
qvbr8|-rc_mode QVBR -b:v 8M -maxrate 12M -global_quality 22
M
