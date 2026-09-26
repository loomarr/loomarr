# 2b: cold start at scale. Files never touched by this spike (the checkpoint-1 lists are regenerated with the
# same seeds and excluded). 1 s segments, minimal probe, fMP4 path, first-segment exit.
cd /work; mkdir -p res
find /media/tv -maxdepth 3 -name '*1080p*x264*.mkv' 2>/dev/null > /tmp/all.txt
shuf -n 40 --random-source=<(yes 17) /tmp/all.txt > /tmp/used.txt
grep -vxFf /tmp/used.txt /tmp/all.txt | shuf -n 120 --random-source=<(yes 23) >> /tmp/used.txt
grep -vxFf /tmp/used.txt /tmp/all.txt | grep -vF "$SAMPLE_H264_1080P" | shuf -n 50 --random-source=<(yes 31) | sed 's/^/h264 false /' > /tmp/b.txt
find /media/tv -maxdepth 3 -name '*1080p*x265*.mkv' 2>/dev/null | grep -vF "$SAMPLE_HEVC_1080P" | shuf -n 40 --random-source=<(yes 37) | sed 's/^/hevc false /' >> /tmp/b.txt
find /media/movies -maxdepth 2 \( -name '*2160p*HDR*.mkv' -o -name '*2160p*DV*.mkv' \) 2>/dev/null | grep -vF "$SAMPLE_4K_HDR" | shuf -n 20 --random-source=<(yes 41) | sed 's/^/hdr true /' >> /tmp/b.txt
echo "pool: h264=$(wc -l < /tmp/all.txt) picked: $(cut -d' ' -f1 /tmp/b.txt | sort | uniq -c | tr '\n' ' ')"
IN="-analyzeduration 0 -probesize 32768 -fpsprobesize 0"
echo "class,idx,seek,container,probe_done_ms,header_ms,first_segment_ms,err" > res/p2b_cold.csv
i=0; while read -r cls hdr f; do i=$((i+1)); seek=$(( 60 + (i * 7919) % 900 ))
  printf '[{"name":"x","file":"%s","seek":%s,"dur":30,"hdr":%s}]' "$f" $seek $hdr > /tmp/s.json; rm -rf /tmp/o
  timeout 20 ./spikepack -mode fmp4 -schedule /tmp/s.json -out /tmp/o -seg 1 -runahead 0 -first-segment-exit -inopts "$IN" 2>/dev/null
  g() { grep "\"ev\":\"$1\"" /tmp/o/events.jsonl | grep -o "\"$2\":[0-9.]*" | head -1 | cut -d: -f2; }
  e=$(grep -c '"ev":"first_segment"' /tmp/o/events.jsonl)
  echo "$cls,$i,$seek,${f##*.},$(g probe_done since_spawn_ms),$(g header since_spawn_ms),$(g first_segment ms),$([ "$e" = 1 ] || echo nosegment)" >> res/p2b_cold.csv
done < /tmp/b.txt
