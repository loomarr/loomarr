// Docs screenshots (#1572): the same demo seed, viewport, routes and settings every run, so
// `make docs-capture` regenerates every image after a UI change. Demo data only: it signs in as
// the admin `make demo-seed` created and reads everything else from DEMO_DIR/seed.json.
//
// The watermark is verified on the GPU a few minutes after the backend starts, and an encoder
// started before that plays without it: wait for "watermark: GPU overlay verified" in the log.
//
// Usage: node capture.mjs <seed.json> <raw dir> [shot name ...]
import { mkdirSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";

const require = createRequire(process.env.PLAYWRIGHT_DIR);
const { chromium } = require("playwright");

const [seedPath, out, ...only] = process.argv.slice(2);
const seed = JSON.parse(readFileSync(seedPath, "utf8"));
const web = seed.web;
const watermarked = seed.channels.find((c) => c.watermark);
mkdirSync(out, { recursive: true });

// name → the route, and what must be on screen before the shot. `settle` waits for a playing
// stream: the watermark is burned into the picture, so the shot needs real frames.
const SHOTS = [
  { name: "settings-connections", path: "/settings/connections", ready: "Connections" },
  { name: "settings-playback", path: "/settings/system/playback", ready: "Playback" },
  // 2h: the demo episodes are 22 minutes, and a block too narrow for its title shows none.
  { name: "guide", path: "/guide", ready: seed.channels[0].name, click: "2h" },
  { name: "channel-watch", path: `/channels/${watermarked.id}/watch`, settle: "video" },
];

const browser = await chromium.launch();
const ctx = await browser.newContext({
  viewport: { width: 1280, height: 800 },
  deviceScaleFactor: 2,
  colorScheme: "dark",
  reducedMotion: "reduce",
});
const login = await ctx.request.post(`${web}/v1/auth/login`, {
  headers: { "X-Loomarr-Csrf": "1" },
  data: { username: seed.adminUser, password: seed.adminPassword },
});
if (!login.ok()) throw new Error(`sign in as ${seed.adminUser}: ${login.status()}`);

const page = await ctx.newPage();
for (const s of SHOTS.filter((s) => only.length === 0 || only.includes(s.name))) {
  await page.goto(`${web}${s.path}`, { waitUntil: "networkidle" });
  await page.evaluate(() => document.fonts.ready);
  if (s.ready) await page.getByText(s.ready, { exact: true }).first().waitFor();
  if (s.click) {
    await page.getByText(s.click, { exact: true }).first().click();
    await page.waitForLoadState("networkidle");
  }
  if (s.settle === "video") {
    await page.waitForFunction(
      () => {
        const v = document.querySelector("video");
        return v && v.readyState >= 3 && v.currentTime > 2;
      },
      null,
      { timeout: 90_000 },
    );
    // Controls fade after inactivity; move away so the shot is the picture.
    await page.mouse.move(0, 0);
    await page.waitForTimeout(4_000);
  }
  await page.screenshot({ path: `${out}/${s.name}.png` });
  console.log(`${out}/${s.name}.png`);
}
await browser.close();
