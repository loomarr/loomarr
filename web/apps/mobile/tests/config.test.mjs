import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

const autolinkedNativeModules = async (platform) => {
  const autolinking = fileURLToPath(
    new URL("../node_modules/.bin/expo-modules-autolinking", import.meta.url),
  );
  const { stdout } = await execFileAsync(autolinking, [
    "react-native-config",
    "--platform",
    platform,
    "--json",
  ]);
  return new Set(Object.keys(JSON.parse(stdout).dependencies));
};

test("uses a prototype application identity", async () => {
  const config = JSON.parse(await readFile(new URL("../app.json", import.meta.url), "utf8"));
  assert.match(config.expo.ios.bundleIdentifier, /\.prototype$/);
  assert.match(config.expo.android.package, /\.prototype$/);
  assert.equal(config.expo.orientation, "default");
  assert.ok(config.expo.plugins.includes("../../scripts/with-memory-safe-android-build.cjs"));
});

test("keeps unused animation modules out of Android without breaking Apple pods", async () => {
  const manifest = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8"));
  const androidExcluded = new Set(manifest.expo.autolinking.android.exclude);

  for (const dependency of ["react-native-reanimated", "react-native-worklets"]) {
    assert.equal(manifest.dependencies[dependency], undefined);
    assert.ok(androidExcluded.has(dependency));
  }

  const androidModules = await autolinkedNativeModules("android");
  assert.equal(androidModules.has("react-native-reanimated"), false);
  assert.equal(androidModules.has("react-native-worklets"), false);

  const appleModules = await autolinkedNativeModules("ios");
  assert.equal(appleModules.has("react-native-reanimated"), false);
  assert.equal(appleModules.has("react-native-worklets"), true);
});
