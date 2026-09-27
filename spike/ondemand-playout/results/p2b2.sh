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
# 2b follow-up: same list, now warm. (1) durations of the no-segment files; (2) ts vs fmp4 path, alternating.
IN="-analyzeduration 0 -probesize 32768 -fpsprobesize 0"
echo "idx,seek,duration_s" > res/p2b_noseg.csv
for i in 1 11 15 25 30 40 41 76; do f=$(sed -n ${i}p /tmp/b.txt | cut -d' ' -f3-); echo "$i,$(( 60 + (i * 7919) % 900 )),$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$f")" >> res/p2b_noseg.csv; done
echo "mode,idx,first_segment_ms" > res/p2b_warm.csv
i=0; head -50 /tmp/b.txt | while read -r cls hdr f; do i=$((i+1)); seek=$(( 60 + (i * 7919) % 900 ))
  printf '[{"name":"x","file":"%s","seek":%s,"dur":30,"hdr":%s}]' "$f" $seek $hdr > /tmp/s.json
  for m in ts fmp4 ts fmp4; do rm -rf /tmp/o; timeout 10 ./spikepack -mode $m -schedule /tmp/s.json -out /tmp/o -seg 1 -runahead 0 -first-segment-exit -inopts "$IN" 2>/dev/null
    echo "$m,$i,$(grep -o '"ev":"first_segment".*' /tmp/o/events.jsonl | grep -o '"ms":[0-9.]*' | cut -d: -f2)" >> res/p2b_warm.csv; done
done
