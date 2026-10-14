cd /work; A="$SAMPLE_H264_1080P"
run() { for i in $(seq 1 20); do seek=$(( 60 + (i * 104729 + $2) % 550 ))
  printf '[{"name":"x","file":"%s","seek":%s,"dur":30}]' "$A" $seek > /tmp/s.json; rm -rf /tmp/o
  ./spikepack -schedule /tmp/s.json -out /tmp/o -seg 2 -runahead 0 -first-segment-exit 2>/dev/null
  echo "$1,$i,$seek,$(grep -o '"since_spawn_ms":[0-9.]*' /tmp/o/events.jsonl | cut -d: -f2),$(grep -o '"ms":[0-9.]*' /tmp/o/events.jsonl | cut -d: -f2)"; done; }
echo "cache,run,seek,first_video_ms,first_segment_ms" > res/cold_io.csv
run cold 7 >> res/cold_io.csv
cat "$A" > /dev/null
run warm 7 >> res/cold_io.csv
