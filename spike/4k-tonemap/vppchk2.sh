ya() { ffmpeg -nostdin -i $1 -vf "signalstats,metadata=print:key=lavfi.signalstats.YAVG" -f null - 2>&1 | grep -o 'YAVG=[0-9.]*' | awk -F= '{s+=$2;n++} END{printf "n=%d mean=%.1f", n, s/n}'; }
for s in h1 h2 h4; do for t in vaapi_1080 vaapi_4k ocl_hable_1080; do tm_run $t $s 20 2 /work/tmp/x.mp4 >/dev/null; echo "$s $t $(ya /work/tmp/x.mp4)"; done; done
# the production image (no OpenCL): same tonemap_vaapi graph
