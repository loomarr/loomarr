cd /work; mkdir -p res; export TIMEFORMAT='%R %U %S'
A="$SAMPLE_H264_1080P"
H="$SAMPLE_HEVC_1080P"
B="$SAMPLE_4K_HDR"
sched() { printf '[{"name":"x","file":"%s","seek":%s,"dur":%s,"hdr":%s}]' "$1" $2 $3 $4 > $5; }
rss() { for p in /proc/[0-9]*; do c=$(cat $p/comm 2>/dev/null); case $c in ffmpeg|spikepack) echo "$c $(grep VmRSS $p/status | awk '{print $2}')";; esac; done; }
cls_file() { case $1 in h264) echo "$A";; hevc) echo "$H";; hdr) echo "$B";; esac; }
cls_hdr() { [ $1 = hdr ] && echo true || echo false; }
cls_max() { case $1 in h264) echo 550;; hevc) echo 1200;; hdr) echo 5500;; esac; }
if [ "$PART" = cold ]; then
 echo "class,seg,run,seek,first_segment_ms" > res/cold.csv
 for seg in 2 1; do for c in h264 hevc hdr; do for i in $(seq 1 20); do
  seek=$(( 60 + (RANDOM * 7919 + i * 104729) % $(cls_max $c) )); sched "$(cls_file $c)" $seek 30 $(cls_hdr $c) /tmp/s.json; rm -rf /tmp/o
  ./spikepack -schedule /tmp/s.json -out /tmp/o -seg $seg -runahead 0 -first-segment-exit 2>/dev/null
  echo "$c,$seg,$i,$seek,$(grep -o '"ms":[0-9.]*' /tmp/o/events.jsonl | cut -d: -f2)" >> res/cold.csv
 done; done; done
fi
if [ "$PART" = speed ]; then
 echo "class,mode,content_s,real_s,user_s,sys_s" > res/speed.csv
 for c in h264 hevc hdr; do
  sched "$(cls_file $c)" 300 60 $(cls_hdr $c) /tmp/s.json; rm -rf /tmp/o
  t=$( { time ./spikepack -schedule /tmp/s.json -out /tmp/o -runahead 0 2>/dev/null ; } 2>&1 ); echo "$c,unpaced,60,${t// /,}" >> res/speed.csv
 done
 for c in h264 hdr; do
  sched "$(cls_file $c)" 400 60 $(cls_hdr $c) /tmp/s.json; rm -rf /tmp/o
  t=$( { time ./spikepack -schedule /tmp/s.json -out /tmp/o -runahead 12 2>/dev/null ; } 2>&1 ); echo "$c,paced1x,60,${t// /,}" >> res/speed.csv
 done
fi
if [ "$PART" = conc ]; then
 sched "$A" 300 60 false /tmp/p.json; ./spikepack -schedule /tmp/p.json -out /tmp/p -runahead 0 -seg 1 -cpuprofile res/packager.pprof 2>/dev/null
 echo "hdr,n1080,stream,content_s,real_s,user_s,sys_s" > res/conc.csv; : > res/conc_rss.txt; : > res/conc_cg.txt
 for spec in "0 1" "0 2" "0 4" "0 6" "0 8" "0 10" "0 12" "0 16" "1 2" "1 4" "1 6"; do
  set -- $spec; hd=$1; n=$2; before=$(grep -E "usage_usec|nr_throttled|throttled_usec" /sys/fs/cgroup/cpu.stat | tr '\n' ' ')
  for i in $(seq 1 $n); do ( sched "$A" $((30 + i * 25)) 90 false /tmp/c$i.json; t=$( { time ./spikepack -schedule /tmp/c$i.json -out /tmp/c$i -runahead 0 -inopts "-analyzeduration 0 -probesize 32768 -fpsprobesize 0" -seg 1 2>/dev/null ; } 2>&1 ); echo "$hd,$n,$i,90,${t// /,}" >> res/conc.csv ) & done
  if [ $hd = 1 ]; then ( sched "$B" 1200 90 true /tmp/ch.json; t=$( { time ./spikepack -schedule /tmp/ch.json -out /tmp/ch -runahead 0 -inopts "-analyzeduration 0 -probesize 32768 -fpsprobesize 0" -seg 1 2>/dev/null ; } 2>&1 ); echo "$hd,$n,hdr,90,${t// /,}" >> res/conc.csv ) & fi
  sleep 4; echo "hdr=$hd n=$n $(rss | sort | awk '{s[$1]+=$2; c[$1]++} END {for (k in s) printf "%s: n=%d avgRSS=%dkB ", k, c[k], s[k]/c[k]}')" >> res/conc_rss.txt
  wait; after=$(grep -E "usage_usec|nr_throttled|throttled_usec" /sys/fs/cgroup/cpu.stat | tr '\n' ' ')
  echo "hdr=$hd n=$n before: $before after: $after" >> res/conc_cg.txt; rm -rf /tmp/c* 
 done
fi
