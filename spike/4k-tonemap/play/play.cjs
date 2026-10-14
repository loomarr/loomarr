// Throwaway: hls.js playback of the HEVC sets and the DVR playlist. usage: node play.cjs <pw-dir> <hls.min.js> <mode:hevc|dvr> <browser>
const [pwDir, hlsPath, mode, bname] = process.argv.slice(2);
const pw = require(pwDir), base = 'http://127.0.0.1:8765';
(async () => {
  const browser = await pw[bname].launch({ args: bname === 'chromium' ? ['--autoplay-policy=no-user-gesture-required'] : [] });
  const page = await browser.newPage();
  await page.setContent('<video id="v" muted playsinline style="width:640px"></video>');
  await page.addScriptTag({ path: hlsPath });
  const out = await page.evaluate(async ({ base, mode }) => {
    const ts = c => MediaSource.isTypeSupported(`video/mp4; codecs="${c}"`);
    const r = { hvc1_main10: ts('hvc1.2.4.L153.B0'), hev1_main10: ts('hev1.2.4.L153.B0'), hvc1_main: ts('hvc1.1.6.L153.B0'), avc: ts('avc1.640029'), runs: {} };
    const play = (url, secs) => new Promise(async done => {
      const v = document.getElementById('v'), x = { errors: [], waiting: 0, levels: [] }, t0 = performance.now();
      const hls = new Hls({ lowLatencyMode: false });
      v.requestVideoFrameCallback?.(() => x.firstFrameMs = Math.round(performance.now() - t0));
      v.onwaiting = () => x.waiting++;
      hls.on(Hls.Events.ERROR, (_, d) => x.errors.push(d.details + (d.fatal ? '!' : '')));
      hls.on(Hls.Events.LEVEL_LOADED, (_, d) => { const s = d.stats; x.levels.push({ n: d.details.fragments.length, skipped: d.details.skippedSegments || 0, load: +(s.loading.end - s.loading.start).toFixed(1), parse: +(s.parsing.end - s.parsing.start).toFixed(2), delta: !!d.details.deltaUpdateFailed === false && (d.details.skippedSegments || 0) > 0 }); });
      hls.loadSource(url); hls.attachMedia(v); v.play().catch(e => x.playErr = String(e));
      await new Promise(s => setTimeout(s, secs * 1000));
      const q = v.getVideoPlaybackQuality(); x.frames = q.totalVideoFrames; x.dropped = q.droppedVideoFrames; x.ct = +v.currentTime.toFixed(2);
      hls.destroy(); done(x);
    });
    if (mode === 'hevc') { if (r.hvc1_main10 || r.hvc1_main) for (const s of ['hdr', 'sdr']) r.runs[s] = await play(`${base}/${s}/index.m3u8`, 34); }
    else r.runs.dvr = await play(`${base}/dvr/live.m3u8`, 45);
    return r;
  }, { base, mode });
  if (out.runs.dvr) { const L = out.runs.dvr.levels.slice(1); const avg = k => +(L.reduce((a, b) => a + b[k], 0) / L.length).toFixed(2); out.runs.dvr.summary = { reloads: L.length, avgLoadMs: avg('load'), avgParseMs: avg('parse'), maxParseMs: Math.max(...L.map(l => l.parse)), skippedAvg: avg('skipped') }; delete out.runs.dvr.levels; }
  console.log(bname, mode, JSON.stringify(out)); await browser.close();
})();
