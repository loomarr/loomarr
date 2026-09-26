# 2f: rate control on a HARD live-action sample ($SAMPLE_HARD: the highest-bitrate SDR 1080p remux in the
# library, i.e. the most grain/motion the encoder will see). Three 15 s windows at 25/50/80% of runtime.
cd /work; mkdir -p rc2 res
H="$SAMPLE_HARD"
D=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$H" | cut -d. -f1)
HW="-hwaccel vaapi -hwaccel_device /dev/dri/renderD128 -hwaccel_output_format vaapi"
VF="scale_vaapi=w=1920:h=1080:force_original_aspect_ratio=decrease:force_divisible_by=2:format=nv12,pad_vaapi=w=1920:h=1080,setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv:chroma_location=left"
echo "mode,args,window_s,mbps,vmaf_mean,vmaf_p5,vmaf_min" > res/rc2.csv
while IFS='|' read -r name rc; do for pct in 25 50 80; do ss=$(( D * pct / 100 ))
  ffmpeg -nostdin -loglevel error -y $HW -ss $ss -t 15 -i "$H" -map 0:v:0 -vf "$VF" -c:v h264_vaapi -profile:v high -level 4.1 -bf 0 -g 30 -sei 0 $rc -f mp4 rc2/$name-$pct.mp4
  mbps=$(awk -v b=$(stat -c %s rc2/$name-$pct.mp4) 'BEGIN {printf "%.2f", b*8/15/1e6}')
  ffmpeg -nostdin -loglevel error -i rc2/$name-$pct.mp4 -ss $ss -t 15 -i "$H" -lavfi "[0:v]setpts=PTS-STARTPTS,format=yuv420p[d];[1:v]setpts=PTS-STARTPTS,scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:-1:-1,format=yuv420p[r];[d][r]libvmaf=n_threads=4:log_fmt=csv:log_path=rc2/$name-$pct.csv" -f null -
  stats=$(awk -F, 'NR==1{for(i=1;i<=NF;i++) if($i=="vmaf") c=i; next} {print $c}' rc2/$name-$pct.csv | sort -n | awk '{a[NR]=$1; s+=$1} END {printf "%.2f,%.2f,%.2f", s/NR, a[int(NR*0.05)+1], a[1]}')
  echo "$name,$rc,$ss,$mbps,$stats" >> res/rc2.csv
done; done <<'M'
cbr5|-rc_mode CBR -b:v 5M
vbr5|-rc_mode VBR -b:v 5M -maxrate 10M
qvbr5|-rc_mode QVBR -b:v 5M -maxrate 10M -global_quality 22
cbr8|-rc_mode CBR -b:v 8M
vbr8|-rc_mode VBR -b:v 8M -maxrate 12M
qvbr8|-rc_mode QVBR -b:v 8M -maxrate 12M -global_quality 22
M
