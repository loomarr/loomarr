# 2a: packager CPU, TS demux path vs ffmpeg-fMP4 forward path. Runs inside run.sh (4 CPUs, A380 VAAPI).
cd /work; mkdir -p res; export TIMEFORMAT='%R %U %S'
IN="-analyzeduration 0 -probesize 32768 -fpsprobesize 0"
sched() { printf '[{"name":"x","file":"%s","seek":%s,"dur":%s,"hdr":%s}]' "$1" $2 $3 $4 > $5; }
usage() { grep usage_usec /sys/fs/cgroup/cpu.stat | awk '{print $2}'; }
echo "mode,class,content_s,real_s,user_s,sys_s" > res/p2a_speed.csv
for mode in ts fmp4; do for c in "h264 $SAMPLE_H264_1080P false 300" "hevc $SAMPLE_HEVC_1080P false 300" "hdr $SAMPLE_4K_HDR true 1200"; do
  set -- $c; n=$1; hdr=${@: -2:1}; seek=${@: -1}; f="${c#* }"; f="${f% * *}"
  sched "$f" $seek 60 $hdr /tmp/s.json; rm -rf /tmp/o
  t=$( { time ./spikepack -mode $mode -schedule /tmp/s.json -out /tmp/o -seg 1 -runahead 0 -inopts "$IN" 2>/dev/null ; } 2>&1 ); echo "$mode,$n,60,${t// /,}" >> res/p2a_speed.csv
done; done
sched "$SAMPLE_H264_1080P" 300 60 false /tmp/p.json; ./spikepack -mode fmp4 -schedule /tmp/p.json -out /tmp/p -runahead 0 -seg 1 -inopts "$IN" -cpuprofile res/p2a_packager.pprof 2>/dev/null
echo "n,stream,content_s,real_s,user_s,sys_s" > res/p2a_conc.csv; : > res/p2a_conc_cg.txt
for n in 12 16 20; do u0=$(usage); t0=$(date +%s.%N)
  for i in $(seq 1 $n); do ( sched "$SAMPLE_H264_1080P" $((30 + i * 25)) 90 false /tmp/c$i.json; t=$( { time ./spikepack -mode fmp4 -schedule /tmp/c$i.json -out /tmp/c$i -runahead 0 -inopts "$IN" -seg 1 2>/dev/null ; } 2>&1 ); echo "$n,$i,90,${t// /,}" >> res/p2a_conc.csv ) & done
  wait; u1=$(usage); echo "n=$n wall=$(echo "$(date +%s.%N) - $t0" | bc) cpu_s=$(( (u1-u0)/1000000 )) total_cores_at_1x=$(echo "($u1-$u0)/1000000/90" | bc -l | cut -c1-5) $(grep -E 'nr_throttled|throttled_usec' /sys/fs/cgroup/cpu.stat | tr '\n' ' ')" >> res/p2a_conc_cg.txt; rm -rf /tmp/c*
done
