// PROTOTYPE — throwaway. Screenshots the device frame of /prototype/* routes.
// node prototype-shots.mjs <outdir> <path> [<path> ...]   (path like "/prototype/home?variant=B")
import { mkdirSync } from "node:fs";
import { chromium } from "@playwright/test";

const [outdir, ...paths] = process.argv.slice(2);
mkdirSync(outdir, { recursive: true });
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1600, height: 1000 }, deviceScaleFactor: 1 });
page.on("console", (m) => {
  if (m.type() === "error") console.log("PAGE ERROR:", m.text().slice(0, 300));
});
page.on("pageerror", (e) => console.log("PAGE EXCEPTION:", e.message.slice(0, 300)));
for (const p of paths) {
  await page.goto(`http://localhost:5173${p}`, { waitUntil: "networkidle" });
  await page.waitForTimeout(400);
  const name = p.replace(/^\/prototype\//, "").replace(/[^a-z0-9]+/gi, "-").replace(/-+$/, "");
  const frame = page.locator("[data-proto-frame]").first();
  if ((await frame.count()) === 0) {
    console.log("NO FRAME:", p);
    await page.screenshot({ path: `${outdir}/${name}.png` });
    continue;
  }
  await frame.screenshot({ path: `${outdir}/${name}.png` });
  console.log("shot", `${outdir}/${name}.png`);
}
await browser.close();
