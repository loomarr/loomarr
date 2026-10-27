import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

// The installed SDK's catalog of native module versions, which expo-doctor also enforces.
const bundledNativeModules = async () =>
  JSON.parse(
    await readFile(createRequire(import.meta.url).resolve("expo/bundledNativeModules.json"), "utf8"),
  );

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
  LOOMARR_APPLE_RELEASE_CHANNEL: "testflight",
  LOOMARR_APPLE_VERSION: "0.2.0-beta.9",
  LOOMARR_APPLE_BUILD_NUMBER: "7",
  ...overrides,
});

test("uses a prototype application identity", async () => {
  const config = JSON.parse(await readFile(new URL("../app.json", import.meta.url), "utf8"));
  assert.match(config.expo.ios.bundleIdentifier, /\.prototype$/);
  assert.match(config.expo.android.package, /\.prototype$/);
  // Watching puts the controls under a 16:9 picture (Q-N5); landscape waits for #1831.
  assert.equal(config.expo.orientation, "portrait");
  assert.ok(config.expo.plugins.includes("../../scripts/with-memory-safe-android-build.cjs"));
});

test("keeps the permanent identity unreachable outside the TestFlight build", async () => {
  const environment = { ...process.env };
  delete environment.LOOMARR_APPLE_RELEASE_CHANNEL;
  delete environment.LOOMARR_APPLE_VERSION;
  delete environment.LOOMARR_APPLE_BUILD_NUMBER;
  const config = await expoConfig(environment);

  assert.equal(config.name, "Loomarr Mobile Prototype");
  assert.match(config.ios.bundleIdentifier, /\.prototype$/);
  assert.match(config.android.package, /\.prototype$/);
  // The release-only compliance settings never reach development builds.
  assert.equal(config.ios.config, undefined);
  assert.equal(config.ios.privacyManifests, undefined);
});

test("lets every build browse the local network for a server", async () => {
  const environment = { ...process.env };
  delete environment.LOOMARR_APPLE_RELEASE_CHANNEL;
  delete environment.LOOMARR_APPLE_VERSION;
  delete environment.LOOMARR_APPLE_BUILD_NUMBER;

  for (const config of [await expoConfig(environment), await expoConfig(releaseEnvironment())]) {
    // Without NSBonjourServices iOS refuses the browse; without the description, the prompt.
    assert.deepEqual(config.ios.infoPlist.NSBonjourServices, ["_loomarr._tcp"]);
    assert.match(config.ios.infoPlist.NSLocalNetworkUsageDescription, /home network/);
    // A server under a local DNS name may be plain HTTP, on both platforms.
    assert.ok(config.plugins.includes("../../scripts/with-loomarr-ios-network.cjs"));
    assert.ok(config.plugins.includes("../../scripts/with-loomarr-android-network.cjs"));
  }
});

test("autolinks LAN discovery into the iPhone app and browses only in the foreground", async () => {
  const manifest = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8"));
  assert.equal(manifest.dependencies["@loomarr/lan-discovery-native"], "workspace:*");
  assert.ok((await autolinkedNativeModules("ios")).has("@loomarr/lan-discovery-native"));
  // Android's Bonjour path still reports a resolved LAN address, which would not work off Wi-Fi.
  assert.equal((await autolinkedNativeModules("android")).has("@loomarr/lan-discovery-native"), false);

  const shell = await readFile(new URL("../app/index.tsx", import.meta.url), "utf8");
  assert.match(shell, /Platform\.OS === "ios" \? createNativeServerDiscovery\(\) : undefined/);
  assert.match(shell, /setAppForeground\(state === "active"\)/);
  assert.match(shell, /discovery=\{discovery\}\s*discoveryForeground=\{appForeground\}/);
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
  assert.deepEqual(config.ios.infoPlist.NSBonjourServices, ["_loomarr._tcp"]);
  assert.deepEqual(config.ios.privacyManifests.NSPrivacyAccessedAPITypes, [
    {
      NSPrivacyAccessedAPIType: "NSPrivacyAccessedAPICategoryUserDefaults",
      NSPrivacyAccessedAPITypeReasons: ["CA92.1"],
    },
  ]);
  // Everything else is the development config, untouched.
  assert.equal(config.orientation, "portrait");
  assert.ok(config.plugins.includes("../../scripts/with-memory-safe-android-build.cjs"));
  assert.match(config.android.package, /\.prototype$/);
});

