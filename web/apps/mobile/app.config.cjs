// Apple accepts only three dot-separated integers as CFBundleShortVersionString, and App Store
// Connect rejects an upload whose CFBundleVersion does not increase within that version. Loomarr's
// release versions carry a prerelease suffix (0.2.0-beta.9). Every prerelease of a version shares
// that version's TestFlight train (0.2.0), and the CI run number orders builds within it.
//
// Returns { version, buildNumber } for Expo's ios config, or throws with a message the build log
// can act on.
const appleVersionFields = (releaseVersion, rawBuildNumber) => {
  const version = /^v?(\d+\.\d+\.\d+)(?:-[0-9A-Za-z.-]+)?$/.exec(releaseVersion ?? "")?.[1];
  if (!version) {
    throw new Error(
      `iPhone release version must be x.y.z with an optional suffix; got "${releaseVersion ?? ""}"`,
    );
  }
  if (!/^[1-9]\d*$/.test(rawBuildNumber ?? "")) {
    throw new Error(`iPhone release build number must be a positive integer; got "${rawBuildNumber ?? ""}"`);
  }
  return { version, buildNumber: rawBuildNumber };
};

const iosReleaseConfig = (config, environment = process.env) => {
  const channel = environment.LOOMARR_IOS_RELEASE_CHANNEL;
  if (!channel) {
    // Release metadata without a channel is a mis-wired release step, which would otherwise ship
    // the prototype identity.
    if (environment.LOOMARR_IOS_VERSION || environment.LOOMARR_IOS_BUILD_NUMBER) {
      throw new Error("iPhone release version metadata requires LOOMARR_IOS_RELEASE_CHANNEL");
    }
    return config;
  }
  if (channel !== "testflight") {
    throw new Error("iPhone release channel must be testflight");
  }

  const { version, buildNumber } = appleVersionFields(
    environment.LOOMARR_IOS_VERSION,
    environment.LOOMARR_IOS_BUILD_NUMBER,
  );

  return {
    ...config,
    name: "Loomarr",
    slug: "loomarr-mobile",
    scheme: "loomarr",
    version,
    ios: {
      ...config.ios,
      bundleIdentifier: "media.loomarr.mobile",
      buildNumber,
      // App Store Connect lets a record add iPad support later but never remove it.
      supportsTablet: false,
      config: {
        ...config.ios?.config,
        // The app uses only HTTPS through the system stack and the Keychain; declaring that here
        // stops App Store Connect asking the export-compliance question on every upload.
        usesNonExemptEncryption: false,
      },
      infoPlist: {
        ...config.ios?.infoPlist,
        // Pairing accepts any http(s) server URL, so a LAN server is a direct local-network connection.
        NSLocalNetworkUsageDescription: "Loomarr connects to your Loomarr server on your home network.",
      },
      privacyManifests: {
        ...config.ios?.privacyManifests,
        NSPrivacyAccessedAPITypes: [
          ...(config.ios?.privacyManifests?.NSPrivacyAccessedAPITypes ?? []),
          // Audit of the autolinked iOS pods: react-native-tvos, expo-constants, expo-file-system
          // and expo-system-ui ship their own manifests, expo-modules-core reads only a file size,
          // and the rest touch no required-reason API. expo-system-ui persists the root view colour
          // in UserDefaults; its manifest declares that, and the app declares it at app level as
          // well: CA92.1 = data read and written by the app itself. React Native's pod install
          // merges this with every pod's declarations, so the shipped manifest has more entries.
          {
            NSPrivacyAccessedAPIType: "NSPrivacyAccessedAPICategoryUserDefaults",
            NSPrivacyAccessedAPITypeReasons: ["CA92.1"],
          },
        ],
      },
    },
  };
};

module.exports = ({ config }) => iosReleaseConfig(config);
module.exports.iosReleaseConfig = iosReleaseConfig;
module.exports.appleVersionFields = appleVersionFields;
