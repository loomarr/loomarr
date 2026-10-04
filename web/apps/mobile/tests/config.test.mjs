import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const { appleVersionFields } = createRequire(import.meta.url)("../app.config.cjs");

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
  // The release-only compliance settings never reach development builds.
  assert.equal(config.ios.config, undefined);
  assert.equal(config.ios.infoPlist, undefined);
  assert.equal(config.ios.privacyManifests, undefined);
});

test("resolves the TestFlight channel to the permanent iPhone identity", async () => {
  const config = await expoConfig(releaseEnvironment());

  assert.equal(config.name, "Loomarr");
  assert.equal(config.slug, "loomarr-mobile");
  assert.equal(config.scheme, "loomarr");
  assert.equal(config.ios.bundleIdentifier, "media.loomarr.mobile");
  // Every prerelease of 0.2.0 shares the 0.2.0 TestFlight train; the build orders them.
  assert.equal(config.version, "0.2.0");
  assert.equal(config.ios.buildNumber, "7");
  assert.equal(config.ios.supportsTablet, false);
  // Without these App Store Connect asks the export-compliance question on every upload, the local
  // network prompt has no explanation, and required-reason API use is undeclared.
  assert.equal(config.ios.config.usesNonExemptEncryption, false);
  assert.match(config.ios.infoPlist.NSLocalNetworkUsageDescription, /home network/);
  assert.deepEqual(config.ios.privacyManifests.NSPrivacyAccessedAPITypes, [
    {
      NSPrivacyAccessedAPIType: "NSPrivacyAccessedAPICategoryUserDefaults",
      NSPrivacyAccessedAPITypeReasons: ["CA92.1"],
    },
  ]);
  // Everything else is the development config, untouched.
  assert.equal(config.orientation, "default");
  assert.ok(config.plugins.includes("../../scripts/with-memory-safe-android-build.cjs"));
  assert.match(config.android.package, /\.prototype$/);
});

test("maps release versions onto Apple's version grammar", () => {
  assert.deepEqual(appleVersionFields("v0.2.0", "12"), { version: "0.2.0", buildNumber: "12" });
  assert.deepEqual(appleVersionFields("1.10.3-rc.2", "300"), { version: "1.10.3", buildNumber: "300" });
  // CFBundleShortVersionString is exactly three integers; the build is the CI run number, so a
  // single positive integer without leading zeros.
  assert.throws(() => appleVersionFields("0.2.0.1", "1"), /must be x\.y\.z/);
  assert.throws(() => appleVersionFields("0.2.0", "007"), /positive integer/);
  assert.throws(() => appleVersionFields("0.2.0", "1.2"), /positive integer/);
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
  await assert.rejects(
    expoConfig(releaseEnvironment({ LOOMARR_IOS_RELEASE_CHANNEL: "" })),
    /requires LOOMARR_IOS_RELEASE_CHANNEL/,
  );
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
