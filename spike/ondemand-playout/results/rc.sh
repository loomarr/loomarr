cd /work; mkdir -p rc
H="$SAMPLE_HEVC_1080P"
HW="-hwaccel vaapi -hwaccel_device /dev/dri/renderD128 -hwaccel_output_format vaapi"
VF="scale_vaapi=w=1920:h=1080:format=nv12,setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv:chroma_location=left"
echo "mode,args,mbps,vmaf_mean,vmaf_p5"
while IFS='|' read -r name rc; do
  ffmpeg -nostdin -loglevel error -y $HW -ss 300 -t 20 -i "$H" -map 0:v:0 -vf "$VF" -c:v h264_vaapi -profile:v high -level 4.1 -bf 0 -g 30 -sei 0 $rc -f mp4 rc/$name.mp4
  mbps=$(echo "$(stat -c %s rc/$name.mp4) * 8 / 20 / 1000000" | bc -l | cut -c1-5)
  ffmpeg -nostdin -loglevel error -i rc/$name.mp4 -ss 300 -t 20 -i "$H" -lavfi "[0:v]setpts=PTS-STARTPTS,format=yuv420p[d];[1:v]setpts=PTS-STARTPTS,format=yuv420p[r];[d][r]libvmaf=n_threads=4:log_fmt=csv:log_path=rc/$name.csv" -f null -
  stats=$(awk -F, 'NR==1{for(i=1;i<=NF;i++) if($i=="vmaf") c=i; next} {print $c}' rc/$name.csv | sort -n | awk '{a[NR]=$1; s+=$1} END {printf "%.2f,%.2f", s/NR, a[int(NR*0.05)+1]}')
  echo "$name,$rc,$mbps,$stats"
done <<'M'
cbr5|-rc_mode CBR -b:v 5M
vbr5|-rc_mode VBR -b:v 5M -maxrate 10M
qvbr5|-rc_mode QVBR -b:v 5M -maxrate 10M -global_quality 22
qvbr8|-rc_mode QVBR -b:v 8M -maxrate 12M -global_quality 22
icq22|-rc_mode ICQ -global_quality 22
icq26|-rc_mode ICQ -global_quality 26
M
