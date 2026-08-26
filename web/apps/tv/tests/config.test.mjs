import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("keeps the TV proof isolated from the shipping application", async () => {
  const config = JSON.parse(await readFile(new URL("../app.json", import.meta.url), "utf8"));
  const tvPlugin = config.expo.plugins.find(
    (plugin) => Array.isArray(plugin) && plugin[0] === "@react-native-tvos/config-tv",
  );
  assert.equal(tvPlugin?.[1].androidTVRequired, true);
  assert.ok(config.expo.plugins.includes("../../scripts/with-memory-safe-android-build.cjs"));
  assert.ok(config.expo.plugins.includes("../../scripts/with-loomarr-android-network.cjs"));
  assert.match(config.expo.android.package, /\.prototype$/);
});

test("runs the authoritative Guide over the still-mounted TV player", async () => {
  const source = await readFile(new URL("../src/app.tsx", import.meta.url), "utf8");
  assert.match(source, /createGuideSourcePort\(authenticatedFetch\)/);
  assert.match(source, /<TvWatching[^>]+player=\{player\}/);
  assert.match(source, /<GuideJourney/);
  assert.match(source, /player\.controller\.tuneChannel\(channelId\)/);
  assert.match(source, /<SurfJourney/);
  assert.match(source, /clientVersion=\{appConfig\.expo\.version\}/);
  assert.match(source, /serverVersion=\{player\.serverVersion\}/);
  assert.match(source, /<PairedNativeImage/);
  assert.match(source, /channel\.now\.artworkUri/);
});
