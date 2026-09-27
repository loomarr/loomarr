// Frame raw captures in the browser-window chrome (light and dark frame), then encode WebP.
// Usage: node frame.mjs <raw dir> <out dir>   (frame.html served at FRAME_URL)
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { readdirSync, mkdirSync, statSync } from "node:fs";
const require = createRequire(process.env.PLAYWRIGHT_DIR);
const { chromium } = require("playwright");

const [raw, out] = process.argv.slice(2);
mkdirSync(out, { recursive: true });
const PATHS = { "settings-connections": "/settings/connections", "settings-defaults": "/settings/defaults", "add-channel": "/guide" };
const browser = await chromium.launch();
for (const f of readdirSync(raw).filter((f) => f.endsWith("-dark.png"))) {
  const name = f.replace("-dark.png", "");
  for (const scheme of ["light", "dark"]) {
    const page = await browser.newPage({ colorScheme: scheme, deviceScaleFactor: 2, viewport: { width: 1400, height: 900 } });
    await page.goto(`${process.env.FRAME_URL}?src=${process.env.RAW_URL}/${f}&w=1280&path=${PATHS[name] ?? ""}`);
    await page.waitForLoadState("networkidle");
    const png = `${out}/${name}-frame-${scheme}.png`;
    await page.locator(".pad").screenshot({ path: png });
    const webp = png.replace(".png", ".webp");
    // Docs size budget: 1600 px wide (retina for an 800 px column), WebP q82.
    execFileSync("magick", [png, "-resize", "1600x", "-quality", "82", webp]);
    console.log(webp, `${Math.round(statSync(webp).size / 1024)} KB`);
    await page.close();
  }
}
await browser.close();
