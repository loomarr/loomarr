# 2e: serve the commercial-break schedule live (paced 1x, fMP4 path, 1 s segments) for ExoPlayer on the
# Android TV emulator. run.sh p2e.sh -p 18999:18999
cd /work; mkdir -p res
sed -e "s|\$SAMPLE_H264_1080P|$SAMPLE_H264_1080P|; s|\$SAMPLE_4K_HDR|$SAMPLE_4K_HDR|" break.json > /tmp/break.json
rm -rf /tmp/e; ./spikepack -mode fmp4 -schedule /tmp/break.json -out /tmp/e -seg 1 -runahead 12 -listahead 6 -http :18999 -linger 20s \
  -inopts "-analyzeduration 0 -probesize 32768 -fpsprobesize 0" 2>/dev/null
cp /tmp/e/events.jsonl res/p2e_events.jsonl
