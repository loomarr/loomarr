# 2g: NVENC on the dev machine (RTX 3080 Ti, driver 615, ffmpeg n9.0.2), full-GPU graph. $NV_DIR holds 120 s
# stream-copy excerpts of the three samples on local NVMe (so start is warm-disk, not CIFS).
cd $NV_DIR; export TIMEFORMAT='%R %U %S'; SP=${SP:-spikepack}
IN="-analyzeduration 0 -probesize 32768 -fpsprobesize 0"; INH="-analyzeduration 0 -probesize 2000000" # n9 needs more for TrueHD
sched() { printf '[{"name":"x","file":"%s/%s.mkv","seek":%s,"dur":%s,"hdr":%s}]' $NV_DIR $1 $2 $3 $4 > $5; }
echo "class,run,probe_ms,header_ms,first_segment_ms" > p2g_start.csv
for c in h264 hevc hdr; do h=false; in=$IN; [ $c = hdr ] && h=true && in=$INH
  for i in $(seq 1 10); do sched $c $((5 + i * 7)) 30 $h s.json; rm -rf o; $SP -hw nvenc -mode fmp4 -schedule s.json -out o -seg 1 -runahead 0 -first-segment-exit -inopts "$in" 2>/dev/null
    g() { grep "\"ev\":\"$1\"" o/events.jsonl | grep -o "\"$2\":[0-9.]*" | head -1 | cut -d: -f2; }
    echo "$c,$i,$(g probe_done since_spawn_ms),$(g header since_spawn_ms),$(g first_segment ms)" >> p2g_start.csv; done; done
echo "class,content_s,real_s,user_s,sys_s,frames" > p2g_speed.csv
for c in h264 hevc hdr; do h=false; in=$IN; [ $c = hdr ] && h=true && in=$INH; sched $c 10 60 $h s.json; rm -rf o
  t=$( { time $SP -hw nvenc -mode fmp4 -schedule s.json -out o -seg 1 -runahead 0 -inopts "$in" 2>/dev/null ; } 2>&1 ); echo "$c,60,${t// /,},$(grep -o '"frames":[0-9]*' o/events.jsonl)" >> p2g_speed.csv; done
echo "n,stream,content_s,real_s,user_s,sys_s,frames" > p2g_conc.csv
for n in 4 8 12 16; do for i in $(seq 1 $n); do ( sched h264 $((2 + i)) 90 false c$i.json; rm -rf oc$i; t=$( { time $SP -hw nvenc -mode fmp4 -schedule c$i.json -out oc$i -runahead 0 -inopts "$IN" -seg 1 2>/dev/null ; } 2>&1 ); echo "$n,$i,90,${t// /,},$(grep -o '"frames":[0-9]*' oc$i/events.jsonl)" >> p2g_conc.csv; grep -o 'encoder_err":"[^"]*' oc$i/events.jsonl | grep -iE 'nvenc|session|error' | head -1 | cut -c1-200 >> p2g_errs.txt ) & done; wait; done
