# CPU-only ladder: 4K HEVC 10-bit HDR → CPU tone-map (Hable) → libx264 veryfast, output fps held at the source rate.
X264="-c:v libx264 -preset veryfast -profile:v high -crf 22 -maxrate 8M -bufsize 12M -g 24 -bf 0"
echo "rung,res,src,first1s_ms,real_s,user_s,sys_s,frames_out" > ladder.csv
for res in 720 480; do w=$([ $res = 720 ] && echo 1280 || echo 854)
 VF="zscale=w=$w:h=$res:f=bilinear,zscale=t=linear:npl=100,format=gbrpf32le,tonemap=hable:desat=0,zscale=p=709:t=709:m=709:r=tv,format=yuv420p,fps=24000/1001"
 for rung in r0_base r1_skiploop r2_nonref r3_nokey; do case $rung in
   r0_base) D="";; r1_skiploop) D="-skip_loop_filter all";; r2_nonref) D="-skip_loop_filter all -skip_frame nonref";; r3_nokey) D="-skip_loop_filter all -skip_frame nokey";; esac
  for s in h1 h2; do : > ff.err
   o=$(t1s $IN $D -ss 20 -i $s.mkv -t 20 -map 0:v:0 -vf "$VF" $X264 $AUD $FMP4 -y /work/tmp/l.mp4 | tr -s ' ' ',')
   f=$(ffprobe -v error -count_packets -select_streams v:0 -show_entries stream=nb_read_packets -of csv=p=0 /work/tmp/l.mp4)
   echo "$rung,$res,$s,$o$f" >> ladder.csv; done; done; done
cat ladder.csv
