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

const expoConfig = async (environment) => {
  const { stdout } = await execFileAsync("pnpm", ["exec", "expo", "config", "--json"], {
    cwd: new URL("..", import.meta.url),
    env: environment,
  });
  return JSON.parse(stdout);
};

const releaseEnvironment = (overrides) => ({
  ...process.env,
  LOOMARR_IOS_RELEASE_CHANNEL: "testflight",
  LOOMARR_IOS_VERSION: "0.2.0-beta.9",
  LOOMARR_IOS_BUILD_NUMBER: "7",
  ...overrides,
});

test("uses a prototype application identity", async () => {
  const config = JSON.parse(await readFile(new URL("../app.json", import.meta.url), "utf8"));
  assert.match(config.expo.ios.bundleIdentifier, /\.prototype$/);
  assert.match(config.expo.android.package, /\.prototype$/);
  assert.equal(config.expo.orientation, "default");
  assert.ok(config.expo.plugins.includes("../../scripts/with-memory-safe-android-build.cjs"));
});

test("keeps the permanent identity unreachable outside the TestFlight build", async () => {
  const environment = { ...process.env };
  delete environment.LOOMARR_IOS_RELEASE_CHANNEL;
  delete environment.LOOMARR_IOS_VERSION;
  delete environment.LOOMARR_IOS_BUILD_NUMBER;
  const config = await expoConfig(environment);

  assert.equal(config.name, "Loomarr Mobile Prototype");
  assert.match(config.ios.bundleIdentifier, /\.prototype$/);
  assert.match(config.android.package, /\.prototype$/);
});

test("resolves the TestFlight channel to the permanent iPhone identity", async () => {
  const config = await expoConfig(releaseEnvironment());

  assert.equal(config.name, "Loomarr");
  assert.equal(config.slug, "loomarr-mobile");
  assert.equal(config.scheme, "loomarr");
  assert.equal(config.ios.bundleIdentifier, "media.loomarr.mobile");
  // Apple's grammar: CFBundleShortVersionString is exactly three dot-separated integers;
  // CFBundleVersion is one to three.
  assert.match(config.version, /^\d+\.\d+\.\d+$/);
  // Every prerelease of 0.2.0 shares the 0.2.0 TestFlight train; the build orders them.
  assert.equal(config.version, "0.2.0");
  assert.equal(config.ios.buildNumber, "7");
  assert.match(config.ios.buildNumber, /^\d+(\.\d+){0,2}$/);
  assert.match(config.android.package, /\.prototype$/);
});

test("fails closed when a TestFlight build has an invalid channel or version metadata", async () => {
  await assert.rejects(
    expoConfig(releaseEnvironment({ LOOMARR_IOS_RELEASE_CHANNEL: "appstore" })),
    /channel must be testflight/,
  );
  await assert.rejects(expoConfig(releaseEnvironment({ LOOMARR_IOS_VERSION: "" })), /must be x\.y\.z/);
  await assert.rejects(expoConfig(releaseEnvironment({ LOOMARR_IOS_VERSION: "beta" })), /must be x\.y\.z/);
  await assert.rejects(expoConfig(releaseEnvironment({ LOOMARR_IOS_VERSION: "0.2" })), /must be x\.y\.z/);
  await assert.rejects(expoConfig(releaseEnvironment({ LOOMARR_IOS_BUILD_NUMBER: "" })), /positive integer/);
  await assert.rejects(
    expoConfig(releaseEnvironment({ LOOMARR_IOS_BUILD_NUMBER: "seven" })),
    /positive integer/,
  );
  await assert.rejects(expoConfig(releaseEnvironment({ LOOMARR_IOS_BUILD_NUMBER: "0" })), /positive integer/);
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
