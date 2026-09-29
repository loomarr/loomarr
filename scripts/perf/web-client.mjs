// Web client resource pass (#1794): Chromium driven by Playwright, never the Playwright MCP.
//
//   node scripts/perf/web-client.mjs routes [--base URL] [--runs 3]
//     Cold load of each screen in a fresh context (empty cache): bytes transferred by type (the
//     per-route bundle cost as the browser pays it, with the server's own compression), long tasks
//     and long animation frames on the main thread, total blocking time, LCP. Median of --runs.
//
//   node scripts/perf/web-client.mjs soak [--watch-min 20] [--surf-min 10] [--surf-every 20]
//     One tab on Watch: a steady watch, then surfing with the Channel up button. Every 30 s, after
//     a forced GC: JS heap, DOM nodes (all vs attached, the gap is detached nodes still referenced),
//     event listeners, the renderer processes' PSS (MSE buffers live there, not in the JS heap), and
//     the <video>'s buffered seconds ahead of and behind the playhead (hls.js back buffer).
//     Encodes run while it plays, so wrap it: flock /tmp/loomarr-gpu.lock node …
//
// Needs the web app built into the backend (make fe, or pnpm --filter @loomarr/web build and a
// backend restart) and a lane backend with dev login. Writes --out JSON; prints Markdown.
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";

const require = createRequire(new URL("../../web/apps/web/package.json", import.meta.url));
const { chromium } = require("@playwright/test");

const args = process.argv.slice(2);
const mode = args[0];
const opt = (name, dflt) => {
  const i = args.indexOf(`--${name}`);
  return i >= 0 ? args[i + 1] : dflt;
};
const base = (opt("base", process.env.BASE) ?? "http://localhost:8080").replace(/\/$/, "");
const out = opt("out");

// The main thread, observed from inside the page from the very first script.
const observe = () => {
  window.__perf = { long: [], loaf: [], lcp: 0 };
  const watch = (type, fn) => {
    try {
      new PerformanceObserver((list) => list.getEntries().forEach(fn)).observe({ type, buffered: true });
    } catch {
      // unsupported entry type in this Chromium: that column stays empty
    }
  };
  watch("longtask", (e) => window.__perf.long.push(e.duration));
  watch("long-animation-frame", (e) => window.__perf.loaf.push(e.blockingDuration ?? 0));
  watch("largest-contentful-paint", (e) => {
    window.__perf.lcp = e.startTime;
  });
};

const session = async (browser) => {
  const context = await browser.newContext({ viewport: { width: 1920, height: 1080 } });
  const login = await context.request.post(`${base}/v1/auth/dev-login`, {
    data: {},
    headers: { "X-Loomarr-Csrf": "1" },
  });
  if (!login.ok()) throw new Error(`dev-login: ${login.status()}`);
  await context.addInitScript(observe);
  return context;
};

const demoChannels = async (context) => {
  const res = await context.request.get(`${base}/v1/channels`);
  const body = await res.json();
  const list = Array.isArray(body) ? body : (body.channels ?? body.items ?? []);
  return list.filter((c) => (c.number ?? 0) >= 101).sort((a, b) => a.number - b.number);
};

const median = (xs) => {
  const s = [...xs].sort((a, b) => a - b);
  return s.length ? s[Math.floor(s.length / 2)] : 0;
};

