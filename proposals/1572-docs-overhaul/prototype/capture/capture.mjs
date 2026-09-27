// PROTOTYPE docs capture (#1572 imagery track). Reproducible: same backend seed, same viewport,
// same routes, same scheme, animations off. Usage:
//   BASE=http://localhost:15012 API=http://localhost:18012 node capture.mjs <outdir> [light|dark]
import { createRequire } from "node:module";
import { mkdirSync } from "node:fs";
const require = createRequire(process.env.PLAYWRIGHT_DIR);
const { chromium } = require("playwright");

const [out = "raw", scheme = "dark"] = process.argv.slice(2);
const BASE = process.env.BASE;
mkdirSync(out, { recursive: true });

const SHOTS = [
  { name: "settings-connections", path: "/settings/connections" },
  { name: "settings-defaults", path: "/settings/defaults" },
  // Crop to the panel the page is about (style guide: crop to the relevant panel).
  { name: "add-channel", path: "/guide", open: "Add a channel", type: "90s Saturday morning cartoons for the kids", clip: { x: 0, y: 0, width: 1280, height: 400 } },
];

const browser = await chromium.launch();
const ctx = await browser.newContext({
  viewport: { width: 1280, height: 800 },
  deviceScaleFactor: 2,
  colorScheme: scheme,
  reducedMotion: "reduce",
  recordVideo: process.env.CLIP ? { dir: `${out}/video`, size: { width: 1280, height: 800 } } : undefined,
});
// Dev login (LOOMARR_DEV_LOGIN=1 lane backends only): an admin session with no credential.
await ctx.request.post(`${BASE}/v1/auth/dev-login`, { headers: { "X-Loomarr-Csrf": "1" } });
const page = await ctx.newPage();

for (const s of SHOTS) {
  if (process.env.CLIP && s.name !== "add-channel") continue;
  await page.goto(`${BASE}${s.path}`, { waitUntil: "networkidle" });
  await page.evaluate(() => document.fonts.ready);
  if (s.open) await page.getByRole("button", { name: s.open }).first().click();
  if (s.type) {
    const box = page.getByRole("textbox").first();
    await box.click();
    await box.pressSequentially(s.type, { delay: process.env.CLIP ? 45 : 0 });
    await page.waitForTimeout(process.env.CLIP ? 1200 : 200);
  }
  if (!process.env.CLIP) await page.screenshot({ path: `${out}/${s.name}-${scheme}.png`, clip: s.clip });
}
await ctx.close();
await browser.close();
