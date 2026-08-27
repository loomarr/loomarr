const assert = require("node:assert/strict");
const path = require("node:path");
const { test } = require("node:test");
const { createLoomarrBabelConfig } = require("./loomarr-babel-config.cjs");

const appRoot = path.resolve(__dirname, "../apps/tv");

const configFor = (enabled) => {
  const previous = process.env.LOOMARR_TAMAGUI_COMPILER;
  if (enabled) process.env.LOOMARR_TAMAGUI_COMPILER = "1";
  else delete process.env.LOOMARR_TAMAGUI_COMPILER;
  try {
    let cacheKey;
    const config = createLoomarrBabelConfig(
      { cache: { using: (callback) => (cacheKey = callback()) } },
      appRoot,
    );
    return { cacheKey, config };
  } finally {
    if (previous === undefined) delete process.env.LOOMARR_TAMAGUI_COMPILER;
    else process.env.LOOMARR_TAMAGUI_COMPILER = previous;
  }
};

test("keeps the compiler off by default", () => {
  const { cacheKey, config } = configFor(false);
  assert.equal(cacheKey, false);
  assert.deepEqual(config.plugins, []);
  assert.match(config.presets[0], /babel-preset-expo/);
});

test("enables only the Loomarr design-system compiler with the shared config", () => {
  const { cacheKey, config } = configFor(true);
  assert.equal(cacheKey, true);
  assert.equal(config.plugins.length, 1);
  assert.match(config.plugins[0][0], /@tamagui[\\/]babel-plugin/);
  assert.deepEqual(config.plugins[0][1].components, ["@loomarr/design-system"]);
  assert.match(config.plugins[0][1].config, /web[\\/]tamagui\.config\.ts$/);
});
