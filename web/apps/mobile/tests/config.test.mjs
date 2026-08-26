import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("uses a prototype application identity", async () => {
  const config = JSON.parse(await readFile(new URL("../app.json", import.meta.url), "utf8"));
  assert.match(config.expo.ios.bundleIdentifier, /\.prototype$/);
  assert.match(config.expo.android.package, /\.prototype$/);
  assert.ok(config.expo.plugins.includes("../../scripts/with-memory-safe-android-build.cjs"));
});

test("runs the authoritative Guide over the still-mounted player", async () => {
  const source = await readFile(new URL("../app/index.tsx", import.meta.url), "utf8");
  assert.match(source, /createGuideSourcePort\(authenticatedFetch\)/);
  assert.match(source, /<MobileWatching[^>]+player=\{player\}/);
  assert.match(source, /<GuideJourney/);
  assert.match(source, /player\.controller\.tuneChannel\(channelId\)/);
  assert.match(source, /<SurfJourney/);
});
