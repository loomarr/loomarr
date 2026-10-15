const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { test } = require("node:test");
const { loomarrFontFiles } = require("./with-loomarr-fonts.cjs");

const families = require("../packages/design-system/native-fonts.json");

for (const app of ["mobile", "tv"]) {
  const projectRoot = path.join(__dirname, "..", "apps", app);

  test(`${app} embeds exactly the weights the design system names, from files that exist`, () => {
    const files = loomarrFontFiles(projectRoot);
    const expected = Object.values(families).flatMap(({ file, weights }) =>
      Object.entries(weights).map(([weight, style]) => `${file}_${weight}${style}.ttf`),
    );

    assert.deepEqual(files.map((file) => path.basename(file)), expected);
    for (const file of files) assert.ok(fs.existsSync(path.resolve(projectRoot, file)), file);
  });

  test(`${app} runs the font plugin from its app config`, () => {
    const config = JSON.parse(fs.readFileSync(path.join(projectRoot, "app.json"), "utf8"));
    assert.ok(config.expo.plugins.includes("../../scripts/with-loomarr-fonts.cjs"));
  });
}