const routes = async (browser) => {
  const probe = await session(browser);
  const [first] = await demoChannels(probe);
  await probe.close();
  const screens = [
    ["Home", "/"],
    ["Guide", "/guide"],
    ["Watch", `/channels/${first.id}/watch`],
    ["Channels", "/channels"],
    ["Dashboard", "/dashboard"],
  ];
  const runs = Number(opt("runs", 3));
  const rows = [];
  for (const [name, path] of screens) {
    const samples = [];
    for (let r = 0; r < runs; r++) {
      const context = await session(browser);
      const page = await context.newPage();
      const cdp = await context.newCDPSession(page);
      await cdp.send("Network.enable");
      await cdp.send("Performance.enable");
      const kinds = new Map();
      const bytes = { Script: 0, Stylesheet: 0, Fetch: 0, Image: 0, Font: 0, Document: 0, Other: 0 };
      const counts = { ...Object.fromEntries(Object.keys(bytes).map((k) => [k, 0])) };
      cdp.on("Network.responseReceived", (e) => kinds.set(e.requestId, e.type));
      cdp.on("Network.loadingFinished", (e) => {
        const t = kinds.get(e.requestId);
        const k = t === "XHR" ? "Fetch" : t in bytes ? t : "Other";
        bytes[k] += e.encodedDataLength;
        counts[k] += 1;
      });
      const t0 = Date.now();
      await page.goto(base + path, { waitUntil: "networkidle" });
      await page.waitForTimeout(2000); // let late effects and polls land inside the window
      const perf = await page.evaluate(() => window.__perf);
      const { metrics } = await cdp.send("Performance.getMetrics");
      const m = Object.fromEntries(metrics.map((x) => [x.name, x.value]));
      samples.push({
        url: page.url().replace(base, ""),
        idleMs: Date.now() - t0,
        lcpMs: perf.lcp,
        jsKB: bytes.Script / 1000,
        jsFiles: counts.Script,
        cssKB: bytes.Stylesheet / 1000,
        apiKB: bytes.Fetch / 1000,
        apiCalls: counts.Fetch,
        totalKB: Object.values(bytes).reduce((a, b) => a + b, 0) / 1000,
        longTasks: perf.long.length,
        longMaxMs: Math.max(0, ...perf.long),
        tbtMs: perf.long.reduce((a, d) => a + Math.max(0, d - 50), 0),
        loafBlockingMs: perf.loaf.reduce((a, b) => a + b, 0),
        scriptMs: (m.ScriptDuration ?? 0) * 1000,
        heapMB: (m.JSHeapUsedSize ?? 0) / 1e6,
      });
      await context.close();
    }
    const row = { screen: name, url: samples[0].url };
    for (const k of Object.keys(samples[0]).filter((k) => k !== "url"))
      row[k] = Math.round(median(samples.map((s) => s[k])) * 10) / 10;
    rows.push(row);
    console.error(JSON.stringify(row));
  }
  const cols = [
    "screen",
    "url",
    "totalKB",
    "jsKB",
    "jsFiles",
    "cssKB",
    "apiCalls",
    "apiKB",
    "lcpMs",
    "idleMs",
    "longTasks",
    "longMaxMs",
    "tbtMs",
    "loafBlockingMs",
    "scriptMs",
    "heapMB",
  ];
  console.log(`### web routes — ${new Date().toISOString()}, cold cache, median of ${runs}\n`);
  console.log(`| ${cols.join(" | ")} |\n|${cols.map(() => "---").join("|")}|`);
  for (const r of rows) console.log(`| ${cols.map((c) => r[c]).join(" | ")} |`);
  return { at: new Date().toISOString(), runs, rows };
};

// Renderer and GPU process PSS, from /proc: every Chromium process under this node process.
const chromePss = () => {
  const kids = new Map();
  for (const d of readdirSync("/proc")) {
    if (!/^\d+$/.test(d)) continue;
    try {
      const stat = readFileSync(`/proc/${d}/stat`, "utf8");
      const ppid = Number(stat.slice(stat.lastIndexOf(")") + 2).split(" ")[1]);
      if (!kids.has(ppid)) kids.set(ppid, []);
      kids.get(ppid).push(Number(d));
    } catch {
      // gone between readdir and read
    }
  }
  const todo = [process.pid];
  const byType = {};
  while (todo.length) {
    const p = todo.pop();
    for (const c of kids.get(p) ?? []) {
      todo.push(c);
      try {
        const cmd = readFileSync(`/proc/${c}/cmdline`, "utf8");
        const type = /--type=([a-z-]+)/.exec(cmd)?.[1] ?? (cmd.includes("chrom") ? "browser" : null);
        if (!type) continue;
        const pss = /Pss:\s+(\d+)/.exec(readFileSync(`/proc/${c}/smaps_rollup`, "utf8"));
        byType[type] = (byType[type] ?? 0) + (pss ? Number(pss[1]) / 1000 : 0);
      } catch {
        // gone
      }
    }
  }
  return byType;
};

// Listeners by target and event type (window, document, the <video>), to name a listener leak.
// JSEventListeners counts them all; this says where they are.
const listenerTypes = async (cdp) => {
  const out = {};
  for (const [name, expr] of [
    ["window", "window"],
    ["document", "document"],
    ["video", "document.querySelector('video')"],
  ]) {
    const { result } = await cdp.send("Runtime.evaluate", { expression: expr });
    if (!result.objectId) continue;
    const { listeners } = await cdp.send("DOMDebugger.getEventListeners", { objectId: result.objectId });
    for (const l of listeners) out[`${name}:${l.type}`] = (out[`${name}:${l.type}`] ?? 0) + 1;
    await cdp.send("Runtime.releaseObject", { objectId: result.objectId });
  }
  return out;
};

