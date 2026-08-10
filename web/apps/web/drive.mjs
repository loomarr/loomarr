import { readFileSync } from "node:fs";
import { chromium } from "@playwright/test";

const id = readFileSync("/tmp/v51f-channel-id", "utf8").trim();
const b = await chromium.launch();
const ctx = await b.newContext({ viewport: { width: 1280, height: 1200 } });
const p = await ctx.newPage();
await ctx.request.post("http://localhost:8080/v1/auth/dev-login");

await p.goto(`http://localhost:5173/channels/${id}/filler`, { waitUntil: "networkidle" });
const filler = p.getByRole("button", { name: /^filler/i });
if (await filler.count()) {
  await filler.first().click();
  await p.waitForTimeout(3500);
}
const dead = await p.getByText("This clip is no longer in your catalog").count();
console.log("pins reported as gone:", dead, "(expected 1 — only the fake deadbeef hash)");
await p.screenshot({ path: "/tmp/v51f-pins.png", fullPage: false });
await b.close();
