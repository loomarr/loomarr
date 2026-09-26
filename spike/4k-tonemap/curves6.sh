# Six-curve sheets for the maintainer (output never committed), same frames as curves.sh:
# Hable | Mobius | Reinhard (tonemap_opencl, zero-copy) | BT.2390 | BT.2446a | Spline (libplacebo, CPU hop). 4K HDR → 1080p SDR.
# Then per-curve cost: 20 s of H1 → 1080p H.264, the same graph a channel would run.
mkdir -p /work/c6 && cd /work/c6
OCLV() { echo "scale_vaapi=w=1920:h=1080:format=p010,hwmap=derive_device=opencl,tonemap_opencl=tonemap=$1:desat=0:$TM709:format=nv12"; }
PLV() { echo "scale_vaapi=w=1920:h=1080:format=p010,hwdownload,format=p010le,libplacebo=tonemapping=$1:format=nv12:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv"; }
while read k sc t; do f=$(grep "^$k=" /work/samples.env | cut -d= -f2-); f="/media/movies/${f##*/movies/}"; o=${k}_${sc}_t$t
  for c in hable mobius reinhard; do ffmpeg -nostdin -v error $OCL -ss $t -i "$f" -frames:v 1 -vf "$(OCLV $c),hwdownload,format=nv12" -y ${o}_$c.png; done
  for c in bt.2390 bt.2446a spline; do ffmpeg -nostdin -v error $VA -ss $t -i "$f" -frames:v 1 -vf "$(PLV $c)" -y ${o}_$c.png; done
  ffmpeg -nostdin -v error -i ${o}_hable.png -i ${o}_mobius.png -i ${o}_reinhard.png -i ${o}_bt.2390.png -i ${o}_bt.2446a.png -i ${o}_spline.png \
    -filter_complex "[0]scale=960:-2[a];[1]scale=960:-2[b];[2]scale=960:-2[c];[3]scale=960:-2[d];[4]scale=960:-2[e];[5]scale=960:-2[g];[a][b][c]hstack=3[top];[d][e][g]hstack=3[bot];[top][bot]vstack" -y sheet6_$o.png
  echo "$o $(for c in hable mobius reinhard bt.2390 bt.2446a spline; do ffmpeg -nostdin -i ${o}_$c.png -vf signalstats,metadata=print:key=lavfi.signalstats.YAVG -f null - 2>&1 | grep -o 'YAVG=[0-9.]*' | tail -1 | cut -d= -f2; done | tr '\n' ' ')"
done < /work/frames.txt
H1=$(grep "^H1=" /work/samples.env | cut -d= -f2-); H1="/media/movies/${H1##*/movies/}"
echo "curve,content_s,first1s_ms,real_s,user_s,sys_s"
for c in hable mobius reinhard; do echo "$c,20,$(t1s $OCL $IN -ss 1200 -i "$H1" -t 20 -map 0:v:0 -vf "$(OCLV $c),hwmap=derive_device=vaapi:reverse=1,format=vaapi" $H264 $GOP $AUD $FMP4 -y /work/tmp/x.mp4 | tr -s ' ' ',')"; done
for c in bt.2390 bt.2446a spline; do echo "$c,20,$(t1s $VA $IN -ss 1200 -i "$H1" -t 20 -map 0:v:0 -vf "$(PLV $c),hwupload" $H264 $GOP $AUD $FMP4 -y /work/tmp/x.mp4 | tr -s ' ' ',')"; done
ls sheet6_* | wc -l
