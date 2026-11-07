// The TestFlight release identity of an Apple app, shared by the iPhone and Apple TV app configs.
// build-apple-testflight.sh sets LOOMARR_APPLE_RELEASE_CHANNEL=testflight with a release version and
// build number; without the channel every build keeps the app's prototype identity.

// Apple accepts only three dot-separated integers as CFBundleShortVersionString, and App Store
// Connect rejects an upload whose CFBundleVersion does not increase within that version. Loomarr's
// release versions carry a prerelease suffix (0.2.0-beta.9). Every prerelease of a version shares
// that version's TestFlight train (0.2.0), and the build number orders builds within it.
//
// Returns { version, buildNumber } for Expo's ios config, or throws with a message the build log
// can act on.
const appleVersionFields = (label, releaseVersion, rawBuildNumber) => {
  const version = /^v?(\d+\.\d+\.\d+)(?:-[0-9A-Za-z.-]+)?$/.exec(releaseVersion ?? "")?.[1];
  if (!version) {
    throw new Error(
      `${label} release version must be x.y.z with an optional suffix; got "${releaseVersion ?? ""}"`,
    );
  }
  if (!/^[1-9]\d*$/.test(rawBuildNumber ?? "")) {
    throw new Error(`${label} release build number must be a positive integer; got "${rawBuildNumber ?? ""}"`);
  }
  return { version, buildNumber: rawBuildNumber };
};

// Applies `release` ({ label, name, slug, scheme?, ios: { bundleIdentifier, ... } }) on top of the
// development config when the TestFlight channel is set. Only the iOS-family identity changes; an
// Android identity stays as the development config has it.
const appleTestFlightConfig = (config, release, environment = process.env) => {
  const channel = environment.LOOMARR_APPLE_RELEASE_CHANNEL;
  if (!channel) {
    // Release metadata without a channel is a mis-wired release step, which would otherwise ship
    // the prototype identity.
    if (environment.LOOMARR_APPLE_VERSION || environment.LOOMARR_APPLE_BUILD_NUMBER) {
      throw new Error(`${release.label} release version metadata requires LOOMARR_APPLE_RELEASE_CHANNEL`);
    }
    return config;
  }
  if (channel !== "testflight") {
    throw new Error(`${release.label} release channel must be testflight`);
  }

  // The crash-report proof trigger (apps/mobile/src/crash-reports.ts) is bundled only when this is
  // set, so a release build must never carry it.
  if (environment.EXPO_PUBLIC_LOOMARR_CRASH_TEST) {
    throw new Error(`${release.label} release builds must not set EXPO_PUBLIC_LOOMARR_CRASH_TEST`);
  }

  const { version, buildNumber } = appleVersionFields(
    release.label,
    environment.LOOMARR_APPLE_VERSION,
    environment.LOOMARR_APPLE_BUILD_NUMBER,
  );
  const { label: _label, ios, ...identity } = release;

  return {
    ...config,
    ...identity,
    version,
    ios: {
      ...config.ios,
      ...ios,
      buildNumber,
      config: {
        ...config.ios?.config,
        // The apps use only HTTPS or plain HTTP through the system stack and the Keychain;
        // declaring that here stops App Store Connect asking the export-compliance question on
        // every upload.
        usesNonExemptEncryption: false,
      },
      privacyManifests: {
        ...config.ios?.privacyManifests,
        NSPrivacyAccessedAPITypes: [
          ...(config.ios?.privacyManifests?.NSPrivacyAccessedAPITypes ?? []),
          // Audit of the autolinked Apple pods of both apps: react-native-tvos, expo-constants,
          // expo-file-system and expo-system-ui ship their own manifests, expo-modules-core reads
          // only a file size, and the rest touch no required-reason API. expo-system-ui persists
          // the root view colour in UserDefaults; its manifest declares that, and the app declares
          // it at app level as well: CA92.1 = data read and written by the app itself. React
          // Native's pod install merges this with every pod's declarations, so the shipped
          // manifest has more entries.
          {
            NSPrivacyAccessedAPIType: "NSPrivacyAccessedAPICategoryUserDefaults",
            NSPrivacyAccessedAPITypeReasons: ["CA92.1"],
          },
        ],
      },
    },
  };
};

module.exports = { appleTestFlightConfig, appleVersionFields };