test("fails closed when a TestFlight build has an invalid channel or version metadata", async () => {
  await assert.rejects(
    expoConfig(releaseEnvironment({ LOOMARR_APPLE_RELEASE_CHANNEL: "appstore" })),
    /channel must be testflight/,
  );
  await assert.rejects(expoConfig(releaseEnvironment({ LOOMARR_APPLE_VERSION: "" })), /must be x\.y\.z/);
  await assert.rejects(expoConfig(releaseEnvironment({ LOOMARR_APPLE_VERSION: "beta" })), /must be x\.y\.z/);
  await assert.rejects(expoConfig(releaseEnvironment({ LOOMARR_APPLE_VERSION: "0.2" })), /must be x\.y\.z/);
  await assert.rejects(
    expoConfig(releaseEnvironment({ LOOMARR_APPLE_BUILD_NUMBER: "" })),
    /positive integer/,
  );
  await assert.rejects(
    expoConfig(releaseEnvironment({ LOOMARR_APPLE_BUILD_NUMBER: "seven" })),
    /positive integer/,
  );
  await assert.rejects(
    expoConfig(releaseEnvironment({ LOOMARR_APPLE_BUILD_NUMBER: "0" })),
    /positive integer/,
  );
  await assert.rejects(
    expoConfig(releaseEnvironment({ LOOMARR_APPLE_RELEASE_CHANNEL: "" })),
    /requires LOOMARR_APPLE_RELEASE_CHANNEL/,
  );
  await assert.rejects(
    expoConfig(releaseEnvironment({ EXPO_PUBLIC_LOOMARR_CRASH_TEST: "1" })),
    /must not set EXPO_PUBLIC_LOOMARR_CRASH_TEST/,
  );
});

test("captures crashes for the paired server, with MetricKit linked into the iPhone app only", async () => {
  const manifest = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8"));
  assert.equal(manifest.dependencies["@loomarr/crash-reports-native"], "workspace:*");
  // The synchronous file API the fatal handler writes with; first-party Expo, at the version the SDK bundles.
  assert.equal(
    `~${manifest.dependencies["expo-file-system"]}`,
    (await bundledNativeModules())["expo-file-system"],
  );
  assert.ok((await autolinkedNativeModules("ios")).has("@loomarr/crash-reports-native"));
  assert.equal((await autolinkedNativeModules("android")).has("@loomarr/crash-reports-native"), false);

  const layout = await readFile(new URL("../app/_layout.tsx", import.meta.url), "utf8");
  assert.match(layout, /^startCrashReporting\(\);$/m);
  const shell = await readFile(new URL("../app/index.tsx", import.meta.url), "utf8");
  assert.match(shell, /useCrashReportUpload\(credential, session\)/);
  // The test trigger is gated on a bundle-time variable, never on a runtime value.
  const capture = await readFile(new URL("../src/crash-reports.ts", import.meta.url), "utf8");
  assert.match(capture, /if \(process\.env\.EXPO_PUBLIC_LOOMARR_CRASH_TEST === "1" && /);
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

test("opens on the Guide without tuning a channel", async () => {
  const shell = await readFile(new URL("../app/index.tsx", import.meta.url), "utf8");
  assert.match(shell, /usePairedClient\(\{[^}]*initialTune: "none"[^}]*\}\)/);
  assert.match(shell, /useState<ClientDestination>\("guide"\)/);
});

test("pauses the picture off Watching and forgets the pause before tuning from the Guide", async () => {
  const shell = await readFile(new URL("../app/index.tsx", import.meta.url), "utf8");
  assert.match(shell, /useShellPause\(controller, active === "watching"\)/);
  assert.match(shell, /forgetShellPause\(channelId\);\s*void controller\.tuneChannel\(channelId\)/);
});

test("builds the Expo modules core from source on iOS, against react-native-tvos", async () => {
  const manifest = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8"));

  // Expo's precompiled ExpoModulesCore is built against upstream React Native, not
  // react-native-tvos. The SDK 58 canary's sized React's ShadowNodeFamily 64 bytes smaller than
  // react-native-tvos did, so every Expo view overflowed the heap (#1816). Building it from
  // source also builds every Expo pod that depends on it (expo-video included) from source.
  assert.deepEqual(manifest.expo.autolinking.ios.buildFromSource, ["expo-modules-core"]);
});

test("pins the native modules the shared player imports to the versions the SDK bundles", async () => {
  const manifest = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8"));
  const bundled = await bundledNativeModules();

  assert.equal(`~${manifest.dependencies["expo-video"]}`, bundled["expo-video"]);
  assert.equal(`~${manifest.dependencies["expo-crypto"]}`, bundled["expo-crypto"]);
});
