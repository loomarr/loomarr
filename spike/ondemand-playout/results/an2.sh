cd /work

rm -rf slate1; ./spikepack -schedule /work/slate.json -out slate1 -seg 1 -runahead 12 -slate slate.ts -inopts "-analyzeduration 0 -probesize 32768 -fpsprobesize 0" 2>/dev/null
grep -o '"ev":"slate_fallback"[^}]*\|"item":"[^"]*","pps[^}]*"slate":[a-z]*' slate1/events.jsonl | cut -c1-160
OUT=slate1 bash /work/analyze.sh 2>&1 | sed -n '/timestamp/,/per-item/p'
