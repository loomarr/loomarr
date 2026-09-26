# Pattern check: which PQ code does SDR white (and 50% grey) land on? BT.2408 target: white → 203 nits → Y10 ≈ 572.
DISP="35400 14600|8500 39850|6550 2300|15635 16450|10000000 50"
for c in white gray; do for p in vpp_scale vpp_tonemap libplacebo zscale203; do
 case $p in
  vpp_scale) g="-init_hw_device vaapi=va:/dev/dri/renderD128 -filter_hw_device va"; vf="format=nv12,hwupload,scale_vaapi=format=p010:out_color_matrix=bt2020nc:out_color_primaries=bt2020:out_color_transfer=smpte2084,hwdownload,format=p010le";;
  vpp_tonemap) g="-init_hw_device vaapi=va:/dev/dri/renderD128 -filter_hw_device va"; vf="format=nv12,hwupload,scale_vaapi=format=p010,tonemap_vaapi=format=p010:t=smpte2084:p=bt2020:m=bt2020nc:display=$DISP:light=1000 400,hwdownload,format=p010le";;
  libplacebo) g=""; vf="libplacebo=format=yuv420p10le:colorspace=bt2020nc:color_primaries=bt2020:color_trc=smpte2084:range=tv";;
  zscale203) g=""; vf="zscale=t=linear:npl=203:pin=709:tin=709:min=709,format=gbrpf32le,zscale=p=2020:t=smpte2084:m=2020_ncl:r=tv,format=yuv420p10le";;
 esac
 y=$(ffmpeg -nostdin $g -f lavfi -i "color=$c:s=1920x1080,format=yuv420p,setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv" -frames:v 1 -vf "$vf,signalstats,metadata=print:key=lavfi.signalstats.YAVG" -f null - 2>&1 | grep -o 'YAVG=[0-9.]*' | tail -1)
 echo "$c,$p,${y:-FAIL}"; done; done
: > ff.err; r=$(t1s $VA $IN -ss 30 -i sd.mkv -t 30 -map 0:v:0 -vf "$UP,tonemap_vaapi=format=p010:t=smpte2084:p=bt2020:m=bt2020nc:display=$DISP:light=1000 400" -c:v hevc_vaapi -profile:v main10 -rc_mode QVBR -global_quality 22 -b:v 16M -maxrate 24M $GOP $AUD $FMP4 -y /work/tmp/s2h_tm.mp4); echo "s2h_tonemap_vaapi $r"; grep -v PPS ff.err | head -3 | cut -c1-160
