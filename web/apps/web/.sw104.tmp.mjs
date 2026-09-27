import { chromium } from "@playwright/test";
const b = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const p = await b.newPage({ viewport: { width: 1280, height: 900 } });
if (process.env.SOUND_OFF)
  await p.addInitScript(() => localStorage.setItem("loomarr.player.channel-change-sound", "off"));
await p.goto("http://localhost:15159/channels/ch_248ee9a15b70f275/watch");
await p.waitForFunction(
  () => {
    const v = document.querySelector("video");
    return v && v.readyState >= 2 && !v.paused && v.currentTime > 2;
  },
  null,
  { timeout: 60_000 },
);
const box = await p.locator("video").boundingBox();
await p.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
await p.mouse.move(box.x + box.width / 2 + 10, box.y + box.height - 20);
const up = p.getByRole("button", { name: "Channel up" });
await up.waitFor({ state: "visible" });
await up.click();
await p.waitForTimeout(15_000);
const st = await p.evaluate(() => {
  const v = document.querySelector("video");
  const b = v?.buffered;
  const ranges = [];
  for (let i = 0; i < (b?.length ?? 0); i++) ranges.push([b.start(i).toFixed(2), b.end(i).toFixed(2)]);
  return { rs: v?.readyState, t: v?.currentTime, ranges, wash: !!document.querySelector("[data-wash]") };
});
console.log(process.env.SOUND_OFF ? "sound OFF" : "sound ON", JSON.stringify(st));
await b.close();
