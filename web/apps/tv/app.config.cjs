const { appleTestFlightConfig } = require("../../scripts/apple-testflight-config.cjs");

const shieldReleaseConfig = (config, environment = process.env) => {
  const channel = environment.LOOMARR_SHIELD_RELEASE_CHANNEL;
  if (!channel) return config;
  if (channel !== "sideload" && channel !== "play") {
    throw new Error("Shield release channel must be sideload or play");
  }

  const version = environment.LOOMARR_ANDROID_VERSION_NAME;
  const rawVersionCode = environment.LOOMARR_ANDROID_VERSION_CODE;
  const versionCode = Number(rawVersionCode);
  if (!version || !rawVersionCode || !/^\d+$/.test(rawVersionCode)) {
    throw new Error("Shield release requires Loomarr version name and code");
  }
  if (!Number.isSafeInteger(versionCode) || versionCode < 1 || versionCode >= 2_100_000_000) {
    throw new Error("Shield release version code must be between 1 and 2099999999");
  }

  return {
    ...config,
    name: "Loomarr",
    slug: "loomarr-tv",
    version,
    android: {
      ...config.android,
      package: "loomarr.media",
      versionCode,
    },
  };
};

// The permanent Apple TV identity: its own App Store Connect record, "Loomarr TV", apart from the
// iPhone's. The Shield identity above is a different store and package name, so either release
// leaves the other platform's prototype identity in place.
const appleTvRelease = {
  label: "Apple TV",
  name: "Loomarr TV",
  slug: "loomarr-tv",
  ios: { bundleIdentifier: "media.loomarr.tv" },
};

module.exports = ({ config }) => {
  // One build releases one store's app; both channels at once is a mis-wired release step.
  if (process.env.LOOMARR_SHIELD_RELEASE_CHANNEL && process.env.LOOMARR_APPLE_RELEASE_CHANNEL) {
    throw new Error("a TV build releases either the Shield or the Apple TV app, not both");
  }
  return appleTestFlightConfig(shieldReleaseConfig(config), appleTvRelease);
};
