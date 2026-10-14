// Throwaway: static server for the HEVC sets + a synthetic 1 s-segment live DVR playlist (window W entries,
// advancing 1/s), with optional EXT-X-SKIP delta updates. Logs every playlist request (bytes, skip) to stdout.
const http = require('http'), fs = require('fs'), path = require('path');
const root = __dirname, W = +(process.env.W || 900), SKIP = process.env.SKIP === '1', t0 = Date.now();
const head = seq => `#EXTM3U\n#EXT-X-VERSION:9\n#EXT-X-TARGETDURATION:1\n` + (SKIP ? `#EXT-X-SERVER-CONTROL:CAN-SKIP-UNTIL=6.0\n` : '') + `#EXT-X-MEDIA-SEQUENCE:${seq}\n#EXT-X-MAP:URI="/dvr/init.mp4"\n`;
function live(skip) {
  const last = Math.min(899 + Math.floor((Date.now() - t0) / 1000), 999), first = Math.max(0, last - W + 1);
  let s = head(first), from = first;
  if (skip) { const keep = 12; from = last - keep + 1; s += `#EXT-X-SKIP:SKIPPED-SEGMENTS=${from - first}\n`; }
  for (let i = from; i <= last; i++) s += `#EXT-X-PROGRAM-DATE-TIME:${new Date(1.8e12 + i * 1000).toISOString()}\n#EXTINF:1.000,\n/dvr/${i}.m4s\n`;
  return s;
}
http.createServer((req, res) => {
  const u = new URL(req.url, 'http://x'); res.setHeader('Access-Control-Allow-Origin', '*');
  if (u.pathname === '/dvr/live.m3u8') {
    const skip = SKIP && u.searchParams.get('_HLS_skip') === 'YES', body = live(skip);
    console.log(JSON.stringify({ t: Date.now() - t0, ua: (req.headers['user-agent'] || '').slice(0, 20), skip, q: u.search, bytes: body.length }));
    res.setHeader('Content-Type', 'application/vnd.apple.mpegurl'); return res.end(body);
  }
  const f = path.join(root, path.normalize(u.pathname));
  fs.readFile(f, (e, d) => { if (e) { res.statusCode = 404; return res.end(); }
    res.setHeader('Content-Type', f.endsWith('.m3u8') ? 'application/vnd.apple.mpegurl' : 'video/mp4'); res.end(d); });
}).listen(+(process.env.PORT || 8765), '0.0.0.0');
