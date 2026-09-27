// Throwaway: play the spike's live HLS through hls.js in Chromium and count stalls across the break.
// usage: node hlsjs-stalls.cjs <playwright-dir> <hls.min.js> <base-url> <seconds> <out.json>
const [pwDir, hlsPath, base, secs, out] = process.argv.slice(2);
const { chromium } = require(pwDir);
const fs = require('fs');

(async () => {
  const browser = await chromium.launch({ args: ['--autoplay-policy=no-user-gesture-required'] });
  const page = await browser.newPage();
  await page.setContent('<video id="v" muted playsinline style="width:640px"></video>');
  await page.addScriptTag({ path: hlsPath });
  const result = await page.evaluate(async ({ base, secs }) => {
    const r = { avc: MediaSource.isTypeSupported('video/mp4; codecs="avc1.640029,mp4a.40.2"'), waiting: [], errors: [], samples: [] };
    // wait for the packager's HTTP server (the tune starts when the manifest is first requested)
    for (let i = 0; i < 600; i++) { try { await fetch(base + '/init.mp4', { method: 'HEAD' }); break; } catch { await new Promise(s => setTimeout(s, 100)); } }
    const v = document.getElementById('v');
    const hls = new Hls({ lowLatencyMode: false, liveSyncDuration: 6, startFragPrefetch: true });
    const t0 = performance.now();
    let first = false;
    v.requestVideoFrameCallback(() => { r.firstFrameMs = performance.now() - t0; first = true; });
    v.addEventListener('waiting', () => { if (first) r.waiting.push({ at: +(performance.now() - t0).toFixed(0), ct: v.currentTime }); });
    hls.on(Hls.Events.ERROR, (_, d) => r.errors.push({ at: +(performance.now() - t0).toFixed(0), ct: v.currentTime, type: d.type, details: d.details, fatal: d.fatal }));
    hls.loadSource(base + '/live.m3u8');
    hls.attachMedia(v);
    v.play().catch(e => r.playErr = String(e));
    const end = performance.now() + secs * 1000;
    while (performance.now() < end) {
      await new Promise(s => setTimeout(s, 1000));
      r.samples.push([+((performance.now() - t0) / 1000).toFixed(1), +v.currentTime.toFixed(3), v.buffered.length ? +(v.buffered.end(v.buffered.length - 1) - v.currentTime).toFixed(2) : 0]);
    }
    const q = v.getVideoPlaybackQuality();
    r.quality = { total: q.totalVideoFrames, dropped: q.droppedVideoFrames };
    return r;
  }, { base, secs: +secs });
  fs.writeFileSync(out, JSON.stringify(result, null, 1));
  console.log(JSON.stringify({ avc: result.avc, firstFrameMs: result.firstFrameMs, waiting: result.waiting.length, errors: result.errors.map(e => e.details + (e.fatal ? '!' : '')), quality: result.quality, playErr: result.playErr }));
  await browser.close();
})();
