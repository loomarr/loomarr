// Frame raw captures in the window chrome and encode the docs WebP (#1572 conventions: 1600 px
// wide, q82, at most 150 KB). Writes <out>/<name>-dark.webp and fails on an image over budget.
//
// Usage: node frame.mjs <raw dir> <out dir>
import { execFileSync } from "node:child_process";
import { mkdirSync, readdirSync, statSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const require = createRequire(process.env.PLAYWRIGHT_DIR);
const { chromium } = require("playwright");

const [raw, out] = process.argv.slice(2).map((p) => resolve(p));
const frame = pathToFileURL(join(dirname(fileURLToPath(import.meta.url)), "frame.html"));
const BUDGET_KB = 150;
// The address bar shows the route the capture came from (capture.mjs's SHOTS).
const ROUTES = {
  "settings-connections": "/settings/connections",
  "settings-playback": "/settings/system/playback",
  guide: "/guide",
  "channel-watch": "/channels/…/watch",
};
mkdirSync(out, { recursive: true });

const browser = await chromium.launch();
const page = await browser.newPage({ deviceScaleFactor: 2, viewport: { width: 1400, height: 900 } });
let over = 0;
for (const f of readdirSync(raw).filter((f) => f.endsWith(".png") && ROUTES[f.slice(0, -4)])) {
  const name = f.slice(0, -4);
  const src = pathToFileURL(join(raw, f));
  await page.goto(`${frame}?src=${encodeURIComponent(src)}&w=1280&path=${encodeURIComponent(ROUTES[name])}`);
  await page.locator("#i").evaluate((img) => img.decode());
  const png = join(raw, `${name}-framed.png`);
  await page.locator(".pad").screenshot({ path: png, omitBackground: true });
  const webp = join(out, `${name}-dark.webp`);
  execFileSync("magick", [png, "-resize", "1600x", "-quality", "82", "-strip", webp]);
  const kb = Math.round(statSync(webp).size / 1024);
  console.log(`${webp} ${kb} KB`);
  if (kb > BUDGET_KB) over++;
}
await browser.close();
if (over) throw new Error(`${over} screenshot(s) over the ${BUDGET_KB} KB budget`);
