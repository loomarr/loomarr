const shieldSideloadConfig = (config, environment = process.env) => {
  if (environment.LOOMARR_SHIELD_SIDELOAD !== "1") return config;

  const version = environment.LOOMARR_ANDROID_VERSION_NAME;
  const rawVersionCode = environment.LOOMARR_ANDROID_VERSION_CODE;
  const versionCode = Number(rawVersionCode);
  if (!version || !rawVersionCode || !/^\d+$/.test(rawVersionCode)) {
    throw new Error("Shield sideload requires Loomarr version name and code");
  }
  if (!Number.isSafeInteger(versionCode) || versionCode < 1 || versionCode >= 2_100_000_000) {
    throw new Error("Shield sideload version code must be between 1 and 2099999999");
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

module.exports = ({ config }) => shieldSideloadConfig(config);
module.exports.shieldSideloadConfig = shieldSideloadConfig;
