cd /work
find /media/tv -maxdepth 3 -name '*1080p*x264*.mkv' 2>/dev/null > /tmp/all.txt
shuf -n 40 --random-source=<(yes 17) /tmp/all.txt > /tmp/used.txt
echo "== run15 diag: $(sed -n 15p /tmp/used.txt | sed 's|.*/||' | cut -c1-12)… seek=$(( 60 + (15 * 7919) % 900 ))"
f=$(sed -n 15p /tmp/used.txt); ffprobe -v error -show_entries format=duration:stream=index,codec_type,codec_name -of csv=p=0 "$f"
printf '[{"name":"x","file":"%s","seek":%s,"dur":30}]' "$f" $(( 60 + (15 * 7919) % 900 )) > /tmp/s.json; rm -rf /tmp/o
timeout 20 ./spikepack -schedule /tmp/s.json -out /tmp/o -seg 1 -runahead 0 -first-segment-exit 2>&1 | grep -v '"ev":"start"' | cut -c1-400; echo "exit=$?"
grep -vxFf /tmp/used.txt /tmp/all.txt | shuf -n 120 --random-source=<(yes 23) > /tmp/fresh.txt; echo "fresh files: $(wc -l < /tmp/fresh.txt) of $(wc -l < /tmp/all.txt)"
echo "variant,idx,seek,probe_done_ms,first_video_ms,first_segment_ms" > res/cold_variants.csv
v=0; for opts in "" "-analyzeduration 0 -probesize 32768 -fpsprobesize 0" "-f matroska -analyzeduration 0 -probesize 32768 -fpsprobesize 0"; do v=$((v+1)); i=0
 sed -n "$(( (v-1)*40 + 1 )),$(( v*40 ))p" /tmp/fresh.txt | while read -r f; do i=$((i+1)); seek=$(( 60 + (i * 7919) % 900 ))
  printf '[{"name":"x","file":"%s","seek":%s,"dur":30}]' "$f" $seek > /tmp/s.json; rm -rf /tmp/o
  timeout 20 ./spikepack -schedule /tmp/s.json -out /tmp/o -seg 1 -runahead 0 -first-segment-exit -inopts "$opts" 2>/dev/null
  g() { grep "\"ev\":\"$1\"" /tmp/o/events.jsonl | grep -o "\"$2\":[0-9.]*" | head -1 | cut -d: -f2; }
  echo "V$((v-1)),$i,$seek,$(g probe_done since_spawn_ms),$(g first_video since_spawn_ms),$(g first_segment ms)" >> res/cold_variants.csv
 done; done
