cd "$(dirname "$0")"; PW=${WEB:?set WEB to a checkout web/ dir}/node_modules/.pnpm/playwright@1.62.1/node_modules/playwright; HLS=${WEB:?set WEB to a checkout web/ dir}/node_modules/.pnpm/hls.js@1.7.1/node_modules/hls.js/dist/hls.min.js
for skip in 0 1; do SKIP=$skip PORT=8765 node server.cjs > srv-dvr-$skip.log & S=$!; sleep 1
  timeout 120 node play.cjs $PW $HLS dvr chromium 2>&1 | tail -1 | cut -c1-700; kill $S; wait $S 2>/dev/null; done
PORT=8765 node server.cjs > srv-hevc.log & S=$!; sleep 1; timeout 150 node play.cjs $PW $HLS hevc webkit 2>&1 | tail -2 | cut -c1-900; kill $S
for s in 0 1; do echo "server skip=$s: $(grep -c . srv-dvr-$s.log) playlist reqs; skip-requests=$(grep -c '"skip":true' srv-dvr-$s.log); bytes: $(grep -o '"bytes":[0-9]*' srv-dvr-$s.log | cut -d: -f2 | sort -n | uniq -c | sort -rn | head -3 | tr '\n' ' ')"; done
