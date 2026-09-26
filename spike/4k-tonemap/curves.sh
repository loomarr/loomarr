# Curve side-by-sides for the maintainer (never committed): per film, 5 candidate timestamps; pick the brightest,
# darkest and middle by HDR luma; render Hable (tonemap_opencl) | BT.2390 (libplacebo) | Intel VPP (tonemap_vaapi).
rm -rf /work/curves; mkdir -p /work/curves && cd /work/curves
for k in H1 H2 H3 H4; do f=$(grep "^$k=" /work/samples.env | cut -d= -f2-); f="/media/movies/${f##*/movies/}"
  d=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$f" | cut -d. -f1); : > cand_$k.txt
  for p in 15 30 45 60 75; do t=$((d*p/100)); y=$(ffmpeg -nostdin -ss $t -i "$f" -frames:v 1 -vf "scale=480:-2,signalstats,metadata=print:key=lavfi.signalstats.YAVG" -f null - 2>&1 | grep -o 'YAVG=[0-9.]*' | tail -1 | cut -d= -f2); echo "$y $t" >> cand_$k.txt; done
  sort -n cand_$k.txt | awk 'NR==1{print "dark",$2} NR==3{print "mid",$2} NR==5{print "bright",$2}' | while read sc t; do
    o=${k}_${sc}_t$t
    ffmpeg -nostdin -v error $OCL -ss $t -i "$f" -frames:v 1 -vf "scale_vaapi=w=1920:h=1080:format=p010,hwmap=derive_device=opencl,tonemap_opencl=tonemap=hable:desat=0:$TM709:format=nv12,hwdownload,format=nv12" -y ${o}_hable.png
    ffmpeg -nostdin -v error $VA -ss $t -i "$f" -frames:v 1 -vf "scale_vaapi=w=1920:h=1080:format=p010,hwdownload,format=p010le,libplacebo=tonemapping=bt.2390:format=yuv420p:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv" -y ${o}_bt2390.png
    ffmpeg -nostdin -v error $VA -ss $t -i "$f" -frames:v 1 -vf "scale_vaapi=w=1920:h=1080:format=p010,tonemap_vaapi=format=nv12:$TM709,hwdownload,format=nv12" -y ${o}_vpp.png
    ffmpeg -nostdin -v error -i ${o}_hable.png -i ${o}_bt2390.png -i ${o}_vpp.png -filter_complex "[0]scale=960:-2[a];[1]scale=960:-2[b];[2]scale=960:-2[c];[a][b][c]hstack=3" -y sheet_${o}.png
  done; done; ls sheet_* | wc -l; du -sh .
