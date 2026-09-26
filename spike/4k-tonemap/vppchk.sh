ya() { ffmpeg -nostdin -i $1 -vf "signalstats,metadata=print:key=lavfi.signalstats.YAVG" -f null - 2>&1 | grep -o 'YAVG=[0-9.]*' | awk -F= '{s+=$2;n++; if(n<=3) f=f" "$2} END{print "n="n" first3:"f" mean="s/n}'; }
tm_run vaapi_1080 h3 20 3 /work/tmp/v.mp4 >/dev/null; echo "stream vaapi_1080: $(ya /work/tmp/v.mp4)"
tm_run ocl_hable_1080 h3 20 3 /work/tmp/o.mp4 >/dev/null; echo "stream ocl_hable_1080: $(ya /work/tmp/o.mp4)"
for n in 1 2 5; do ffmpeg -nostdin -v error $VA -ss 1500 -i "/work/h3.mkv" -frames:v $n -vf "scale_vaapi=w=1920:h=1080:format=p010,tonemap_vaapi=format=nv12:$TM709,hwdownload,format=nv12" -y -update 1 /work/tmp/v$n.png; echo "png frames=$n: $(ya /work/tmp/v$n.png)"; done