const soak = async (browser) => {
  const context = await session(browser);
  const [first] = await demoChannels(context);
  const page = await context.newPage();
  const cdp = await context.newCDPSession(page);
  await cdp.send("Performance.enable");
  await page.goto(`${base}/channels/${first.id}/watch`, { waitUntil: "networkidle" });
  await page.waitForFunction(
    () => {
      const v = document.querySelector("video");
      return v && v.currentTime > 1;
    },
    null,
    { timeout: 60_000 },
  );
  const watchMs = Number(opt("watch-min", 20)) * 60_000;
  const surfMs = Number(opt("surf-min", 10)) * 60_000;
  const surfEvery = Number(opt("surf-every", 20)) * 1000;
  const listenersAtStart = await listenerTypes(cdp);
  const t0 = Date.now();
  const timeline = [];
  let nextSample = 0;
  let nextSurf = watchMs;
  let surfs = 0;
  let surfFailures = 0;
  while (Date.now() - t0 < watchMs + surfMs) {
    const t = Date.now() - t0;
    if (t >= nextSurf) {
      // The controls hide while the picture plays; a pointer move brings them back, as for a user.
      await page.mouse.move(900 + (surfs % 2) * 40, 500);
      await page.mouse.move(960, 540, { steps: 4 });
      try {
        await page.getByRole("button", { name: "Channel up" }).click({ timeout: 5_000 });
        surfs += 1;
      } catch (e) {
        surfFailures += 1;
        console.error(`surf failed: ${e.message.split("\n")[0]}`);
      }
      nextSurf += surfEvery;
    }
    if (t >= nextSample) {
      await cdp.send("HeapProfiler.collectGarbage");
      const { metrics } = await cdp.send("Performance.getMetrics");
      const m = Object.fromEntries(metrics.map((x) => [x.name, x.value]));
      const dom = await page.evaluate(() => {
        let attached = 0;
        const w = document.createTreeWalker(document, NodeFilter.SHOW_ALL);
        while (w.nextNode()) attached += 1;
        const v = document.querySelector("video");
        let ahead = 0;
        let behind = 0;
        let total = 0;
        if (v) {
          for (let i = 0; i < v.buffered.length; i++) {
            const s = v.buffered.start(i);
            const e = v.buffered.end(i);
            total += e - s;
            if (v.currentTime >= s && v.currentTime <= e) {
              ahead = e - v.currentTime;
              behind = v.currentTime - s;
            }
          }
        }
        const q = v?.getVideoPlaybackQuality?.();
        return {
          attached,
          ahead,
          behind,
          total,
          playing: v ? !v.paused && v.readyState >= 3 : false,
          dropped: q?.droppedVideoFrames ?? 0,
          frames: q?.totalVideoFrames ?? 0,
        };
      });
      const pss = chromePss();
      const row = {
        min: Math.round(t / 6000) / 10,
        phase: t < watchMs ? "watch" : "surf",
        surfs,
        surfFailures,
        heapMB: Math.round((m.JSHeapUsedSize ?? 0) / 1e5) / 10,
        nodes: m.Nodes,
        attachedNodes: dom.attached,
        detachedNodes: (m.Nodes ?? 0) - dom.attached,
        listeners: m.JSEventListeners,
        documents: m.Documents,
        rendererMB: Math.round(pss.renderer ?? 0),
        gpuProcMB: Math.round(pss["gpu-process"] ?? 0),
        bufAheadS: Math.round(dom.ahead),
        bufBehindS: Math.round(dom.behind),
        bufTotalS: Math.round(dom.total),
        playing: dom.playing,
        droppedFrames: dom.dropped,
        frames: dom.frames,
      };
      timeline.push(row);
      console.error(JSON.stringify(row));
      nextSample += 30_000;
    }
    await page.waitForTimeout(1000);
  }
  await cdp.send("HeapProfiler.collectGarbage");
  const listenersAtEnd = await listenerTypes(cdp);
  await context.close();
  const pick = timeline.filter((_, i) => i % 10 === 0 || i === timeline.length - 1);
  const cols = Object.keys(timeline[0]);
  console.log(
    `### web soak — ${new Date(t0).toISOString()}, watch ${watchMs / 60000} min then surf every ${surfEvery / 1000} s for ${surfMs / 60000} min\n`,
  );
  console.log(`| ${cols.join(" | ")} |\n|${cols.map(() => "---").join("|")}|`);
  for (const r of pick) console.log(`| ${cols.map((c) => r[c]).join(" | ")} |`);
  const grown = Object.keys({ ...listenersAtStart, ...listenersAtEnd })
    .map((k) => [k, listenersAtStart[k] ?? 0, listenersAtEnd[k] ?? 0])
    .filter(([, a, b]) => a !== b);
  console.log("\n| listener (target:type) | start | end |\n|---|---|---|");
  for (const [k, a, b] of grown) console.log(`| ${k} | ${a} | ${b} |`);
  return {
    at: new Date(t0).toISOString(),
    watchMs,
    surfMs,
    surfEvery,
    timeline,
    listenersAtStart,
    listenersAtEnd,
  };
};

const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required", "--enable-precise-memory-info"],
});
try {
  let result;
  if (mode === "routes") result = await routes(browser);
  else if (mode === "soak") result = await soak(browser);
  else throw new Error("usage: web-client.mjs routes|soak [--base URL] …");
  if (out) writeFileSync(out, JSON.stringify(result, null, 2));
} finally {
  await browser.close();
}
