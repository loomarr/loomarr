const { withAndroidManifest } = require("@expo/config-plugins");

/**
 * Loomarr's supported deployment model includes a server selected by the user
 * on a plain-HTTP trusted LAN. Android blocks that traffic by default in
 * release builds, so preserve the shipping client's explicit opt-in. HTTPS
 * origins still use the platform's normal certificate validation.
 */
function permitConfiguredLoomarrHttp(manifest) {
  const application = manifest.manifest?.application?.[0];
  if (!application?.$) {
    throw new Error("Could not find the Android application manifest while applying Loomarr networking");
  }

  application.$["android:usesCleartextTraffic"] = "true";
  return manifest;
}

function withLoomarrAndroidNetwork(config) {
  return withAndroidManifest(config, (manifestConfig) => {
    manifestConfig.modResults = permitConfiguredLoomarrHttp(manifestConfig.modResults);
    return manifestConfig;
  });
}

module.exports = withLoomarrAndroidNetwork;
module.exports.permitConfiguredLoomarrHttp = permitConfiguredLoomarrHttp;
