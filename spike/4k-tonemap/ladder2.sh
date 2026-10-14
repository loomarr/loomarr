echo "decode-only (null sink), 20 s content: rung,src,real,user,sys,frames"
for D in "-skip_loop_filter all -skip_frame noref"; do for s in h1 h2; do
  t=$( { time ffmpeg -nostdin -v error $IN $D -ss 20 -i $s.mkv -t 20 -map 0:v:0 -f null - 2>/dev/null; } 2>&1 | tr ' ' ',')
  n=$(ffmpeg -nostdin $IN $D -ss 20 -i $s.mkv -t 20 -map 0:v:0 -f null - 2>&1 | grep -o 'frame= *[0-9]*' | tail -1 | tr -d ' ')
  echo "[$D],$s,$t,$n"; done; done
X264="-c:v libx264 -preset veryfast -profile:v high -crf 22 -maxrate 8M -bufsize 12M -g 24 -bf 0"
for res in 720 480; do w=$([ $res = 720 ] && echo 1280 || echo 854)
 VF="zscale=w=$w:h=$res:f=bilinear,zscale=t=linear:npl=100,format=gbrpf32le,tonemap=hable:desat=0,zscale=p=709:t=709:m=709:r=tv,format=yuv420p,fps=24000/1001"
 for s in h1 h2; do o=$(t1s $IN -skip_loop_filter all -skip_frame noref -ss 20 -i $s.mkv -t 20 -map 0:v:0 -vf "$VF" $X264 $AUD $FMP4 -y /work/tmp/l.mp4 | tr -s ' ' ',')
   f=$(ffprobe -v error -count_packets -select_streams v:0 -show_entries stream=nb_read_packets -of csv=p=0 /work/tmp/l.mp4); echo "r2_noref,$res,$s,$o$f"; done; done
