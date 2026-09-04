import assert from "node:assert/strict";
import { access, readFile } from "node:fs/promises";
import test from "node:test";

test("keeps the TV proof isolated from the shipping application", async () => {
  const config = JSON.parse(await readFile(new URL("../app.json", import.meta.url), "utf8"));
  const tvPlugin = config.expo.plugins.find(
    (plugin) => Array.isArray(plugin) && plugin[0] === "@react-native-tvos/config-tv",
  );
  assert.equal(tvPlugin?.[1].androidTVRequired, true);
  assert.ok(config.expo.plugins.includes("../../scripts/with-memory-safe-android-build.cjs"));
  assert.equal(config.expo.name, "Loomarr TV Prototype");
  assert.equal(config.expo.slug, "loomarr-tv-prototype");
  assert.match(config.expo.ios.bundleIdentifier, /\.prototype$/);
  assert.match(config.expo.android.package, /\.prototype$/);
});

test("declares the shared TV journey and native playback boundaries", async () => {
  const manifest = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8"));

  assert.equal(manifest.dependencies["@loomarr/player"], "workspace:*");
  assert.equal(manifest.dependencies["@loomarr/ui-tv"], "workspace:*");
  assert.equal(manifest.dependencies["expo-video"], manifest.dependencies.expo);
  assert.equal(manifest.dependencies["react-native"], "npm:react-native-tvos@0.86.2-0");
  assert.match(manifest.scripts.bundle, /--platform android/);
  assert.match(manifest.scripts.bundle, /--platform ios/);
});

test("starts from SecureStore without the abandoned Compose credential bridge", async () => {
  const appSource = await readFile(new URL("../src/app.tsx", import.meta.url), "utf8");

  assert.match(appSource, /createPairingCredentialStore/);
  assert.doesNotMatch(appSource, /createMigratingPairingCredentialStore|legacyPairingSource/);
  await assert.rejects(access(new URL("../src/legacy-pairing.ts", import.meta.url)), {
    code: "ENOENT",
  });
  await assert.rejects(access(new URL("../modules/loomarr-legacy-pairing", import.meta.url)), {
    code: "ENOENT",
  });
});
