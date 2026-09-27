# usage: OUT=<dir under /work> analyze.sh
cd /work/$OUT
cat init.mp4 $(ls seg*.m4s) > /tmp/hls.mp4
ffprobe -v error -show_entries packet=stream_index,pts,duration,flags -of csv=p=0 channel.ts > ts_pkts.csv
ffprobe -v error -show_entries packet=stream_index,pts,duration,flags -of csv=p=0 /tmp/hls.mp4 > hls_pkts.csv
echo "== streams (hls)"; ffprobe -v error -show_entries stream=index,codec_name,profile,width,height,r_frame_rate,sample_rate,channels,color_transfer -of csv=p=0 /tmp/hls.mp4
echo "== decode errors hls:"; ffmpeg -nostdin -v error -i /tmp/hls.mp4 -f null - 2>&1 | sort | uniq -c | head -5
echo "== decode errors ts:"; ffmpeg -nostdin -v error -i channel.ts -f null - 2>&1 | sort | uniq -c | head -5
echo "== loudness per item (channel.ts, integrated LUFS)"
grep '"item_done"' events.jsonl | sed 's/.*"dur_s":\([0-9.]*\).*"item":"\([^"]*\)".*"start_s":\([0-9.]*\).*/\2 \3 \1/' | while read n s d; do
  I=$(ffmpeg -nostdin -hide_banner -nostats -ss $s -t $d -i channel.ts -vn -af ebur128 -f null - 2>&1 | grep -A2 "Integrated loudness" | grep " I:" | awk '{print $2}')
  echo "$n start=$s dur=$d I=$I"
done
# gaps: video delta must be one frame; audio delta one AAC frame. tb: video ticks/s, audio ticks/s
gaps() { awk -F, -v vtb=$2 -v atb=$3 -v name=$1 '
  $2=="N/A"{next}
  { s=$1; p=$2+0; d=$3+0
    if (s in last) { dl=p-last[s]; ex=(s==0)?vtb*1001/30000:atb*1024/48000; if (dl!=ex) {bad[s]++; if (dl>mx[s]) mx[s]=dl; if (dl<mn[s]||!(s in mn)) mn[s]=dl} }
    else first[s]=p
    last[s]=p; dur[s]=d; n[s]++ }
  END { for (s=0;s<2;s++) { tb=(s==0)?vtb:atb; printf "%s stream %d: pkts=%d off-grid-deltas=%d minDelta=%s maxDelta=%s (frame=%g ticks) first=%.4fs\n", name, s, n[s], bad[s]+0, mn[s]"", mx[s]"", (s==0)?vtb*1001/30000:atb*1024/48000, first[s]/tb }
        ve=(last[0]+vtb*1001/30000)/vtb; ae=(last[1]+atb*1024/48000)/atb
        printf "%s end: video_end=%.4fs audio_end=%.4fs A/V drift=%.2f ms\n", name, ve, ae, (ae-ve)*1000 }' $4; }
echo "== timestamp continuity"
gaps TS 90000 90000 ts_pkts.csv
gaps HLS 90000 48000 hls_pkts.csv
echo "== per-item (packager)"; grep item_done events.jsonl | while read -r l; do echo "$l" | grep -o "\"item\":\"[^\"]*\"\|\"first_video_ms\":[0-9.]*\|\"av_drift_end_ms\":[-0-9.e]*\|\"sps_identical\":[a-z]*\|\"pps_identical\":[a-z]*\|\"slate\":[a-z]*\|\"src_pts_gaps\":[0-9]*" | paste -sd" "; done
