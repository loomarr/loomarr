const path = require("node:path");

const createLoomarrBabelConfig = (api, appRoot) => {
  const compilerEnabled = process.env.LOOMARR_TAMAGUI_COMPILER === "1";
  api.cache.using(() => compilerEnabled);
  const workspaceRoot = path.resolve(appRoot, "../..");
  const expoPackage = require.resolve("expo/package.json", { paths: [appRoot] });
  const preset = require.resolve("babel-preset-expo", { paths: [expoPackage] });
  const plugins = compilerEnabled
    ? [
        [
          require.resolve("@tamagui/babel-plugin", { paths: [workspaceRoot] }),
          {
            components: ["@loomarr/design-system"],
            config: path.join(workspaceRoot, "tamagui.config.ts"),
            disableExtraction: false,
            logTimings: false,
          },
        ],
      ]
    : [];
  return { plugins, presets: [preset] };
};

module.exports = { createLoomarrBabelConfig };
