const productionAndroidConfig = (config, environment = process.env) => {
  const renderer = environment.LOOMARR_ANDROID_RENDERER ?? "";
  if (!renderer) return config;
  if (renderer !== "react-native") {
    throw new Error(`unsupported Loomarr Android renderer: ${renderer}`);
  }

  const version = environment.LOOMARR_ANDROID_VERSION_NAME;
  const rawVersionCode = environment.LOOMARR_ANDROID_VERSION_CODE;
  const versionCode = Number(rawVersionCode);
  if (!version || !rawVersionCode || !/^\d+$/.test(rawVersionCode)) {
    throw new Error("React Native Android release requires Loomarr version name and code");
  }
  if (!Number.isSafeInteger(versionCode) || versionCode < 1 || versionCode >= 2_100_000_000) {
    throw new Error("React Native Android release version code is outside Play's accepted range");
  }

  return {
    ...config,
    name: "Loomarr",
    version,
    android: {
      ...config.android,
      package: "loomarr.media",
      versionCode,
    },
  };
};

module.exports = ({ config }) => productionAndroidConfig(config);
module.exports.productionAndroidConfig = productionAndroidConfig;
