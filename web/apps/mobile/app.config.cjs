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
  if (!channel) return config;
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
    },
  };
};

module.exports = ({ config }) => iosReleaseConfig(config);
module.exports.iosReleaseConfig = iosReleaseConfig;
module.exports.appleVersionFields = appleVersionFields;
