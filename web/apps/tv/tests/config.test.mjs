import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

test("keeps the TV proof isolated from the shipping application", async () => {
  const config = JSON.parse(await readFile(new URL("../app.json", import.meta.url), "utf8"));
  const tvPlugin = config.expo.plugins.find(
    (plugin) => Array.isArray(plugin) && plugin[0] === "@react-native-tvos/config-tv",
  );
  assert.equal(tvPlugin?.[1].androidTVRequired, true);
  assert.ok(config.expo.plugins.includes("../../scripts/with-memory-safe-android-build.cjs"));
  assert.ok(config.expo.plugins.includes("../../scripts/with-workspace-bundle-inputs.cjs"));
  assert.ok(config.expo.plugins.includes("../../scripts/with-loomarr-android-release.cjs"));
  assert.ok(config.expo.plugins.includes("../../scripts/with-tv-macrobenchmark.cjs"));
  assert.ok(config.expo.plugins.includes("../../scripts/with-loomarr-android-network.cjs"));
  assert.match(config.expo.android.package, /\.prototype$/);
});

test("resolves an explicit React Native release to the permanent Android identity", async () => {
  const appRoot = new URL("..", import.meta.url);
  const { stdout } = await execFileAsync("pnpm", ["exec", "expo", "config", "--json"], {
    cwd: appRoot,
    env: {
      ...process.env,
      LOOMARR_ANDROID_RENDERER: "react-native",
      LOOMARR_ANDROID_VERSION_CODE: "1020003",
      LOOMARR_ANDROID_VERSION_NAME: "0.1.2-beta.3",
    },
  });
  const config = JSON.parse(stdout);

  assert.equal(config.name, "Loomarr");
  assert.equal(config.version, "0.1.2-beta.3");
  assert.equal(config.android.package, "loomarr.media");
  assert.equal(config.android.versionCode, 1020003);
});

test("keeps the production identity unreachable without an explicit renderer", async () => {
  const appRoot = new URL("..", import.meta.url);
  const environment = { ...process.env };
  delete environment.LOOMARR_ANDROID_RENDERER;
  delete environment.LOOMARR_ANDROID_VERSION_CODE;
  delete environment.LOOMARR_ANDROID_VERSION_NAME;
  const { stdout } = await execFileAsync("pnpm", ["exec", "expo", "config", "--json"], {
    cwd: appRoot,
    env: environment,
  });
  const config = JSON.parse(stdout);

  assert.match(config.android.package, /\.prototype$/);
  assert.notEqual(config.android.package, "loomarr.media");
});

test("rejects an unknown shipping renderer", async () => {
  const appRoot = new URL("..", import.meta.url);

  await assert.rejects(
    execFileAsync("pnpm", ["exec", "expo", "config", "--json"], {
      cwd: appRoot,
      env: { ...process.env, LOOMARR_ANDROID_RENDERER: "unknown" },
    }),
    /unsupported Loomarr Android renderer/,
  );
});

test("runs the authoritative Guide over the still-mounted TV player", async () => {
  const source = await readFile(new URL("../src/app.tsx", import.meta.url), "utf8");
  assert.match(source, /createGuideSourcePort\(authenticatedFetch\)/);
  assert.match(source, /<TvWatching[^>]+player=\{player\}/);
  assert.match(source, /<GuideJourney/);
  assert.match(source, /tvGuideRowWindow/);
  assert.match(source, /player\.controller\.tuneChannel\(channelId\)/);
  assert.match(source, /<SurfJourney/);
  assert.match(source, /onDisconnect=\{\(\) => session\.disconnect\(\)\}/);
  assert.match(source, /restoreSelection=\{restoreTvSurfSelection\}/);
  assert.match(
    source,
    /clientVersion = process\.env\.EXPO_PUBLIC_LOOMARR_CLIENT_VERSION \?\? appConfig\.expo\.version/,
  );
  assert.match(source, /clientVersion=\{clientVersion\}/);
  assert.match(source, /serverVersion=\{player\.serverVersion\}/);
  assert.match(source, /<PairedNativeImage/);
  assert.match(source, /channel\.now\.artworkUri/);
  assert.match(source, /watchingScheduleFromGuide/);
  assert.match(source, /schedule=\{schedule\}/);
  assert.match(source, /onChannelEvent/);
  assert.match(source, /ClientDiagnosticsReporter/);
  assert.match(source, /source: "android_tv"/);
});
