cmp_ref() { local p=$(ffmpeg -nostdin -i $1 -i /work/tmp/ref.mkv -filter_complex "[0:v]trim=end_frame=48,format=yuv420p10le[a];[a][1:v]psnr" -f null - 2>&1 | grep -o 'y:[0-9.inf]*' | tail -1)
  local y=$(ffmpeg -nostdin -i $1 -vf "trim=end_frame=48,signalstats,metadata=print:key=lavfi.signalstats.YAVG" -f null - 2>&1 | grep -o 'YAVG=[0-9.]*' | tail -1); echo "$p $y $(ffprobe -v error -show_entries stream=color_transfer,color_primaries -select_streams v:0 -of csv=p=0 $1)"; }
DISP="35400 14600|8500 39850|6550 2300|15635 16450|10000000 50"
for v in scale scale_noenctags tonemap; do : > ff.err; ENC=$H265H; [ $v = scale_noenctags ] && ENC="-c:v hevc_vaapi -profile:v main10 -rc_mode QVBR -global_quality 22 -b:v 16M -maxrate 24M"
  if [ $v != tonemap ]; then vf="$UP:out_color_matrix=bt2020nc:out_color_primaries=bt2020:out_color_transfer=smpte2084"; else vf="$UP,tonemap_vaapi=format=p010:t=smpte2084:p=bt2020:m=bt2020nc:display=$DISP:light=1000 400"; fi
  r=$(t1s $VA $IN -ss 30 -i sd.mkv -t 30 -map 0:v:0 -vf "$vf" $ENC $GOP $AUD $FMP4 -y /work/tmp/s2h_$v.mp4 | tr -s ' ' ',')
  echo "s2h_${v}_vaapi,$r | $(cmp_ref /work/tmp/s2h_$v.mp4)"; grep -vE 'PPS changed' ff.err | head -3 | cut -c1-200; done
