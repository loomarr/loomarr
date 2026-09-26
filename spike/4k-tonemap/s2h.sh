# reference: CPU zscale, SDR white → 203 nits in PQ (BT.2408)
ffmpeg -nostdin -v error -ss 30 -i sd.mkv -frames:v 48 -vf "zscale=w=3840:h=2160:f=spline36,zscale=t=linear:npl=203:pin=709:tin=709:min=709,format=gbrpf32le,zscale=p=2020:t=smpte2084:m=2020_ncl:r=tv,format=yuv420p10le" -c:v ffv1 -y /work/tmp/ref.mkv
echo "graph,first1s_ms,real_s,user_s,sys_s | psnr_y vs ref | YAVG out/ref"
for t in s2h_scale_vaapi s2h_tonemap_vaapi s2h_pl_hop s2h_pl_hop_itm; do : > ff.err
  r=$(tm_run $t sd 30 30 /work/tmp/$t.mp4)
  p=$(ffmpeg -nostdin -v error -i /work/tmp/$t.mp4 -i /work/tmp/ref.mkv -filter_complex "[0:v]trim=end_frame=48,format=yuv420p10le[a];[a][1:v]psnr" -f null - 2>&1; ffmpeg -nostdin -i /work/tmp/$t.mp4 -i /work/tmp/ref.mkv -filter_complex "[0:v]trim=end_frame=48,format=yuv420p10le[a];[a][1:v]psnr" -f null - 2>&1 | grep -o 'y:[0-9.inf]*' | tail -1)
  y=$(for f in /work/tmp/$t.mp4 /work/tmp/ref.mkv; do ffmpeg -nostdin -i $f -vf "trim=end_frame=48,signalstats,metadata=print:key=lavfi.signalstats.YAVG" -f null - 2>&1 | grep -o 'YAVG=[0-9.]*' | tail -1; done | tr '\n' '/')
  echo "$t,$r | $p | $y"; grep -vE 'PPS changed|FINISHME' ff.err | head -2 | cut -c1-200
  ffprobe -v error -show_entries stream=pix_fmt,color_transfer,color_primaries,width,height -select_streams v:0 -of csv=p=0 /work/tmp/$t.mp4; done
