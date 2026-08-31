const { createLoomarrBabelConfig } = require("../../scripts/loomarr-babel-config.cjs");

module.exports = (api) => createLoomarrBabelConfig(api, __dirname);
