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

test("composes the production TV root around shared dark pairing and paired API transport", async () => {
  const appSource = await readFile(new URL("../src/app.tsx", import.meta.url), "utf8");

  assert.match(appSource, /<SafeAreaProvider>/);
  assert.match(appSource, /<LoomarrProvider insets=\{insets\} theme="dark">/);
  assert.match(appSource, /<PairingShell/);
  assert.match(appSource, /createPairingTransport/);
  assert.match(appSource, /validatePairingCredential/);
  assert.match(appSource, /createAuthenticatedFetch\(credential, onRevoked\)/);
  assert.match(appSource, /<TvPairedRoot credential=\{credential\} session=\{session\} \/>/);
});

test("keeps the native player and Watching mounted beneath Guide and Surf", async () => {
  const appSource = await readFile(new URL("../src/app.tsx", import.meta.url), "utf8");

  const watching = appSource.indexOf("<WatchingSurface");
  const overlay = appSource.indexOf('{active === "watching" ? null : (');
  assert.ok(watching >= 0, "the paired root must render the shared Watching surface");
  assert.ok(overlay > watching, "transient destinations must render after the mounted Watching surface");
  assert.match(appSource, /chromeVisible=\{active === "watching"\}/);
  assert.match(appSource, /player=\{<NativePlayerView style=\{\{ flex: 1 \}\} transport=\{transport\} \/>\}/);
  assert.match(appSource, /position: "absolute"/);
});

test("drives every Watching state from the generated catalog and authoritative Guide", async () => {
  const appSource = await readFile(new URL("../src/app.tsx", import.meta.url), "utf8");

  assert.match(appSource, /createChannelCatalogPort\(runtime\.request\)/);
  assert.match(appSource, /createGuideSourcePort\(runtime\.request\)/);
  assert.match(appSource, /await controller\.reconcile\(await catalog\.list\(request\.signal\)\)/);
  assert.match(appSource, /watchingScheduleFromGuide\(/);
  assert.match(appSource, /loading=\{catalogLoading\}/);
  assert.match(appSource, /loadError=\{loadError\}/);
  assert.match(appSource, /loadError \? refresh\(\) : controller\.retry\(\)/);
});

test("mounts the bounded authoritative Guide and returns tune intent to Watching", async () => {
  const appSource = await readFile(new URL("../src/app.tsx", import.meta.url), "utf8");

  assert.match(appSource, /active === "guide"/);
  assert.match(appSource, /<GuideJourney/);
  assert.match(appSource, /tvGuideRowWindow\(/);
  assert.match(appSource, /controller=\{guide\}/);
  assert.match(appSource, /void controller\.tuneChannel\(channelId\)/);
  assert.match(appSource, /setActive\("watching"\)/);
});
