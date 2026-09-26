# 2h: software-only household host: same production image, NO /dev/dri, --cpus 4 (run_sw.sh). libx264 veryfast,
# closed 1 s GOP; 4K HDR is downscaled first, then tone-mapped on the CPU at 720p (zscale+tonemap=hable).
cd /work; mkdir -p res; export TIMEFORMAT='%R %U %S'
IN="-analyzeduration 0 -probesize 32768 -fpsprobesize 0"
sched() { printf '[{"name":"x","file":"%s","seek":%s,"dur":%s,"hdr":%s}]' "$1" $2 $3 $4 > $5; }
cls() { case $1 in h264*) echo "$SAMPLE_H264_1080P";; hevc*) echo "$SAMPLE_HEVC_1080P";; hdr*) echo "$SAMPLE_4K_HDR";; esac; }
echo "class,res,run,first_segment_ms" > res/p2h_start.csv; echo "class,res,content_s,real_s,user_s,sys_s,frames" > res/p2h_speed.csv
for c in "h264 1080 false 300" "h264 720 false 300" "hevc 1080 false 300" "hevc 720 false 300" "hdr 720 true 1200"; do set -- $c
  for i in 1 2 3 4 5; do sched "$(cls $1)" $(( $4 + i * 37 )) 20 $3 /tmp/s.json; rm -rf /tmp/o
    ./spikepack -hw sw -res $2 -mode fmp4 -schedule /tmp/s.json -out /tmp/o -seg 1 -runahead 0 -first-segment-exit -inopts "$IN" 2>/dev/null
    echo "$1,$2,$i,$(grep -o '"ev":"first_segment".*' /tmp/o/events.jsonl | grep -o '"ms":[0-9.]*' | cut -d: -f2)" >> res/p2h_start.csv; done
  sched "$(cls $1)" $4 30 $3 /tmp/s.json; rm -rf /tmp/o
  t=$( { time ./spikepack -hw sw -res $2 -mode fmp4 -schedule /tmp/s.json -out /tmp/o -seg 1 -runahead 0 -inopts "$IN" 2>/dev/null ; } 2>&1 ); echo "$1,$2,30,${t// /,},$(grep -o '"frames":[0-9]*' /tmp/o/events.jsonl)" >> res/p2h_speed.csv
done
echo "class,res,n,stream,content_s,real_s,user_s,sys_s" > res/p2h_conc.csv
for spec in "1080 2" "1080 3" "1080 4" "720 3" "720 4" "720 5" "720 6"; do set -- $spec
  for i in $(seq 1 $2); do ( sched "$SAMPLE_H264_1080P" $((60 + i * 40)) 30 false /tmp/c$i.json; t=$( { time ./spikepack -hw sw -res $1 -mode fmp4 -schedule /tmp/c$i.json -out /tmp/c$i -runahead 0 -inopts "$IN" -seg 1 2>/dev/null ; } 2>&1 ); echo "h264,$1,$2,$i,30,${t// /,}" >> res/p2h_conc.csv ) & done; wait; rm -rf /tmp/c*; done
